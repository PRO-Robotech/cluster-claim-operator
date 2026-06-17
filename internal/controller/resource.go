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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/labels"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/renderer"
)

const fieldManager = "cluster-claim-operator"

// ensureResource renders templateRefName and applies the result via server-side apply.
func (r *ClusterClaimReconciler) ensureResource(
	ctx context.Context,
	claim *clusterclaimv1alpha1.ClusterClaim,
	templateRefName string,
	resourceName string,
	namespace string,
	tmplCtx renderer.TemplateContext,
) error {
	var tmpl clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate
	if err := r.Get(ctx, client.ObjectKey{Name: templateRefName}, &tmpl); err != nil {
		if apierrors.IsNotFound(err) {
			return NewTerminalError(fmt.Errorf("fetch template %q: %w", templateRefName, err))
		}
		return fmt.Errorf("fetch template %q: %w", templateRefName, err)
	}

	rendered, err := renderer.Render(tmpl.Spec.Value, tmplCtx)
	if err != nil {
		return NewTerminalError(fmt.Errorf("render template %q: %w", templateRefName, err))
	}

	desired := buildDesiredResource(claim, &tmpl, rendered, resourceName, namespace)

	if err := r.Patch(ctx, desired, client.Apply, client.FieldOwner(fieldManager), client.ForceOwnership); err != nil {
		return classifyAPIError(fmt.Errorf("apply resource %s/%s: %w", namespace, resourceName, err))
	}
	return nil
}

// buildDesiredResource creates the full unstructured object from template + rendered output.
func buildDesiredResource(
	claim *clusterclaimv1alpha1.ClusterClaim,
	tmpl *clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate,
	rendered map[string]interface{},
	resourceName, namespace string,
) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: make(map[string]interface{})}

	// Set GVK.
	gvk := schema.FromAPIVersionAndKind(tmpl.Spec.APIVersion, tmpl.Spec.Kind)
	obj.SetGroupVersionKind(gvk)

	// Set name and namespace.
	obj.SetName(resourceName)
	obj.SetNamespace(namespace)

	// Set ownerReference to ClusterClaim.
	obj.SetOwnerReferences([]metav1.OwnerReference{
		*metav1.NewControllerRef(claim, clusterclaimv1alpha1.GroupVersion.WithKind("ClusterClaim")),
	})

	// Extract and merge labels.
	renderedLabels := extractStringMap(rendered, "metadata", "labels")
	stdLabels := labels.StandardLabels(claim.Name, claim.Namespace)
	obj.SetLabels(labels.MergeLabels(renderedLabels, stdLabels))

	// Extract and set annotations.
	renderedAnnotations := extractStringMap(rendered, "metadata", "annotations")
	if len(renderedAnnotations) > 0 {
		obj.SetAnnotations(renderedAnnotations)
	}

	// Set spec from rendered.
	if spec, ok := rendered["spec"]; ok {
		_ = unstructured.SetNestedField(obj.Object, spec, "spec")
	}

	// Set data from rendered (for ConfigMaps).
	if data, ok := rendered["data"]; ok {
		_ = unstructured.SetNestedField(obj.Object, data, "data")
	}

	return obj
}

// extractStringMap extracts a map[string]string from a nested path in the rendered output.
func extractStringMap(rendered map[string]interface{}, fields ...string) map[string]string {
	val, found, err := unstructured.NestedStringMap(rendered, fields...)
	if err != nil || !found {
		// Try as map[string]interface{} and convert.
		raw, found2, err2 := unstructured.NestedMap(rendered, fields...)
		if err2 != nil || !found2 {
			return nil
		}
		result := make(map[string]string, len(raw))
		for k, v := range raw {
			result[k] = fmt.Sprintf("%v", v)
		}
		return result
	}
	return val
}
