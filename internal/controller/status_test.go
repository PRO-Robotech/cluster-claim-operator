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
	It("should be a no-op when both clusters are nil", func() {
		claim := &clusterclaimv1alpha1.ClusterClaim{}
		syncClusterStatuses(ctx, claim, nil, nil)
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

		syncClusterStatuses(ctx, claim, infraCluster, nil)

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

		syncClusterStatuses(ctx, claim, nil, clientCluster)

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

		syncClusterStatuses(ctx, claim, infraCluster, clientCluster)

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

		syncClusterStatuses(ctx, claim, emptyCluster, nil)

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

		syncClusterStatuses(ctx, claim, cluster, nil)

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
