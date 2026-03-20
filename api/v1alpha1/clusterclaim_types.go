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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Phase constants for ClusterClaim lifecycle.
const (
	PhaseProvisioning      = "Provisioning"
	PhaseWaitingDependency = "WaitingDependency"
	PhaseReady             = "Ready"
	PhaseFailed            = "Failed"
	PhaseDegraded          = "Degraded"
	PhasePaused            = "Paused"
	PhaseDeleting          = "Deleting"
)

// Condition type constants for ClusterClaim.
const (
	ConditionReady                 = "Ready"
	ConditionApplicationCreated    = "ApplicationCreated"
	ConditionInfraCertificateReady = "InfraCertificateReady"
	ConditionInfraProvisioned      = "InfraProvisioned"
	ConditionInfraCPReady          = "InfraCPReady"
	ConditionCcmCsrcCreated        = "CcmCsrcCreated"
	ConditionRemoteConfigApplied   = "RemoteConfigApplied"
	ConditionClientCPReady         = "ClientCPReady"
	ConditionPaused                = "Paused"
)

// Finalizer for cleanup on deletion.
const ClusterClaimFinalizer = "clusterclaim.in-cloud.io/finalizer"

// Annotation for pausing reconciliation.
const PausedAnnotation = "clusterclaim.in-cloud.io/paused"

// Label constants for managed resources.
const (
	LabelClaimName      = "clusterclaim.in-cloud.io/claim-name"
	LabelClaimNamespace = "clusterclaim.in-cloud.io/claim-namespace"
)

// TemplateRef is a reference to a cluster-scoped ClusterClaimObserveResourceTemplate.
type TemplateRef struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// DualTemplateRef holds infra and optional client template references.
type DualTemplateRef struct {
	Infra TemplateRef `json:"infra"`
	// +optional
	Client *TemplateRef `json:"client,omitempty"`
}

// ConfigurationSpec defines compute resource configuration for control plane nodes.
type ConfigurationSpec struct {
	// +kubebuilder:validation:Minimum=1
	CpuCount int32 `json:"cpuCount"`
	// +kubebuilder:validation:Minimum=1
	DiskSize int32 `json:"diskSize"`
	// +kubebuilder:validation:Minimum=1
	Memory int32 `json:"memory"`
}

// NetworkConfig defines network settings for a cluster.
type NetworkConfig struct {
	// +kubebuilder:validation:MinLength=1
	ServiceCidr string `json:"serviceCidr"`
	// +kubebuilder:validation:MinLength=1
	PodCidr string `json:"podCidr"`
	// +optional
	PodCidrMaskSize *int32 `json:"podCidrMaskSize,omitempty"`
	// +kubebuilder:validation:MinLength=1
	ClusterDNS string `json:"clusterDNS"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	KubeApiserverPort int32 `json:"kubeApiserverPort"`
}

// ComponentVersion defines a version for a cluster component.
type ComponentVersion struct {
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`
}

// InfraSpec defines the infra cluster configuration.
type InfraSpec struct {
	// +kubebuilder:validation:MinLength=1
	Role   string `json:"role"`
	Paused bool   `json:"paused"`

	Network           NetworkConfig               `json:"network"`
	ComponentVersions map[string]ComponentVersion `json:"componentVersions"`
}

// ClientSpec defines the client cluster configuration.
type ClientSpec struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="client.enabled is immutable"
	Enabled bool `json:"enabled"`

	// +optional
	Paused *bool `json:"paused,omitempty"`
	// +optional
	Network *NetworkConfig `json:"network,omitempty"`
	// +optional
	ComponentVersions map[string]ComponentVersion `json:"componentVersions,omitempty"`
}

// ClusterClaimSpec defines the desired state of ClusterClaim.
type ClusterClaimSpec struct {
	// Template references.
	ObserveTemplateRef        TemplateRef     `json:"observeTemplateRef"`
	CertificateSetTemplateRef DualTemplateRef `json:"certificateSetTemplateRef"`
	ClusterTemplateRef        DualTemplateRef `json:"clusterTemplateRef"`
	CcmCsrTemplateRef         TemplateRef     `json:"ccmCsrTemplateRef"`
	// +optional
	ConfigMapTemplateRef *DualTemplateRef `json:"configMapTemplateRef,omitempty"`

	// Cluster parameters.

	// +kubebuilder:validation:Minimum=1
	Replicas      int32             `json:"replicas"`
	Configuration ConfigurationSpec `json:"configuration"`

	// +optional
	RemoteNamespace string `json:"remoteNamespace,omitempty"`

	// +optional
	ExtraEnvs map[string]apiextensionsv1.JSON `json:"extraEnvs,omitempty"`

	Infra  InfraSpec  `json:"infra"`
	Client ClientSpec `json:"client"`
}

// ClusterClaimStatus defines the observed state of ClusterClaim.
type ClusterClaimStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +kubebuilder:validation:Enum=Provisioning;WaitingDependency;Ready;Failed;Degraded;Paused;Deleting
	// +optional
	Phase string `json:"phase,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterClaim is the Schema for the clusterclaims API.
type ClusterClaim struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec ClusterClaimSpec `json:"spec"`

	// +optional
	Status ClusterClaimStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ClusterClaimList contains a list of ClusterClaim.
type ClusterClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ClusterClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterClaim{}, &ClusterClaimList{})
}
