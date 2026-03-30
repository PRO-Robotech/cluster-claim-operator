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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/labels"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/renderer"
)

// ensureResource fetches a template, renders it, and creates or updates the target resource.
func (r *ClusterClaimReconciler) ensureResource(
	ctx context.Context,
	claim *clusterclaimv1alpha1.ClusterClaim,
	templateRefName string,
	resourceName string,
	namespace string,
	tmplCtx renderer.TemplateContext,
) error {
	logger := log.FromContext(ctx)

	// 1. Fetch template (cluster-scoped).
	var tmpl clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate
	if err := r.Get(ctx, client.ObjectKey{Name: templateRefName}, &tmpl); err != nil {
		if apierrors.IsNotFound(err) {
			return NewTerminalError(fmt.Errorf("fetch template %q: %w", templateRefName, err))
		}
		return fmt.Errorf("fetch template %q: %w", templateRefName, err)
	}

	// 2. Render template.
	rendered, err := renderer.Render(tmpl.Spec.Value, tmplCtx)
	if err != nil {
		return NewTerminalError(fmt.Errorf("render template %q: %w", templateRefName, err))
	}

	// 3. Build desired unstructured object.
	desired := buildDesiredResource(claim, &tmpl, rendered, resourceName, namespace)

	// 4. Create or Update.
	gvk := schema.FromAPIVersionAndKind(tmpl.Spec.APIVersion, tmpl.Spec.Kind)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	err = r.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: namespace}, existing)
	if apierrors.IsNotFound(err) {
		logger.Info("creating resource", "gvk", gvk, "name", resourceName, "namespace", namespace)
		if err := r.Create(ctx, desired); err != nil {
			return classifyAPIError(fmt.Errorf("create resource %s/%s: %w", namespace, resourceName, err))
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get existing resource: %w", err)
	}

	// Check if update is needed before calling the API.
	if !resourceNeedsUpdate(existing, desired, rendered) {
		return nil
	}

	// Update: apply desired state to existing object (preserves resourceVersion, uid, etc).
	applyDesiredToExisting(existing, desired, rendered)
	logger.Info("updating resource", "gvk", gvk, "name", resourceName, "namespace", namespace)
	if err := r.Update(ctx, existing); err != nil {
		return classifyAPIError(fmt.Errorf("update resource %s/%s: %w", namespace, resourceName, err))
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

// applyDesiredToExisting updates the existing resource with desired state.
// It merges rendered spec/data into existing rather than replacing, so that
// fields set by external controllers (e.g. controlPlaneEndpoint by CAPI) are preserved.
func applyDesiredToExisting(existing, desired *unstructured.Unstructured, rendered map[string]interface{}) {
	existing.SetLabels(desired.GetLabels())
	existing.SetAnnotations(desired.GetAnnotations())
	existing.SetOwnerReferences(desired.GetOwnerReferences())

	if spec, ok := rendered["spec"]; ok {
		if specMap, ok := spec.(map[string]interface{}); ok {
			mergeNestedMap(existing.Object, specMap, "spec")
		} else {
			_ = unstructured.SetNestedField(existing.Object, spec, "spec")
		}
	}
	if data, ok := rendered["data"]; ok {
		if dataMap, ok := data.(map[string]interface{}); ok {
			mergeNestedMap(existing.Object, dataMap, "data")
		} else {
			_ = unstructured.SetNestedField(existing.Object, data, "data")
		}
	}
}

// mergeNestedMap merges src into the nested map at the given path in dst.
// Top-level keys from src overwrite keys in the existing map; keys not present in src are preserved.
func mergeNestedMap(dst map[string]interface{}, src map[string]interface{}, fields ...string) {
	existing, _, _ := unstructured.NestedMap(dst, fields...)
	if existing == nil {
		existing = make(map[string]interface{})
	}
	for k, v := range src {
		existing[k] = v
	}
	_ = unstructured.SetNestedField(dst, existing, fields...)
}

// resourceNeedsUpdate compares labels, annotations, ownerReferences, spec, and data
// between an existing resource and the desired state. Returns true if an update is needed.
// For spec and data, only keys present in rendered are compared (merge semantics).
func resourceNeedsUpdate(existing, desired *unstructured.Unstructured, rendered map[string]interface{}) bool {
	if !reflect.DeepEqual(existing.GetLabels(), desired.GetLabels()) {
		return true
	}
	if !reflect.DeepEqual(existing.GetAnnotations(), desired.GetAnnotations()) {
		return true
	}
	if !reflect.DeepEqual(existing.GetOwnerReferences(), desired.GetOwnerReferences()) {
		return true
	}
	if spec, ok := rendered["spec"]; ok {
		if nestedMapKeysDiffer(existing.Object, spec, "spec") {
			return true
		}
	}
	if data, ok := rendered["data"]; ok {
		if nestedMapKeysDiffer(existing.Object, data, "data") {
			return true
		}
	}
	return false
}

// nestedMapKeysDiffer checks whether the rendered keys differ from what's in existing.
// Keys in existing that are not in rendered are ignored (set by external controllers).
func nestedMapKeysDiffer(existingObj map[string]interface{}, rendered interface{}, fields ...string) bool {
	renderedMap, ok := rendered.(map[string]interface{})
	if !ok {
		existingVal, _, _ := unstructured.NestedFieldNoCopy(existingObj, fields...)
		return !reflect.DeepEqual(existingVal, rendered)
	}
	existingMap, _, _ := unstructured.NestedMap(existingObj, fields...)
	if existingMap == nil {
		return len(renderedMap) > 0
	}
	for k, v := range renderedMap {
		if !reflect.DeepEqual(existingMap[k], v) {
			return true
		}
	}
	return false
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
