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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// isConditionTrue checks if a condition of the given type has status "True"
// on an unstructured object. It parses the status.conditions array.
func isConditionTrue(obj *unstructured.Unstructured, condType string) bool {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == condType && cond["status"] == "True" {
			return true
		}
	}
	return false
}

// getResource fetches an unstructured resource by GVK, name, and namespace.
func (r *ClusterClaimReconciler) getResource(ctx context.Context, gvk schema.GroupVersionKind, name, namespace string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	if err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj); err != nil {
		return nil, fmt.Errorf("get %s %s/%s: %w", gvk.Kind, namespace, name, err)
	}
	return obj, nil
}
