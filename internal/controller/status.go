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
	"time"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
)

// setPhase updates the claim's phase and observedGeneration.
func setPhase(claim *clusterclaimv1alpha1.ClusterClaim, phase string) {
	claim.Status.Phase = phase
	claim.Status.ObservedGeneration = claim.Generation
}

// setCondition sets a condition on the claim's status.
func setCondition(claim *clusterclaimv1alpha1.ClusterClaim, condType string, status metav1.ConditionStatus, reason, message string) {
	apimeta.SetStatusCondition(&claim.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: claim.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// setReady sets the claim to Ready phase with Ready=True condition.
func setReady(claim *clusterclaimv1alpha1.ClusterClaim) {
	setPhase(claim, clusterclaimv1alpha1.PhaseReady)
	setCondition(claim, clusterclaimv1alpha1.ConditionReady, metav1.ConditionTrue, "AllStepsComplete", "All pipeline steps completed successfully")
}

// setFailed sets the claim to Failed phase with Ready=False condition.
func setFailed(claim *clusterclaimv1alpha1.ClusterClaim, reason string, err error) {
	setPhase(claim, clusterclaimv1alpha1.PhaseFailed)
	setCondition(claim, clusterclaimv1alpha1.ConditionReady, metav1.ConditionFalse, reason, err.Error())
}

// setWaiting sets the claim to WaitingDependency phase with a descriptive condition.
func setWaiting(claim *clusterclaimv1alpha1.ClusterClaim, condType, message string) {
	setPhase(claim, clusterclaimv1alpha1.PhaseWaitingDependency)
	setCondition(claim, condType, metav1.ConditionFalse, "WaitingDependency", message)
}

// syncClusterStatuses mirrors Cluster[infra] and Cluster[client] status fields
// into ClusterClaim.Status.Clusters. Called at the start of every pipeline execution.
// Errors are logged as warnings but never stop the pipeline.
func (r *ClusterClaimReconciler) syncClusterStatuses(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, infraCluster, clientCluster *unstructured.Unstructured) {
	if infraCluster == nil && clientCluster == nil {
		return
	}

	logger := log.FromContext(ctx)

	if claim.Status.Clusters == nil {
		claim.Status.Clusters = &clusterclaimv1alpha1.ClustersStatus{}
	}

	if infraCluster != nil {
		claim.Status.Clusters.Infra = &clusterclaimv1alpha1.InfraClusterStatusSummary{
			ClusterStatusSummary: *extractClusterStatusSummary(ctx, infraCluster),
			ControlPlaneVersion:  r.fetchControlPlaneVersion(ctx, infraCluster),
		}
		logger.V(1).Info("mirrored Cluster[infra] status", "phase", claim.Status.Clusters.Infra.Phase)
	}
	if clientCluster != nil {
		claim.Status.Clusters.Client = extractClusterStatusSummary(ctx, clientCluster)
		logger.V(1).Info("mirrored Cluster[client] status", "phase", claim.Status.Clusters.Client.Phase)
	}
}

// fetchControlPlaneVersion follows spec.controlPlaneRef from the infra CAPI Cluster to the
// KubeadmControlPlane and extracts spec.version and status.version. Returns nil if the reference
// is missing, the KCP cannot be fetched, or neither version is set.
func (r *ClusterClaimReconciler) fetchControlPlaneVersion(ctx context.Context, cluster *unstructured.Unstructured) *clusterclaimv1alpha1.ControlPlaneVersion {
	logger := log.FromContext(ctx)
	clusterName := cluster.GetName()

	cpRefName, _, err := unstructured.NestedString(cluster.Object, "spec", "controlPlaneRef", "name")
	if err != nil || cpRefName == "" {
		return nil
	}

	cpRefNamespace, _, _ := unstructured.NestedString(cluster.Object, "spec", "controlPlaneRef", "namespace")
	if cpRefNamespace == "" {
		cpRefNamespace = cluster.GetNamespace()
	}

	kcp := &unstructured.Unstructured{}
	kcp.SetGroupVersionKind(KubeadmControlPlaneGVK)
	if err := r.Get(ctx, client.ObjectKey{Name: cpRefName, Namespace: cpRefNamespace}, kcp); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "failed to fetch KubeadmControlPlane", "cluster", clusterName, "kcp", cpRefName)
		}
		return nil
	}

	specVersion, _, _ := unstructured.NestedString(kcp.Object, "spec", "version")
	statusVersion, _, _ := unstructured.NestedString(kcp.Object, "status", "version")
	if specVersion == "" && statusVersion == "" {
		return nil
	}

	return &clusterclaimv1alpha1.ControlPlaneVersion{
		SpecVersion:   specVersion,
		StatusVersion: statusVersion,
	}
}

// extractClusterStatusSummary reads status fields from an unstructured CAPI Cluster.
// Parse errors are logged as warnings; the corresponding field is skipped.
func extractClusterStatusSummary(ctx context.Context, cluster *unstructured.Unstructured) *clusterclaimv1alpha1.ClusterStatusSummary {
	logger := log.FromContext(ctx)
	clusterName := cluster.GetName()
	summary := &clusterclaimv1alpha1.ClusterStatusSummary{}

	host, _, err := unstructured.NestedString(cluster.Object, "spec", "controlPlaneEndpoint", "host")
	if err != nil {
		logger.Error(err, "failed to read spec.controlPlaneEndpoint.host from Cluster", "cluster", clusterName)
	}
	summary.Host = host

	port, _, err := unstructured.NestedInt64(cluster.Object, "spec", "controlPlaneEndpoint", "port")
	if err != nil {
		logger.Error(err, "failed to read spec.controlPlaneEndpoint.port from Cluster", "cluster", clusterName)
	}
	summary.Port = int32(port)

	phase, _, err := unstructured.NestedString(cluster.Object, "status", "phase")
	if err != nil {
		logger.Error(err, "failed to read status.phase from Cluster", "cluster", clusterName)
	}
	summary.Phase = phase

	// Conditions.
	conditions, found, err := unstructured.NestedSlice(cluster.Object, "status", "conditions")
	if err != nil {
		logger.Error(err, "failed to read status.conditions from Cluster", "cluster", clusterName)
	}
	if found {
		for i, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				logger.Error(nil, "condition entry is not a map, skipping", "cluster", clusterName, "index", i)
				continue
			}
			mc := metav1.Condition{}
			mc.Type, _, _ = unstructured.NestedString(cond, "type")
			if mc.Type == "" {
				continue
			}
			statusStr, _, _ := unstructured.NestedString(cond, "status")
			mc.Status = metav1.ConditionStatus(statusStr)
			mc.Reason, _, _ = unstructured.NestedString(cond, "reason")
			mc.Message, _, _ = unstructured.NestedString(cond, "message")
			obsGen, _, _ := unstructured.NestedInt64(cond, "observedGeneration")
			mc.ObservedGeneration = obsGen
			lastTransStr, _, _ := unstructured.NestedString(cond, "lastTransitionTime")
			if lastTransStr != "" {
				t, parseErr := time.Parse(time.RFC3339, lastTransStr)
				if parseErr != nil {
					logger.Error(parseErr, "failed to parse lastTransitionTime", "cluster", clusterName, "condition", mc.Type, "value", lastTransStr)
				} else {
					mc.LastTransitionTime = metav1.NewTime(t)
				}
			}
			summary.Conditions = append(summary.Conditions, mc)
		}
	}

	// ControlPlane replicas.
	cpMap, cpFound, err := unstructured.NestedMap(cluster.Object, "status", "controlPlane")
	if err != nil {
		logger.Error(err, "failed to read status.controlPlane from Cluster", "cluster", clusterName)
	}
	if cpFound && len(cpMap) > 0 {
		summary.ControlPlane = extractReplicaStatus(cpMap)
	}

	// Workers replicas.
	wMap, wFound, err := unstructured.NestedMap(cluster.Object, "status", "workers")
	if err != nil {
		logger.Error(err, "failed to read status.workers from Cluster", "cluster", clusterName)
	}
	if wFound && len(wMap) > 0 {
		summary.Workers = extractReplicaStatus(wMap)
	}

	return summary
}

// extractReplicaStatus converts an unstructured replica status map to a typed ReplicaStatus.
func extractReplicaStatus(m map[string]interface{}) *clusterclaimv1alpha1.ReplicaStatus {
	rs := &clusterclaimv1alpha1.ReplicaStatus{
		Replicas:          toInt32(m["replicas"]),
		ReadyReplicas:     toInt32(m["readyReplicas"]),
		AvailableReplicas: toInt32(m["availableReplicas"]),
		DesiredReplicas:   toInt32(m["desiredReplicas"]),
		UpToDateReplicas:  toInt32(m["upToDateReplicas"]),
	}
	return rs
}

// toInt32 converts an interface{} (int64 or float64 from JSON) to int32.
func toInt32(v interface{}) int32 {
	switch n := v.(type) {
	case int64:
		return int32(n)
	case float64:
		return int32(n)
	default:
		return 0
	}
}

// updateStatusIfChanged compares the current status with the snapshot taken before
// pipeline execution and only calls Status().Update() if something actually changed.
// This prevents no-op status updates from bumping resourceVersion and causing
// infinite reconcile loops through the For() watch.
func (r *ClusterClaimReconciler) updateStatusIfChanged(ctx context.Context, claim *clusterclaimv1alpha1.ClusterClaim, oldStatus *clusterclaimv1alpha1.ClusterClaimStatus) error {
	if apiequality.Semantic.DeepEqual(oldStatus, &claim.Status) {
		return nil
	}
	return r.Status().Update(ctx, claim)
}
