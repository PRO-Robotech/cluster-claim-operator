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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/controller/remote"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/naming"
)

// ClusterClaimReconciler reconciles a ClusterClaim object.
type ClusterClaimReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	ClusterManager *remote.ClusterManager
	Recorder       record.EventRecorder

	// RemoteNamespace is the default namespace in remote clusters where ConfigMaps are applied.
	// Set via the --remote-namespace flag. Can be overridden per-claim via spec.remoteNamespace.
	RemoteNamespace string
}

// remoteNamespaceForClaim returns the effective remote namespace for a given claim.
// Per-claim spec.remoteNamespace takes priority over the controller-level flag.
func (r *ClusterClaimReconciler) remoteNamespaceForClaim(claim *clusterclaimv1alpha1.ClusterClaim) string {
	if claim.Spec.RemoteNamespace != "" {
		return claim.Spec.RemoteNamespace
	}
	return r.RemoteNamespace
}

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=clusterclaim.in-cloud.io,resources=clusterclaimobserveresourcetemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=argoproj.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=in-cloud.io,resources=certificatesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=controller.in-cloud.io,resources=ccmcsrcs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=clusterclaim.in-cloud.io,resources=clusterclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=clusterclaim.in-cloud.io,resources=clusterclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=clusterclaim.in-cloud.io,resources=clusterclaims/finalizers,verbs=update

func (r *ClusterClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var claim clusterclaimv1alpha1.ClusterClaim
	if err := r.Get(ctx, req.NamespacedName, &claim); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !claim.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &claim)
	}

	if isPaused(&claim) {
		return r.reconcilePaused(ctx, &claim)
	}

	if err := r.ensureFinalizer(ctx, &claim); err != nil {
		return ctrl.Result{}, err
	}

	return r.executePipeline(ctx, &claim)
}

func isPaused(claim *clusterclaimv1alpha1.ClusterClaim) bool {
	return claim.Annotations[clusterclaimv1alpha1.PausedAnnotation] == "true"
}

func (r *ClusterClaimReconciler) ensureFinalizer(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim) error {
	if controllerutil.AddFinalizer(claim, clusterclaimv1alpha1.ClusterClaimFinalizer) {
		return r.Update(ctx, claim)
	}
	return nil
}

func (r *ClusterClaimReconciler) reconcilePaused(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim) (ctrl.Result, error) {
	oldStatus := claim.Status.DeepCopy()
	setPhase(claim, clusterclaimv1alpha1.PhasePaused)
	setCondition(claim, clusterclaimv1alpha1.ConditionPaused, metav1.ConditionTrue, "Paused", "Reconciliation paused via annotation")
	if err := r.updateStatusIfChanged(ctx, claim, oldStatus); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ClusterClaimReconciler) reconcileDelete(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(claim, clusterclaimv1alpha1.ClusterClaimFinalizer) {
		return ctrl.Result{}, nil
	}

	oldStatus := claim.Status.DeepCopy()
	setPhase(claim, clusterclaimv1alpha1.PhaseDeleting)
	if err := r.updateStatusIfChanged(ctx, claim, oldStatus); err != nil {
		return ctrl.Result{}, err
	}

	r.event(claim, corev1.EventTypeNormal, "DeletingResources", "Starting deletion of managed resources")

	// 1. Delete remote ConfigMaps (ignore errors -- cluster may be unavailable).
	r.deleteRemoteConfigMaps(ctx, claim)

	// 2. CcmCsrc.
	if err := r.removeResourceFinalizer(ctx, CcmCsrcGVK, naming.CcmCsrcName(claim.Name), claim.Namespace); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.deleteResource(ctx, CcmCsrcGVK, naming.CcmCsrcName(claim.Name), claim.Namespace); err != nil {
		return ctrl.Result{}, err
	}

	// 3. Cluster[client]
	if claim.Spec.Client.Enabled {
		if err := r.removeResourceFinalizer(ctx, ClusterGVK, naming.ClusterName(claim.Name, "client"), claim.Namespace); err != nil {
			return ctrl.Result{}, err
		}
		deleted, err := r.deleteAndWait(ctx, ClusterGVK, naming.ClusterName(claim.Name, "client"), claim.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !deleted {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	// 4. Cluster[infra]
	if err := r.removeResourceFinalizer(ctx, ClusterGVK, naming.ClusterName(claim.Name, "infra"), claim.Namespace); err != nil {
		return ctrl.Result{}, err
	}
	deleted, err := r.deleteAndWait(ctx, ClusterGVK, naming.ClusterName(claim.Name, "infra"), claim.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !deleted {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// 5. CertificateSet[client].
	if claim.Spec.Client.Enabled {
		if err := r.removeResourceFinalizer(ctx, CertificateSetGVK, naming.CertificateSetName(claim.Name, "client"), claim.Namespace); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.deleteResource(ctx, CertificateSetGVK, naming.CertificateSetName(claim.Name, "client"), claim.Namespace); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 6. CertificateSet[infra].
	if err := r.removeResourceFinalizer(ctx, CertificateSetGVK, naming.CertificateSetName(claim.Name, "infra"), claim.Namespace); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.deleteResource(ctx, CertificateSetGVK, naming.CertificateSetName(claim.Name, "infra"), claim.Namespace); err != nil {
		return ctrl.Result{}, err
	}

	// 7. Application
	if err := r.removeResourceFinalizer(ctx, ApplicationGVK, naming.ApplicationName(claim.Name), claim.Namespace); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.deleteResource(ctx, ApplicationGVK, naming.ApplicationName(claim.Name), claim.Namespace); err != nil {
		return ctrl.Result{}, err
	}

	// 8. Remove finalizer.
	logger.Info("deletion complete, removing finalizer")
	r.event(claim, corev1.EventTypeNormal, "DeletionComplete", "All managed resources deleted")
	controllerutil.RemoveFinalizer(claim, clusterclaimv1alpha1.ClusterClaimFinalizer)
	return ctrl.Result{}, r.Update(ctx, claim)
}

// deleteResource deletes an unstructured resource by GVK, name, and namespace.
// NotFound errors are ignored (resource already gone).
func (r *ClusterClaimReconciler) deleteResource(ctx context.Context, gvk schema.GroupVersionKind, name, namespace string) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete %s %s/%s: %w", gvk.Kind, namespace, name, err)
	}
	return nil
}

// deleteAndWait deletes a resource and returns true if it is gone.
// Returns false if the resource still exists (caller should requeue).
func (r *ClusterClaimReconciler) deleteAndWait(ctx context.Context, gvk schema.GroupVersionKind, name, namespace string) (bool, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj)
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("get %s %s/%s for deletion: %w", gvk.Kind, namespace, name, err)
	}
	if obj.GetDeletionTimestamp().IsZero() {
		if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("delete %s %s/%s: %w", gvk.Kind, namespace, name, err)
		}
	}
	return false, nil
}

// deleteRemoteConfigMaps attempts to delete remote ConfigMaps via the infra
// cluster kubeconfig. Errors are logged but do not block deletion.
func (r *ClusterClaimReconciler) deleteRemoteConfigMaps(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim) {
	logger := log.FromContext(ctx)

	if claim.Spec.ConfigMapTemplateRef == nil || r.ClusterManager == nil {
		return
	}

	var kubeconfigSecret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Name: naming.KubeconfigSecretName(claim.Name), Namespace: claim.Namespace}, &kubeconfigSecret); err != nil {
		logger.Info("kubeconfig secret not found during deletion, skipping remote cleanup", "error", err)
		return
	}

	remoteClient, err := r.ClusterManager.GetClient(ctx, &kubeconfigSecret)
	if err != nil {
		logger.Info("failed to get remote client during deletion, skipping remote cleanup", "error", err)
		return
	}

	ns := r.remoteNamespaceForClaim(claim)
	for _, cmName := range []string{
		naming.ConfigMapName("infra"),
		naming.ConfigMapName("system"),
		naming.ConfigMapName("client")} {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: ns},
		}
		if err := remoteClient.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
			logger.Info("failed to delete remote ConfigMap", "name", cmName, "error", err)
		}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := SetupIndexers(mgr); err != nil {
		return fmt.Errorf("setup indexers: %w", err)
	}

	appObj := &unstructured.Unstructured{}
	appObj.SetGroupVersionKind(ApplicationGVK)

	certSetObj := &unstructured.Unstructured{}
	certSetObj.SetGroupVersionKind(CertificateSetGVK)

	clusterObj := &unstructured.Unstructured{}
	clusterObj.SetGroupVersionKind(ClusterGVK)

	ccmObj := &unstructured.Unstructured{}
	ccmObj.SetGroupVersionKind(CcmCsrcGVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterclaimv1alpha1.ClusterClaim{},
			builder.WithPredicates(predicate.Or(
				predicate.GenerationChangedPredicate{},
				predicate.AnnotationChangedPredicate{},
			))).
		Owns(appObj).
		Owns(certSetObj).
		Owns(clusterObj).
		Owns(ccmObj).
		Watches(&clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate{},
			handler.EnqueueRequestsFromMapFunc(r.findClaimsForTemplate)).
		Watches(&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findClaimForSecret),
			builder.WithPredicates(kubeconfigSecretPredicate())).
		WithOptions(controller.Options{MaxConcurrentReconciles: 5}).
		Named("clusterclaim").
		Complete(r)
}

// kubeconfigSecretPredicate filters for Secrets matching the kubeconfig naming pattern.
func kubeconfigSecretPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return strings.HasSuffix(obj.GetName(), "-infra-kubeconfig")
	})
}
