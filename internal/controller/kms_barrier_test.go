/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/renderer"
)

// vaultClaimObj builds the satellite as the operator sees it: unstructured.
func vaultClaimObj(name, ns string, withKeys bool, conds map[string]string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(VaultClaimGVK)
	u.SetName(name)
	u.SetNamespace(ns)

	if withKeys {
		if err := unstructured.SetNestedSlice(u.Object, []interface{}{
			map[string]interface{}{"name": ns + "-" + name + "-l2", "type": "aes256-gcm96"},
		}, "spec", "transit", "keys"); err != nil {
			panic(err)
		}
	}
	list := make([]interface{}, 0, len(conds))
	for t, s := range conds {
		list = append(list, map[string]interface{}{
			"type":               t,
			"status":             s,
			"reason":             "Test",
			"lastTransitionTime": metav1.Now().Format("2006-01-02T15:04:05Z"),
		})
	}
	if len(list) > 0 {
		if err := unstructured.SetNestedSlice(u.Object, list, "status", "conditions"); err != nil {
			panic(err)
		}
	}
	return u
}

func kmsBarrierReconciler(objs ...runtime.Object) *ClusterClaimReconciler {
	sch := runtime.NewScheme()
	if err := clusterclaimv1alpha1.AddToScheme(sch); err != nil {
		panic(err)
	}
	builder := ctrlfake.NewClientBuilder().WithScheme(sch)
	for _, o := range objs {
		if u, ok := o.(*unstructured.Unstructured); ok {
			builder = builder.WithObjects(u)
		}
	}
	return &ClusterClaimReconciler{Client: builder.Build(), Scheme: sch}
}

func conditionByType(claim *clusterclaimv1alpha1.ClusterClaim, condType string) *metav1.Condition {
	for i := range claim.Status.Conditions {
		if claim.Status.Conditions[i].Type == condType {
			return &claim.Status.Conditions[i]
		}
	}
	return nil
}

func kmsClaim(withRef bool) *clusterclaimv1alpha1.ClusterClaim {
	c := &clusterclaimv1alpha1.ClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "a8f60d", Namespace: "dlputi1u"},
	}
	if withRef {
		c.Spec.VaultClaimTemplateRef = &clusterclaimv1alpha1.TemplateRef{Name: "common-vaultclaim"}
	}
	return c
}

func TestKmsBarrierSkippedWithoutVaultClaimRef(t *testing.T) {
	r := kmsBarrierReconciler()
	claim := kmsClaim(false)

	res, err := r.stepWaitVaultKmsReady(context.Background(), claim, &renderer.TemplateContext{})
	if err != nil || res != Proceed {
		t.Fatalf("res=%v err=%v, want Proceed/nil", res, err)
	}
}

// The barrier must be invisible to clusters that do not ask for encryption.
func TestKmsBarrierInertWithoutTransitKeys(t *testing.T) {
	vc := vaultClaimObj("a8f60d", "dlputi1u", false, map[string]string{"Ready": "True"})
	r := kmsBarrierReconciler(vc)
	claim := kmsClaim(true)

	res, err := r.stepWaitVaultKmsReady(context.Background(), claim, &renderer.TemplateContext{})
	if err != nil || res != Proceed {
		t.Fatalf("res=%v err=%v, want Proceed/nil", res, err)
	}
	if c := conditionByType(claim, clusterclaimv1alpha1.ConditionVaultKmsReady); c != nil {
		t.Errorf("condition set to %v for a cluster without encryption; the barrier should stay silent", c.Status)
	}
}

func TestKmsBarrierWaitsForKeysAndRoles(t *testing.T) {
	cases := []struct {
		name  string
		conds map[string]string
		want  StepResult
	}{
		{
			name:  "neither condition yet",
			conds: map[string]string{},
			want:  Wait,
		},
		{
			name:  "key ready but role not applied",
			conds: map[string]string{"TransitKeysReady": "True"},
			want:  Wait,
		},
		{
			name:  "role applied but key not ready",
			conds: map[string]string{"RolesApplied": "True"},
			want:  Wait,
		},
		{
			name:  "key missing is reported False, not merely absent",
			conds: map[string]string{"TransitKeysReady": "False", "RolesApplied": "True"},
			want:  Wait,
		},
		{
			name:  "both ready",
			conds: map[string]string{"TransitKeysReady": "True", "RolesApplied": "True"},
			want:  Proceed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vc := vaultClaimObj("a8f60d", "dlputi1u", true, tc.conds)
			r := kmsBarrierReconciler(vc)
			claim := kmsClaim(true)

			res, err := r.stepWaitVaultKmsReady(context.Background(), claim, &renderer.TemplateContext{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res != tc.want {
				t.Errorf("res = %v, want %v", res, tc.want)
			}
			if tc.want == Proceed {
				c := conditionByType(claim, clusterclaimv1alpha1.ConditionVaultKmsReady)
				if c == nil || c.Status != metav1.ConditionTrue {
					t.Errorf("condition = %+v, want True", c)
				}
			}
		})
	}
}

// A missing satellite is the informer catching up, not a failure.
func TestKmsBarrierWaitsWhenVaultClaimAbsent(t *testing.T) {
	r := kmsBarrierReconciler()
	claim := kmsClaim(true)

	res, err := r.stepWaitVaultKmsReady(context.Background(), claim, &renderer.TemplateContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != Wait {
		t.Errorf("res = %v, want Wait", res)
	}
}

func TestHasTrueCondition(t *testing.T) {
	obj := vaultClaimObj("c", "ns", true, map[string]string{
		"TransitKeysReady": "True",
		"RolesApplied":     "False",
	}).Object

	if !hasTrueCondition(obj, "TransitKeysReady") {
		t.Error("TransitKeysReady should read as true")
	}
	if hasTrueCondition(obj, "RolesApplied") {
		t.Error("RolesApplied is False and must not read as true")
	}
	if hasTrueCondition(obj, "NotThere") {
		t.Error("an absent condition must not read as true")
	}
	if hasTrueCondition(map[string]interface{}{}, "Any") {
		t.Error("an object without a status must not read as true")
	}
}

// The barrier must precede ClusterClient and WaitClientCPReady.
func TestKmsBarrierOrderedBeforeClusterClient(t *testing.T) {
	r := &ClusterClaimReconciler{}
	steps := r.pipelineSteps()
	names := make([]string, 0, len(steps))
	for _, s := range steps {
		names = append(names, s.name)
	}
	idx := func(want string) int {
		for i, n := range names {
			if n == want {
				return i
			}
		}
		t.Fatalf("step %q not found in %v", want, names)
		return -1
	}
	barrier := idx("WaitVaultKmsReady")
	if ensure := idx("EnsureVaultClaim"); barrier <= ensure {
		t.Errorf("WaitVaultKmsReady at %d must come after EnsureVaultClaim at %d", barrier, ensure)
	}
	if clusterClient := idx("ClusterClient"); barrier >= clusterClient {
		t.Errorf("WaitVaultKmsReady at %d must come before ClusterClient at %d", barrier, clusterClient)
	}
	if cpReady := idx("WaitClientCPReady"); barrier >= cpReady {
		t.Errorf("WaitVaultKmsReady at %d must come before WaitClientCPReady at %d", barrier, cpReady)
	}
}
