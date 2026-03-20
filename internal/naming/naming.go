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

package naming

// ApplicationName returns the name for the ArgoCD Application.
func ApplicationName(claimName string) string {
	return claimName
}

// CertificateSetName returns the name for a CertificateSet.
// role is "infra" or "client".
func CertificateSetName(claimName, role string) string {
	return claimName + "-" + role
}

// ClusterName returns the name for a CAPI Cluster.
// role is "infra" or "client".
func ClusterName(claimName, role string) string {
	return claimName + "-" + role
}

// CcmCsrcName returns the name for the CcmCsrc resource.
func CcmCsrcName(claimName string) string {
	return claimName
}

// ConfigMapName returns the name for a remote ConfigMap.
// configType is "infra", "system", or "client".
func ConfigMapName(configType string) string {
	return "parameters-" + configType
}

// KubeconfigSecretName returns the name of the kubeconfig Secret
// for the infra cluster.
func KubeconfigSecretName(claimName string) string {
	return claimName + "-infra-kubeconfig"
}
