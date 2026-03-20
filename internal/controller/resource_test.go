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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestResourceNeedsUpdate(t *testing.T) {
	baseLabels := map[string]string{"app": "test", "env": "prod"}
	baseAnnotations := map[string]string{"note": "value"}
	baseOwnerRefs := []metav1.OwnerReference{
		{APIVersion: "v1", Kind: "ClusterClaim", Name: "claim1", UID: "uid1"},
	}
	baseSpec := map[string]interface{}{
		"replicas": int64(3),
		"paused":   false,
	}

	makeObj := func(labels map[string]string, annotations map[string]string, ownerRefs []metav1.OwnerReference, spec map[string]interface{}) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{Object: make(map[string]interface{})}
		obj.SetLabels(labels)
		obj.SetAnnotations(annotations)
		obj.SetOwnerReferences(ownerRefs)
		if spec != nil {
			_ = unstructured.SetNestedField(obj.Object, spec, "spec")
		}
		return obj
	}

	tests := []struct {
		name     string
		existing *unstructured.Unstructured
		desired  *unstructured.Unstructured
		rendered map[string]interface{}
		want     bool
	}{
		{
			name:     "identical — no update needed",
			existing: makeObj(baseLabels, baseAnnotations, baseOwnerRefs, baseSpec),
			desired:  makeObj(baseLabels, baseAnnotations, baseOwnerRefs, baseSpec),
			rendered: map[string]interface{}{"spec": baseSpec},
			want:     false,
		},
		{
			name:     "labels differ",
			existing: makeObj(map[string]string{"app": "test"}, baseAnnotations, baseOwnerRefs, baseSpec),
			desired:  makeObj(baseLabels, baseAnnotations, baseOwnerRefs, baseSpec),
			rendered: map[string]interface{}{"spec": baseSpec},
			want:     true,
		},
		{
			name:     "annotations differ",
			existing: makeObj(baseLabels, map[string]string{"note": "old"}, baseOwnerRefs, baseSpec),
			desired:  makeObj(baseLabels, baseAnnotations, baseOwnerRefs, baseSpec),
			rendered: map[string]interface{}{"spec": baseSpec},
			want:     true,
		},
		{
			name: "ownerReferences differ",
			existing: makeObj(baseLabels, baseAnnotations, []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "ClusterClaim", Name: "other", UID: "uid2"},
			}, baseSpec),
			desired:  makeObj(baseLabels, baseAnnotations, baseOwnerRefs, baseSpec),
			rendered: map[string]interface{}{"spec": baseSpec},
			want:     true,
		},
		{
			name:     "spec differs",
			existing: makeObj(baseLabels, baseAnnotations, baseOwnerRefs, map[string]interface{}{"replicas": int64(1), "paused": false}),
			desired:  makeObj(baseLabels, baseAnnotations, baseOwnerRefs, baseSpec),
			rendered: map[string]interface{}{"spec": baseSpec},
			want:     true,
		},
		{
			name: "data differs",
			existing: func() *unstructured.Unstructured {
				obj := makeObj(baseLabels, baseAnnotations, baseOwnerRefs, nil)
				_ = unstructured.SetNestedField(obj.Object, map[string]interface{}{"key": "old"}, "data")
				return obj
			}(),
			desired:  makeObj(baseLabels, baseAnnotations, baseOwnerRefs, nil),
			rendered: map[string]interface{}{"data": map[string]interface{}{"key": "new"}},
			want:     true,
		},
		{
			name: "data identical",
			existing: func() *unstructured.Unstructured {
				obj := makeObj(baseLabels, baseAnnotations, baseOwnerRefs, nil)
				_ = unstructured.SetNestedField(obj.Object, map[string]interface{}{"key": "same"}, "data")
				return obj
			}(),
			desired:  makeObj(baseLabels, baseAnnotations, baseOwnerRefs, nil),
			rendered: map[string]interface{}{"data": map[string]interface{}{"key": "same"}},
			want:     false,
		},
		{
			name:     "no spec or data in rendered — no update",
			existing: makeObj(baseLabels, baseAnnotations, baseOwnerRefs, baseSpec),
			desired:  makeObj(baseLabels, baseAnnotations, baseOwnerRefs, baseSpec),
			rendered: map[string]interface{}{},
			want:     false,
		},
		{
			name:     "nil annotations both — no update",
			existing: makeObj(baseLabels, nil, baseOwnerRefs, baseSpec),
			desired:  makeObj(baseLabels, nil, baseOwnerRefs, baseSpec),
			rendered: map[string]interface{}{"spec": baseSpec},
			want:     false,
		},
		{
			name: "existing spec has extra keys from external controller — no update",
			existing: makeObj(baseLabels, baseAnnotations, baseOwnerRefs,
				map[string]interface{}{
					"replicas":             int64(3),
					"paused":               false,
					"controlPlaneEndpoint": map[string]interface{}{"host": "10.0.0.1", "port": int64(6443)},
				}),
			desired:  makeObj(baseLabels, baseAnnotations, baseOwnerRefs, baseSpec),
			rendered: map[string]interface{}{"spec": baseSpec},
			want:     false,
		},
		{
			name: "existing spec has extra keys but rendered key differs — update needed",
			existing: makeObj(baseLabels, baseAnnotations, baseOwnerRefs,
				map[string]interface{}{
					"replicas":             int64(1),
					"paused":               false,
					"controlPlaneEndpoint": map[string]interface{}{"host": "10.0.0.1"},
				}),
			desired:  makeObj(baseLabels, baseAnnotations, baseOwnerRefs, baseSpec),
			rendered: map[string]interface{}{"spec": baseSpec},
			want:     true,
		},
		{
			name:     "nil vs empty annotations — update needed",
			existing: makeObj(baseLabels, nil, baseOwnerRefs, baseSpec),
			desired:  makeObj(baseLabels, map[string]string{}, baseOwnerRefs, baseSpec),
			rendered: map[string]interface{}{"spec": baseSpec},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resourceNeedsUpdate(tt.existing, tt.desired, tt.rendered)
			if got != tt.want {
				t.Errorf("resourceNeedsUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
