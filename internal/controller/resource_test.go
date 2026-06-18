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
	"reflect"
	"testing"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuildDesiredResource(t *testing.T) {
	claim := &clusterclaimv1alpha1.ClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claim1",
			Namespace: "ns1",
			UID:       "uid-claim1",
		},
	}
	tmpl := &clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate{
		Spec: clusterclaimv1alpha1.ClusterClaimObserveResourceTemplateSpec{
			APIVersion: "cluster.x-k8s.io/v1beta2",
			Kind:       "Cluster",
		},
	}
	rendered := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels":      map[string]interface{}{"app": "test", "clusterclaim.in-cloud.io/claim-name": "should-be-overridden"},
			"annotations": map[string]interface{}{"note": "value"},
		},
		"spec": map[string]interface{}{
			"paused":   false,
			"replicas": int64(3),
		},
		"data": map[string]interface{}{"key": "val"},
	}

	obj := buildDesiredResource(claim, tmpl, rendered, "claim1-infra", "ns1")

	if got := obj.GetAPIVersion(); got != "cluster.x-k8s.io/v1beta2" {
		t.Errorf("apiVersion = %q, want cluster.x-k8s.io/v1beta2", got)
	}
	if got := obj.GetKind(); got != "Cluster" {
		t.Errorf("kind = %q, want Cluster", got)
	}
	if obj.GetName() != "claim1-infra" || obj.GetNamespace() != "ns1" {
		t.Errorf("name/namespace = %q/%q, want claim1-infra/ns1", obj.GetName(), obj.GetNamespace())
	}

	owners := obj.GetOwnerReferences()
	if len(owners) != 1 || owners[0].Name != "claim1" || owners[0].Kind != "ClusterClaim" {
		t.Fatalf("ownerReferences = %+v, want single ClusterClaim owner claim1", owners)
	}
	if owners[0].Controller == nil || !*owners[0].Controller {
		t.Errorf("ownerReference.Controller = %v, want true", owners[0].Controller)
	}

	// standard claim labels win over rendered labels of the same key
	wantLabels := map[string]string{
		"app":                                 "test",
		"clusterclaim.in-cloud.io/claim-name": "claim1",
		"clusterclaim.in-cloud.io/claim-namespace": "ns1",
	}
	if got := obj.GetLabels(); !reflect.DeepEqual(got, wantLabels) {
		t.Errorf("labels = %v, want %v", got, wantLabels)
	}

	if got := obj.GetAnnotations(); !reflect.DeepEqual(got, map[string]string{"note": "value"}) {
		t.Errorf("annotations = %v, want {note: value}", got)
	}

	wantSpec := map[string]interface{}{"paused": false, "replicas": int64(3)}
	if got, _, _ := unstructured.NestedMap(obj.Object, "spec"); !reflect.DeepEqual(got, wantSpec) {
		t.Errorf("spec = %v, want %v", got, wantSpec)
	}
	if got, _, _ := unstructured.NestedMap(obj.Object, "data"); !reflect.DeepEqual(got, map[string]interface{}{"key": "val"}) {
		t.Errorf("data = %v, want {key: val}", got)
	}
}

func TestBuildDesiredResourceNoOptionalFields(t *testing.T) {
	claim := &clusterclaimv1alpha1.ClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "n"},
	}
	tmpl := &clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate{
		Spec: clusterclaimv1alpha1.ClusterClaimObserveResourceTemplateSpec{
			APIVersion: "in-cloud.io/v1alpha1",
			Kind:       "CertificateSet",
		},
	}

	obj := buildDesiredResource(claim, tmpl, map[string]interface{}{}, "c-infra", "n")

	if _, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec"); found {
		t.Error("spec should be absent when not rendered")
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "data"); found {
		t.Error("data should be absent when not rendered")
	}
	if obj.GetAnnotations() != nil {
		t.Errorf("annotations should be nil when not rendered, got %v", obj.GetAnnotations())
	}
}
