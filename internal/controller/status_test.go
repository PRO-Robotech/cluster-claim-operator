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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
)

var _ = Describe("syncClusterStatuses", func() {
	var r *ClusterClaimReconciler

	BeforeEach(func() {
		r = &ClusterClaimReconciler{Client: k8sClient}
	})

	It("should be a no-op when both clusters are nil", func() {
		claim := &clusterclaimv1alpha1.ClusterClaim{}
		r.syncClusterStatuses(ctx, claim, nil, nil)
		Expect(claim.Status.Clusters).To(BeNil())
	})

	It("should mirror infra cluster status with phase, conditions, and controlPlane", func() {
		claim := &clusterclaimv1alpha1.ClusterClaim{}

		infraCluster := &unstructured.Unstructured{Object: map[string]interface{}{
			"status": map[string]interface{}{
				"phase": "Provisioned",
				"conditions": []interface{}{
					map[string]interface{}{
						"type":               "Available",
						"status":             "True",
						"reason":             "Available",
						"message":            "",
						"observedGeneration": int64(3),
						"lastTransitionTime": "2026-03-19T18:39:50Z",
					},
					map[string]interface{}{
						"type":               "ControlPlaneInitialized",
						"status":             "True",
						"reason":             "Initialized",
						"observedGeneration": int64(3),
						"lastTransitionTime": "2026-03-19T18:39:23Z",
					},
				},
				"controlPlane": map[string]interface{}{
					"replicas":          int64(1),
					"readyReplicas":     int64(1),
					"availableReplicas": int64(1),
					"desiredReplicas":   int64(1),
					"upToDateReplicas":  int64(1),
				},
			},
		}}

		r.syncClusterStatuses(ctx, claim, infraCluster, nil)

		Expect(claim.Status.Clusters).NotTo(BeNil())
		Expect(claim.Status.Clusters.Infra).NotTo(BeNil())
		Expect(claim.Status.Clusters.Client).To(BeNil())

		infra := claim.Status.Clusters.Infra
		Expect(infra.Phase).To(Equal("Provisioned"))
		Expect(infra.Conditions).To(HaveLen(2))

		// Verify first condition.
		Expect(infra.Conditions[0].Type).To(Equal("Available"))
		Expect(infra.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(infra.Conditions[0].Reason).To(Equal("Available"))
		Expect(infra.Conditions[0].ObservedGeneration).To(Equal(int64(3)))

		// Verify controlPlane.
		Expect(infra.ControlPlane).NotTo(BeNil())
		Expect(infra.ControlPlane.Replicas).To(Equal(int32(1)))
		Expect(infra.ControlPlane.ReadyReplicas).To(Equal(int32(1)))
		Expect(infra.ControlPlane.AvailableReplicas).To(Equal(int32(1)))
		Expect(infra.ControlPlane.DesiredReplicas).To(Equal(int32(1)))
		Expect(infra.ControlPlane.UpToDateReplicas).To(Equal(int32(1)))

		// Workers should be nil (not present in status).
		Expect(infra.Workers).To(BeNil())
	})

	It("should mirror client cluster status with phase, conditions, and workers", func() {
		claim := &clusterclaimv1alpha1.ClusterClaim{}

		clientCluster := &unstructured.Unstructured{Object: map[string]interface{}{
			"status": map[string]interface{}{
				"phase": "Provisioned",
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "WorkersAvailable",
						"status": "True",
						"reason": "Available",
					},
				},
				"workers": map[string]interface{}{
					"replicas":          int64(3),
					"readyReplicas":     int64(3),
					"availableReplicas": int64(3),
					"desiredReplicas":   int64(3),
					"upToDateReplicas":  int64(3),
				},
			},
		}}

		r.syncClusterStatuses(ctx, claim, nil, clientCluster)

		Expect(claim.Status.Clusters).NotTo(BeNil())
		Expect(claim.Status.Clusters.Infra).To(BeNil())
		Expect(claim.Status.Clusters.Client).NotTo(BeNil())

		client := claim.Status.Clusters.Client
		Expect(client.Phase).To(Equal("Provisioned"))
		Expect(client.Conditions).To(HaveLen(1))
		Expect(client.Conditions[0].Type).To(Equal("WorkersAvailable"))

		// Workers.
		Expect(client.Workers).NotTo(BeNil())
		Expect(client.Workers.Replicas).To(Equal(int32(3)))
		Expect(client.Workers.AvailableReplicas).To(Equal(int32(3)))

		// ControlPlane should be nil.
		Expect(client.ControlPlane).To(BeNil())
	})

	It("should mirror both clusters when both are present", func() {
		claim := &clusterclaimv1alpha1.ClusterClaim{}

		infraCluster := &unstructured.Unstructured{Object: map[string]interface{}{
			"status": map[string]interface{}{
				"phase": "Provisioned",
				"controlPlane": map[string]interface{}{
					"availableReplicas": int64(1),
					"desiredReplicas":   int64(1),
				},
			},
		}}
		clientCluster := &unstructured.Unstructured{Object: map[string]interface{}{
			"status": map[string]interface{}{
				"phase": "Provisioned",
				"workers": map[string]interface{}{
					"availableReplicas": int64(2),
					"desiredReplicas":   int64(2),
				},
			},
		}}

		r.syncClusterStatuses(ctx, claim, infraCluster, clientCluster)

		Expect(claim.Status.Clusters.Infra).NotTo(BeNil())
		Expect(claim.Status.Clusters.Infra.Phase).To(Equal("Provisioned"))
		Expect(claim.Status.Clusters.Infra.ControlPlane.AvailableReplicas).To(Equal(int32(1)))

		Expect(claim.Status.Clusters.Client).NotTo(BeNil())
		Expect(claim.Status.Clusters.Client.Phase).To(Equal("Provisioned"))
		Expect(claim.Status.Clusters.Client.Workers.AvailableReplicas).To(Equal(int32(2)))
	})

	It("should handle cluster with no status gracefully", func() {
		claim := &clusterclaimv1alpha1.ClusterClaim{}

		emptyCluster := &unstructured.Unstructured{Object: map[string]interface{}{}}

		r.syncClusterStatuses(ctx, claim, emptyCluster, nil)

		Expect(claim.Status.Clusters).NotTo(BeNil())
		Expect(claim.Status.Clusters.Infra).NotTo(BeNil())
		Expect(claim.Status.Clusters.Infra.Phase).To(BeEmpty())
		Expect(claim.Status.Clusters.Infra.Conditions).To(BeNil())
		Expect(claim.Status.Clusters.Infra.ControlPlane).To(BeNil())
		Expect(claim.Status.Clusters.Infra.Workers).To(BeNil())
	})

	It("should skip conditions with empty type", func() {
		claim := &clusterclaimv1alpha1.ClusterClaim{}

		cluster := &unstructured.Unstructured{Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "",
						"status": "True",
					},
					map[string]interface{}{
						"type":   "Available",
						"status": "True",
						"reason": "Available",
					},
				},
			},
		}}

		r.syncClusterStatuses(ctx, claim, cluster, nil)

		Expect(claim.Status.Clusters.Infra.Conditions).To(HaveLen(1))
		Expect(claim.Status.Clusters.Infra.Conditions[0].Type).To(Equal("Available"))
	})
})

var _ = Describe("toInt32", func() {
	It("should convert int64", func() {
		Expect(toInt32(int64(42))).To(Equal(int32(42)))
	})
	It("should convert float64", func() {
		Expect(toInt32(float64(42))).To(Equal(int32(42)))
	})
	It("should return 0 for nil", func() {
		Expect(toInt32(nil)).To(Equal(int32(0)))
	})
	It("should return 0 for string", func() {
		Expect(toInt32("42")).To(Equal(int32(0)))
	})
})

var _ = Describe("fetchControlPlaneVersion", func() {
	var r *ClusterClaimReconciler

	const testNamespace = "default"

	BeforeEach(func() {
		r = &ClusterClaimReconciler{Client: k8sClient}
	})

	// Reference shape matches production: apiGroup and kind, no namespace and no version.
	newClusterWithCPRef := func(name, apiGroup, kind, refName string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta2",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"controlPlaneRef": map[string]interface{}{
					"apiGroup": apiGroup,
					"kind":     kind,
					"name":     refName,
				},
			},
			"status": map[string]interface{}{"phase": "Provisioned"},
		}}
	}

	createAddonClaim := func(name, specVersion, statusVersion string) {
		claim := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "addons.in-cloud.io/v1alpha1",
			"kind":       "AddonClaim",
			"metadata": map[string]interface{}{
				"name":        name,
				"namespace":   testNamespace,
				"annotations": map[string]interface{}{"external-status/type": "controlplane"},
			},
			"spec": map[string]interface{}{
				"addon":         map[string]interface{}{"name": "client-cp-control-plane"},
				"templateRef":   map[string]interface{}{"name": "client-cp-template"},
				"credentialRef": map[string]interface{}{"name": "infra-kubeconfig"},
				"version":       specVersion,
			},
		}}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		if statusVersion != "" {
			Expect(unstructured.SetNestedField(claim.Object, statusVersion, "status", "version")).To(Succeed())
			Expect(k8sClient.Status().Update(ctx, claim)).To(Succeed())
		}
	}

	It("should mirror the client control plane version from an AddonClaim", func() {
		createAddonClaim("client-cp-mirrored", "v1.35.2", "v1.34.5")

		claim := &clusterclaimv1alpha1.ClusterClaim{}
		cluster := newClusterWithCPRef("mirrored-client", "addons.in-cloud.io", "AddonClaim", "client-cp-mirrored")

		r.syncClusterStatuses(ctx, claim, nil, cluster)

		Expect(claim.Status.Clusters.Client).NotTo(BeNil())
		cpv := claim.Status.Clusters.Client.ControlPlaneVersion
		Expect(cpv).NotTo(BeNil())
		Expect(cpv.SpecVersion).To(Equal("v1.35.2"))
		Expect(cpv.StatusVersion).To(Equal("v1.34.5"))
	})

	It("should report an unpublished status version as empty rather than dropping the mirror", func() {
		createAddonClaim("client-cp-provisioning", "v1.35.2", "")

		claim := &clusterclaimv1alpha1.ClusterClaim{}
		cluster := newClusterWithCPRef("provisioning-client", "addons.in-cloud.io", "AddonClaim", "client-cp-provisioning")

		r.syncClusterStatuses(ctx, claim, nil, cluster)

		cpv := claim.Status.Clusters.Client.ControlPlaneVersion
		Expect(cpv).NotTo(BeNil())
		Expect(cpv.SpecVersion).To(Equal("v1.35.2"))
		Expect(cpv.StatusVersion).To(BeEmpty())
	})

	It("should return nil for an unknown control plane kind", func() {
		claim := &clusterclaimv1alpha1.ClusterClaim{}
		cluster := newClusterWithCPRef("unknown-cp", "example.com", "SomeOtherControlPlane", "whatever")

		r.syncClusterStatuses(ctx, claim, nil, cluster)

		Expect(claim.Status.Clusters.Client).NotTo(BeNil())
		Expect(claim.Status.Clusters.Client.ControlPlaneVersion).To(BeNil())
	})

	It("should return nil when the referenced control plane does not exist", func() {
		claim := &clusterclaimv1alpha1.ClusterClaim{}
		cluster := newClusterWithCPRef("missing-cp", "addons.in-cloud.io", "AddonClaim", "does-not-exist")

		r.syncClusterStatuses(ctx, claim, nil, cluster)

		Expect(claim.Status.Clusters.Client.ControlPlaneVersion).To(BeNil())
	})

	It("should return nil when the Cluster carries no controlPlaneRef", func() {
		claim := &clusterclaimv1alpha1.ClusterClaim{}
		cluster := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta2",
			"kind":       "Cluster",
			"metadata":   map[string]interface{}{"name": "no-ref", "namespace": testNamespace},
			"status":     map[string]interface{}{"phase": "Provisioned"},
		}}

		r.syncClusterStatuses(ctx, claim, nil, cluster)

		Expect(claim.Status.Clusters.Client.ControlPlaneVersion).To(BeNil())
	})
})
