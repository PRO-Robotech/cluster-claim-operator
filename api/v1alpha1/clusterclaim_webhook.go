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

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-clusterclaim-in-cloud-io-v1alpha1-clusterclaim,mutating=false,failurePolicy=fail,sideEffects=None,groups=clusterclaim.in-cloud.io,resources=clusterclaims,verbs=create;update,versions=v1alpha1,name=vclusterclaim.kb.io,admissionReviewVersions=v1

// SetupClusterClaimWebhookWithManager registers the webhook with the manager.
func SetupClusterClaimWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&ClusterClaim{}).
		WithValidator(&clusterClaimValidator{}).
		Complete()
}

type clusterClaimValidator struct{}

var _ webhook.CustomValidator = &clusterClaimValidator{}

func (v *clusterClaimValidator) ValidateCreate(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *clusterClaimValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldClaim := oldObj.(*ClusterClaim)
	newClaim := newObj.(*ClusterClaim)

	if oldClaim.Spec.Client.Enabled != newClaim.Spec.Client.Enabled {
		return nil, fmt.Errorf("field spec.client.enabled is immutable")
	}
	return nil, nil
}

func (v *clusterClaimValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
