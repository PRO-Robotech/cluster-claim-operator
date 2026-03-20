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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterClaimObserveResourceTemplateSpec defines the desired state of ClusterClaimObserveResourceTemplate.
type ClusterClaimObserveResourceTemplateSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="apiVersion is immutable"
	APIVersion string `json:"apiVersion"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="kind is immutable"
	Kind string `json:"kind"`

	// +kubebuilder:validation:MinLength=1
	Value string `json:"value"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="APIVersion",type="string",JSONPath=".spec.apiVersion"
// +kubebuilder:printcolumn:name="Kind",type="string",JSONPath=".spec.kind"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterClaimObserveResourceTemplate is the Schema for the clusterclaimobserveresourcetemplates API.
type ClusterClaimObserveResourceTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec ClusterClaimObserveResourceTemplateSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// ClusterClaimObserveResourceTemplateList contains a list of ClusterClaimObserveResourceTemplate.
type ClusterClaimObserveResourceTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ClusterClaimObserveResourceTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterClaimObserveResourceTemplate{}, &ClusterClaimObserveResourceTemplateList{})
}
