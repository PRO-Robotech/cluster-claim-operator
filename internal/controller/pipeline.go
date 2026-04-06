/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/labels"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/naming"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/renderer"
)

// event emits a Kubernetes event on the claim if the recorder is set.
func (r *ClusterClaimReconciler) event(claim *clusterclaimv1alpha1.ClusterClaim, eventType, reason, messageFmt string, args ...interface{}) {
	if r.Recorder != nil {
		r.Recorder.Eventf(claim, eventType, reason, messageFmt, args...)
	}
}

// StepResult indicates the outcome of a pipeline step.
type StepResult int

const (
	// Proceed indicates the step completed and the pipeline should continue.
	Proceed StepResult = iota
	// Wait indicates the step is waiting for an external condition.
	Wait
)

type pipelineStep struct {
	name string
	fn   func(context.Context, *clusterclaimv1alpha1.ClusterClaim, *renderer.TemplateContext) (StepResult, error)
}

// executePipeline runs the full pipeline of steps for a ClusterClaim.
func (r *ClusterClaimReconciler) executePipeline(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	oldStatus := claim.Status.DeepCopy()
	setPhase(claim, clusterclaimv1alpha1.PhaseProvisioning)

	// Convert typed claim to unstructured for template context.
	claimUnstructured, err := toUnstructured(claim, r.Scheme)
	if err != nil {
		setFailed(claim, "ConversionError", err)
		_ = r.updateStatusIfChanged(ctx, claim, oldStatus)
		return ctrl.Result{}, err
	}

	// Pre-fetch clusters so all steps see consistent computed fields.
	var infraCluster, clientCluster *unstructured.Unstructured

	infraObj := &unstructured.Unstructured{}
	infraObj.SetGroupVersionKind(ClusterGVK)
	if err := r.Get(ctx, client.ObjectKey{
		Name:      naming.ClusterName(claim.Name, "infra"),
		Namespace: claim.Namespace,
	}, infraObj); err == nil {
		infraCluster = infraObj
	}

	if claim.Spec.Client.Enabled {
		clientObj := &unstructured.Unstructured{}
		clientObj.SetGroupVersionKind(ClusterGVK)
		if err := r.Get(ctx, client.ObjectKey{
			Name:      naming.ClusterName(claim.Name, "client"),
			Namespace: claim.Namespace,
		}, clientObj); err == nil {
			clientCluster = clientObj
		}
	}

	tmplCtx := renderer.BuildContext(claimUnstructured, infraCluster, clientCluster)

	// Mirror Cluster statuses into ClusterClaim on every reconcile.
	syncClusterStatuses(ctx, claim, infraCluster, clientCluster)

	steps := []pipelineStep{
		{"Application", r.stepApplication},
		{"CertificateSetInfra", r.stepCertificateSetInfra},
		{"WaitCertSetReady", r.stepWaitCertSetReady},
		{"ClusterInfra", r.stepClusterInfra},
		{"WaitInfraProvisioned", r.stepWaitInfraProvisioned},
		{"CertificateSetClient", r.stepCertificateSetClient},
		{"WaitInfraCPReady", r.stepWaitInfraCPReady},
		{"CcmCsrc", r.stepCcmCsrc},
		{"RemoteConfigMaps", r.stepRemoteConfigMaps},
		{"ClusterClient", r.stepClusterClient},
		{"WaitClientCPReady", r.stepWaitClientCPReady},
		{"CcmCsrcUpdate", r.stepCcmCsrcUpdate},
	}

	for _, s := range steps {
		result, stepErr := s.fn(ctx, claim, &tmplCtx)
		if stepErr != nil {
			wrappedErr := fmt.Errorf("step %s: %w", s.name, stepErr)
			if IsTerminalError(stepErr) {
				logger.Error(stepErr, "pipeline step failed (terminal)", "step", s.name)
				r.event(claim, corev1.EventTypeWarning, "StepFailed", "Step %s failed (terminal): %v", s.name, stepErr)
				setFailed(claim, "StepFailed", wrappedErr)
				_ = r.updateStatusIfChanged(ctx, claim, oldStatus)
				return ctrl.Result{}, nil
			}
			logger.Info("pipeline step failed (transient, will retry)", "step", s.name, "error", stepErr)
			r.event(claim, corev1.EventTypeWarning, "StepRetrying", "Step %s failed (transient): %v", s.name, stepErr)
			setCondition(claim, clusterclaimv1alpha1.ConditionReady, metav1.ConditionFalse, "StepRetrying", wrappedErr.Error())
			_ = r.updateStatusIfChanged(ctx, claim, oldStatus)
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}
		if result == Wait {
			_ = r.updateStatusIfChanged(ctx, claim, oldStatus)
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
		}
	}

	// All steps completed.
	setReady(claim)
	r.event(claim, corev1.EventTypeNormal, "ClusterClaimReady", "All pipeline steps completed successfully")
	if err := r.updateStatusIfChanged(ctx, claim, oldStatus); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

// stepClusterClient creates or updates the client CAPI Cluster (Step 10).
// Skipped when client.enabled is false.
func (r *ClusterClaimReconciler) stepClusterClient(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, tmplCtx *renderer.TemplateContext) (StepResult, error) {
	if !claim.Spec.Client.Enabled {
		return Proceed, nil
	}
	if claim.Spec.ClusterTemplateRef.Client == nil {
		return Proceed, NewTerminalErrorf("client enabled but clusterTemplateRef.client not set")
	}
	if err := r.ensureResource(ctx, claim, claim.Spec.ClusterTemplateRef.Client.Name, naming.ClusterName(claim.Name, "client"), claim.Namespace, *tmplCtx); err != nil {
		return Proceed, err
	}
	if err := r.ensureResourceFinalizer(ctx, ClusterGVK, naming.ClusterName(claim.Name, "client"), claim.Namespace); err != nil {
		return Proceed, err
	}
	r.event(claim, corev1.EventTypeNormal, "CreatedClusterClient", "Cluster[client] %s created", naming.ClusterName(claim.Name, "client"))
	return Proceed, nil
}

// stepWaitClientCPReady waits for Cluster[client] controlPlaneInitialized and
// extracts client cluster info into the template context (Step 11).
// Skipped when client.enabled is false.
func (r *ClusterClaimReconciler) stepWaitClientCPReady(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, tmplCtx *renderer.TemplateContext) (StepResult, error) {
	if !claim.Spec.Client.Enabled {
		return Proceed, nil
	}

	cluster, err := r.getResource(ctx, ClusterGVK, naming.ClusterName(claim.Name, "client"), claim.Namespace)
	if err != nil {
		return Proceed, fmt.Errorf("get Cluster[client]: %w", err)
	}

	cpInit, _, _ := unstructured.NestedBool(cluster.Object, "status", "initialization", "controlPlaneInitialized")
	if !cpInit {
		setWaiting(claim, clusterclaimv1alpha1.ConditionClientCPReady, "Waiting for Cluster[client] control plane initialization")
		return Wait, nil
	}

	tmplCtx.ClientControlPlaneInitialized = true
	host, _, _ := unstructured.NestedString(cluster.Object, "spec", "controlPlaneEndpoint", "host")
	port, _, _ := unstructured.NestedInt64(cluster.Object, "spec", "controlPlaneEndpoint", "port")
	if host != "" {
		tmplCtx.ClientControlPlaneEndpoint = &renderer.ControlPlaneEndpoint{Host: host, Port: port}
	}

	available, _, _ := unstructured.NestedInt64(cluster.Object, "status", "controlPlane", "availableReplicas")
	desired, _, _ := unstructured.NestedInt64(cluster.Object, "status", "controlPlane", "desiredReplicas")
	tmplCtx.ClientControlPlaneAvailableReplicas = int32(available)
	tmplCtx.ClientControlPlaneDesiredReplicas = int32(desired)

	r.event(claim, corev1.EventTypeNormal, "ClientCPReady", "Cluster[client] control plane is initialized")
	setCondition(claim, clusterclaimv1alpha1.ConditionClientCPReady, metav1.ConditionTrue, "Ready", "Cluster[client] control plane initialized")
	return Proceed, nil
}

// stepCcmCsrcUpdate re-renders and updates the CcmCsrc resource with client info (Step 12).
// Skipped when client.enabled is false.
func (r *ClusterClaimReconciler) stepCcmCsrcUpdate(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, tmplCtx *renderer.TemplateContext) (StepResult, error) {
	if !claim.Spec.Client.Enabled {
		return Proceed, nil
	}
	if err := r.ensureResource(ctx, claim, claim.Spec.CcmCsrTemplateRef.Name, naming.CcmCsrcName(claim.Name), claim.Namespace, *tmplCtx); err != nil {
		return Proceed, err
	}
	r.event(claim, corev1.EventTypeNormal, "UpdatedCcmCsrc", "CcmCsrc %s updated with client info", naming.CcmCsrcName(claim.Name))
	return Proceed, nil
}

// toUnstructured converts a typed ClusterClaim to unstructured.
func toUnstructured(claim *clusterclaimv1alpha1.ClusterClaim, scheme *runtime.Scheme) (*unstructured.Unstructured, error) {
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(claim)
	if err != nil {
		return nil, fmt.Errorf("convert claim to unstructured: %w", err)
	}
	u := &unstructured.Unstructured{Object: obj}
	gvks, _, _ := scheme.ObjectKinds(claim)
	if len(gvks) > 0 {
		u.SetGroupVersionKind(gvks[0])
	}
	return u, nil
}

// stepApplication creates or updates the ArgoCD Application (Step 1).
func (r *ClusterClaimReconciler) stepApplication(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, tmplCtx *renderer.TemplateContext) (StepResult, error) {
	if err := r.ensureResource(ctx, claim, claim.Spec.ObserveTemplateRef.Name, naming.ApplicationName(claim.Name), claim.Namespace, *tmplCtx); err != nil {
		return Proceed, err
	}
	if err := r.ensureResourceFinalizer(ctx, ApplicationGVK, naming.ApplicationName(claim.Name), claim.Namespace); err != nil {
		return Proceed, err
	}
	r.event(claim, corev1.EventTypeNormal, "CreatedApplication", "Application %s created", naming.ApplicationName(claim.Name))
	setCondition(claim, clusterclaimv1alpha1.ConditionApplicationCreated, metav1.ConditionTrue, "Created", "Application created successfully")
	return Proceed, nil
}

// stepCertificateSetInfra creates or updates the infra CertificateSet (Step 2).
func (r *ClusterClaimReconciler) stepCertificateSetInfra(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, tmplCtx *renderer.TemplateContext) (StepResult, error) {
	if err := r.ensureResource(ctx, claim, claim.Spec.CertificateSetTemplateRef.Infra.Name, naming.CertificateSetName(claim.Name, "infra"), claim.Namespace, *tmplCtx); err != nil {
		return Proceed, err
	}
	if err := r.ensureResourceFinalizer(ctx, CertificateSetGVK, naming.CertificateSetName(claim.Name, "infra"), claim.Namespace); err != nil {
		return Proceed, err
	}
	return Proceed, nil
}

// stepWaitCertSetReady waits for CertificateSet[infra] to become Ready (Step 3).
func (r *ClusterClaimReconciler) stepWaitCertSetReady(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, _ *renderer.TemplateContext) (StepResult, error) {
	certSetName := naming.CertificateSetName(claim.Name, "infra")
	certSet, err := r.getResource(ctx, CertificateSetGVK, certSetName, claim.Namespace)
	if err != nil {
		return Proceed, fmt.Errorf("get CertificateSet[infra]: %w", err)
	}

	if !isConditionTrue(certSet, "Ready") {
		setWaiting(claim, clusterclaimv1alpha1.ConditionInfraCertificateReady, "Waiting for CertificateSet[infra] to become Ready")
		return Wait, nil
	}

	r.event(claim, corev1.EventTypeNormal, "CertificateSetInfraReady", "CertificateSet[infra] is Ready")
	setCondition(claim, clusterclaimv1alpha1.ConditionInfraCertificateReady, metav1.ConditionTrue, "Ready", "CertificateSet[infra] is Ready")
	return Proceed, nil
}

// stepClusterInfra creates or updates the infra CAPI Cluster (Step 4).
func (r *ClusterClaimReconciler) stepClusterInfra(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, tmplCtx *renderer.TemplateContext) (StepResult, error) {
	if err := r.ensureResource(ctx, claim, claim.Spec.ClusterTemplateRef.Infra.Name, naming.ClusterName(claim.Name, "infra"), claim.Namespace, *tmplCtx); err != nil {
		return Proceed, err
	}
	if err := r.ensureResourceFinalizer(ctx, ClusterGVK, naming.ClusterName(claim.Name, "infra"), claim.Namespace); err != nil {
		return Proceed, err
	}

	r.event(claim, corev1.EventTypeNormal, "CreatedClusterInfra", "Cluster[infra] %s created", naming.ClusterName(claim.Name, "infra"))
	return Proceed, nil
}

// stepWaitInfraProvisioned waits for Cluster[infra] infrastructureProvisioned and
// extracts controlPlaneEndpoint into the template context (Step 5).
func (r *ClusterClaimReconciler) stepWaitInfraProvisioned(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, tmplCtx *renderer.TemplateContext) (StepResult, error) {
	clusterName := naming.ClusterName(claim.Name, "infra")
	cluster, err := r.getResource(ctx, ClusterGVK, clusterName, claim.Namespace)
	if err != nil {
		return Proceed, fmt.Errorf("get Cluster[infra]: %w", err)
	}

	provisioned, _, _ := unstructured.NestedBool(cluster.Object, "status", "initialization", "infrastructureProvisioned")
	if !provisioned {
		setWaiting(claim, clusterclaimv1alpha1.ConditionInfraProvisioned, "Waiting for Cluster[infra] infrastructure to be provisioned")
		return Wait, nil
	}

	// Extract control plane endpoint into template context.
	host, _, _ := unstructured.NestedString(cluster.Object, "spec", "controlPlaneEndpoint", "host")
	port, _, _ := unstructured.NestedInt64(cluster.Object, "spec", "controlPlaneEndpoint", "port")
	if host != "" {
		tmplCtx.InfraControlPlaneEndpoint = &renderer.ControlPlaneEndpoint{Host: host, Port: port}
	}

	r.event(claim, corev1.EventTypeNormal, "InfraProvisioned", "Cluster[infra] infrastructure is provisioned")
	setCondition(claim, clusterclaimv1alpha1.ConditionInfraProvisioned, metav1.ConditionTrue, "Provisioned", "Cluster[infra] infrastructure is provisioned")
	return Proceed, nil
}

// stepCertificateSetClient creates or updates the client CertificateSet (Step 6).
// Skipped when client.enabled is false.
func (r *ClusterClaimReconciler) stepCertificateSetClient(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, tmplCtx *renderer.TemplateContext) (StepResult, error) {
	if !claim.Spec.Client.Enabled {
		return Proceed, nil
	}

	if claim.Spec.CertificateSetTemplateRef.Client == nil {
		return Proceed, NewTerminalErrorf("client is enabled but certificateSetTemplateRef.client is not set")
	}

	if err := r.ensureResource(ctx, claim, claim.Spec.CertificateSetTemplateRef.Client.Name, naming.CertificateSetName(claim.Name, "client"), claim.Namespace, *tmplCtx); err != nil {
		return Proceed, err
	}
	if err := r.ensureResourceFinalizer(ctx, CertificateSetGVK, naming.CertificateSetName(claim.Name, "client"), claim.Namespace); err != nil {
		return Proceed, err
	}
	return Proceed, nil
}

// stepWaitInfraCPReady waits for Cluster[infra] controlPlaneInitialized and
// extracts replica counts into the template context (Step 7).
func (r *ClusterClaimReconciler) stepWaitInfraCPReady(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, tmplCtx *renderer.TemplateContext) (StepResult, error) {
	clusterName := naming.ClusterName(claim.Name, "infra")
	cluster, err := r.getResource(ctx, ClusterGVK, clusterName, claim.Namespace)
	if err != nil {
		return Proceed, fmt.Errorf("get Cluster[infra]: %w", err)
	}

	cpInitialized, _, _ := unstructured.NestedBool(cluster.Object, "status", "initialization", "controlPlaneInitialized")
	if !cpInitialized {
		setWaiting(claim, clusterclaimv1alpha1.ConditionInfraCPReady, "Waiting for Cluster[infra] control plane to be initialized")
		return Wait, nil
	}

	// Extract replica counts into template context.
	available, _, _ := unstructured.NestedInt64(cluster.Object, "status", "controlPlane", "availableReplicas")
	desired, _, _ := unstructured.NestedInt64(cluster.Object, "status", "controlPlane", "desiredReplicas")
	tmplCtx.InfraControlPlaneInitialized = true
	tmplCtx.InfraControlPlaneAvailableReplicas = int32(available)
	tmplCtx.InfraControlPlaneDesiredReplicas = int32(desired)

	r.event(claim, corev1.EventTypeNormal, "InfraCPReady", "Cluster[infra] control plane is initialized")
	setCondition(claim, clusterclaimv1alpha1.ConditionInfraCPReady, metav1.ConditionTrue, "Ready", "Cluster[infra] control plane is initialized")
	return Proceed, nil
}

// stepCcmCsrc creates or updates the CcmCsrc resource (Step 8).
func (r *ClusterClaimReconciler) stepCcmCsrc(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, tmplCtx *renderer.TemplateContext) (StepResult, error) {
	if err := r.ensureResource(ctx, claim, claim.Spec.CcmCsrTemplateRef.Name, naming.CcmCsrcName(claim.Name), claim.Namespace, *tmplCtx); err != nil {
		return Proceed, err
	}
	if err := r.ensureResourceFinalizer(ctx, CcmCsrcGVK, naming.CcmCsrcName(claim.Name), claim.Namespace); err != nil {
		return Proceed, err
	}

	r.event(claim, corev1.EventTypeNormal, "CreatedCcmCsrc", "CcmCsrc %s created", naming.CcmCsrcName(claim.Name))
	setCondition(claim, clusterclaimv1alpha1.ConditionCcmCsrcCreated, metav1.ConditionTrue, "Created", "CcmCsrc created successfully")
	return Proceed, nil
}

// stepRemoteConfigMaps renders and applies ConfigMaps in the remote infra cluster (Step 9).
// Skipped when configMapTemplateRef is not set.
func (r *ClusterClaimReconciler) stepRemoteConfigMaps(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, tmplCtx *renderer.TemplateContext) (StepResult, error) {
	if claim.Spec.ConfigMapTemplateRef == nil {
		return Proceed, nil
	}

	// Get kubeconfig Secret for the infra cluster.
	var kubeconfigSecret corev1.Secret
	secretName := naming.KubeconfigSecretName(claim.Name)
	if err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: claim.Namespace}, &kubeconfigSecret); err != nil {
		if apierrors.IsNotFound(err) {
			setWaiting(claim, clusterclaimv1alpha1.ConditionRemoteConfigApplied, "Waiting for kubeconfig Secret")
			return Wait, nil
		}
		return Proceed, fmt.Errorf("get kubeconfig secret: %w", err)
	}

	// Get remote client via ClusterManager.
	remoteClient, err := r.ClusterManager.GetClient(ctx, &kubeconfigSecret)
	if err != nil {
		setCondition(claim, clusterclaimv1alpha1.ConditionRemoteConfigApplied, metav1.ConditionFalse, "RemoteConnectionError", err.Error())
		return Proceed, fmt.Errorf("get remote client: %w", err)
	}

	remoteNamespace := r.remoteNamespaceForClaim(claim)

	// Render and apply configMapTemplateRef.infra as parameters-infra.
	if err := r.applyRemoteConfigMap(ctx, claim, remoteClient, claim.Spec.ConfigMapTemplateRef.Infra.Name, naming.ConfigMapName("infra"), remoteNamespace, *tmplCtx); err != nil {
		setCondition(claim, clusterclaimv1alpha1.ConditionRemoteConfigApplied, metav1.ConditionFalse, "ApplyError", err.Error())
		return Proceed, fmt.Errorf("apply parameters-infra: %w", err)
	}

	// If role contains "system", also apply as parameters-system.
	if strings.Contains(claim.Spec.Infra.Role, "system") {
		if err := r.applyRemoteConfigMap(ctx, claim, remoteClient, claim.Spec.ConfigMapTemplateRef.Infra.Name, naming.ConfigMapName("system"), remoteNamespace, *tmplCtx); err != nil {
			setCondition(claim, clusterclaimv1alpha1.ConditionRemoteConfigApplied, metav1.ConditionFalse, "ApplyError", err.Error())
			return Proceed, fmt.Errorf("apply parameters-system: %w", err)
		}
	}

	// If client.enabled and client template ref set, apply as parameters-client.
	if claim.Spec.Client.Enabled && claim.Spec.ConfigMapTemplateRef.Client != nil {
		if err := r.applyRemoteConfigMap(ctx, claim, remoteClient, claim.Spec.ConfigMapTemplateRef.Client.Name, naming.ConfigMapName("client"), remoteNamespace, *tmplCtx); err != nil {
			setCondition(claim, clusterclaimv1alpha1.ConditionRemoteConfigApplied, metav1.ConditionFalse, "ApplyError", err.Error())
			return Proceed, fmt.Errorf("apply parameters-client: %w", err)
		}
	}

	r.event(claim, corev1.EventTypeNormal, "AppliedRemoteConfigMaps", "Remote ConfigMaps applied to infra cluster")
	setCondition(claim, clusterclaimv1alpha1.ConditionRemoteConfigApplied, metav1.ConditionTrue, "Applied", "Remote ConfigMaps applied successfully")
	return Proceed, nil
}

// applyRemoteConfigMap renders a template and applies it as a ConfigMap in the remote cluster.
func (r *ClusterClaimReconciler) applyRemoteConfigMap(
	ctx context.Context,
	claim *clusterclaimv1alpha1.ClusterClaim,
	remoteClient client.Client,
	templateRefName, configMapName, namespace string,
	tmplCtx renderer.TemplateContext,
) error {
	// Fetch template (cluster-scoped).
	var tmpl clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate
	if err := r.Get(ctx, client.ObjectKey{Name: templateRefName}, &tmpl); err != nil {
		if apierrors.IsNotFound(err) {
			return NewTerminalError(fmt.Errorf("fetch template %q: %w", templateRefName, err))
		}
		return fmt.Errorf("fetch template %q: %w", templateRefName, err)
	}

	// Render template.
	rendered, err := renderer.Render(tmpl.Spec.Value, tmplCtx)
	if err != nil {
		return NewTerminalError(fmt.Errorf("render template %q: %w", templateRefName, err))
	}

	// Build ConfigMap with standard labels merged from rendered.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
	}

	renderedLabels := extractStringMap(rendered, "metadata", "labels")
	stdLabels := labels.StandardLabels(claim.Name, claim.Namespace)
	cm.Labels = labels.MergeLabels(renderedLabels, stdLabels)

	// Extract annotations from rendered template.
	renderedAnnotations := extractStringMap(rendered, "metadata", "annotations")
	if len(renderedAnnotations) > 0 {
		cm.Annotations = renderedAnnotations
	}

	// Extract data from rendered template.
	if data, ok := rendered["data"]; ok {
		if dataMap, ok := data.(map[string]interface{}); ok {
			cm.Data = make(map[string]string, len(dataMap))
			for k, v := range dataMap {
				cm.Data[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	// Create or Update in remote cluster (no ownerReferences for cross-cluster resources).
	existing := &corev1.ConfigMap{}
	err = remoteClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, existing)
	if apierrors.IsNotFound(err) {
		return remoteClient.Create(ctx, cm)
	}
	if err != nil {
		return fmt.Errorf("get existing remote ConfigMap: %w", err)
	}

	if reflect.DeepEqual(existing.Labels, cm.Labels) &&
		reflect.DeepEqual(existing.Annotations, cm.Annotations) &&
		reflect.DeepEqual(existing.Data, cm.Data) {
		return nil
	}

	existing.Labels = cm.Labels
	existing.Annotations = cm.Annotations
	existing.Data = cm.Data
	return remoteClient.Update(ctx, existing)
}
