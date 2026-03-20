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

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
