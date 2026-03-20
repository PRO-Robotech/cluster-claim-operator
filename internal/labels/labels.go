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

package labels

import (
	v1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
)

// StandardLabels returns the standard labels applied to all managed resources.
func StandardLabels(claimName, claimNamespace string) map[string]string {
	return map[string]string{
		v1alpha1.LabelClaimName:      claimName,
		v1alpha1.LabelClaimNamespace: claimNamespace,
	}
}

// MergeLabels merges rendered labels from a template with standard labels.
// Standard labels take precedence over rendered labels.
func MergeLabels(rendered, standard map[string]string) map[string]string {
	result := make(map[string]string, len(rendered)+len(standard))
	for k, v := range rendered {
		result[k] = v
	}
	for k, v := range standard {
		result[k] = v
	}
	return result
}
