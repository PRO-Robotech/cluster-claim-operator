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
	"testing"

	"github.com/stretchr/testify/assert"

	v1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
)

func TestStandardLabels(t *testing.T) {
	tests := []struct {
		name           string
		claimName      string
		claimNamespace string
		want           map[string]string
	}{
		{
			name:           "typical values",
			claimName:      "ec8a00",
			claimNamespace: "dlputi1u",
			want: map[string]string{
				v1alpha1.LabelClaimName:      "ec8a00",
				v1alpha1.LabelClaimNamespace: "dlputi1u",
			},
		},
		{
			name:           "empty values",
			claimName:      "",
			claimNamespace: "",
			want: map[string]string{
				v1alpha1.LabelClaimName:      "",
				v1alpha1.LabelClaimNamespace: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StandardLabels(tt.claimName, tt.claimNamespace)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMergeLabels(t *testing.T) {
	tests := []struct {
		name     string
		rendered map[string]string
		standard map[string]string
		want     map[string]string
	}{
		{
			name:     "nil rendered map",
			rendered: nil,
			standard: map[string]string{
				v1alpha1.LabelClaimName: "claim1",
			},
			want: map[string]string{
				v1alpha1.LabelClaimName: "claim1",
			},
		},
		{
			name:     "empty rendered map",
			rendered: map[string]string{},
			standard: map[string]string{
				v1alpha1.LabelClaimName: "claim1",
			},
			want: map[string]string{
				v1alpha1.LabelClaimName: "claim1",
			},
		},
		{
			name: "non-overlapping merge",
			rendered: map[string]string{
				"app.kubernetes.io/name":    "my-app",
				"app.kubernetes.io/version": "v1",
			},
			standard: map[string]string{
				v1alpha1.LabelClaimName:      "ec8a00",
				v1alpha1.LabelClaimNamespace: "dlputi1u",
			},
			want: map[string]string{
				"app.kubernetes.io/name":     "my-app",
				"app.kubernetes.io/version":  "v1",
				v1alpha1.LabelClaimName:      "ec8a00",
				v1alpha1.LabelClaimNamespace: "dlputi1u",
			},
		},
		{
			name: "conflicting keys - standard wins",
			rendered: map[string]string{
				v1alpha1.LabelClaimName: "rendered-value",
				"custom-label":          "keep-this",
			},
			standard: map[string]string{
				v1alpha1.LabelClaimName:      "standard-value",
				v1alpha1.LabelClaimNamespace: "ns",
			},
			want: map[string]string{
				v1alpha1.LabelClaimName:      "standard-value",
				v1alpha1.LabelClaimNamespace: "ns",
				"custom-label":               "keep-this",
			},
		},
		{
			name:     "both nil",
			rendered: nil,
			standard: nil,
			want:     map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeLabels(tt.rendered, tt.standard)
			assert.Equal(t, tt.want, got)
		})
	}
}
