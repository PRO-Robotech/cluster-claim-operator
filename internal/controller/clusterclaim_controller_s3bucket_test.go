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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
)

func ensureS3BucketClaimTemplate() {
	tmpl := newTestTemplate("default-s3-bucket", "vault.in-cloud.io/v1alpha1", "S3BucketClaim", `
spec:
  vaultConfigRef:
    name: vault-bucket
  clusterRef:
    name: "{{ index .ClusterClaim.metadata "name" }}"
  customerLogin: "{{ index .ClusterClaim.metadata "namespace" }}"
  region: ru1
  bucket:
    managedBy: SYSTEM
    configurationId: s3_v1
  vault:
    secretsPrefix: "clusters/{{ index .ClusterClaim.metadata "namespace" }}-{{ index .ClusterClaim.metadata "name" }}"
    destinationPath: s3
  deletionPolicy: Purge
`)
	existing := &clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: tmpl.Name}, existing); apierrors.IsNotFound(err) {
		Expect(k8sClient.Create(ctx, tmpl)).To(Succeed())
	}
}

func patchS3BucketClaimPhase(name, namespace, phase string) {
	sbc := &unstructured.Unstructured{}
	sbc.SetGroupVersionKind(S3BucketClaimGVK)
	ExpectWithOffset(1, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sbc)).To(Succeed())

	patch, _ := json.Marshal(map[string]interface{}{
		"status": map[string]interface{}{
			"phase": phase,
		},
	})
	ExpectWithOffset(1, k8sClient.Status().Patch(ctx, sbc,
		client.RawPatch(types.MergePatchType, patch))).To(Succeed())
}

var _ = Describe("ClusterClaim S3BucketClaim integration", func() {
	const (
		timeout = 15 * time.Second
		polling = 250 * time.Millisecond
	)

	var ns *corev1.Namespace

	BeforeEach(func() {
		ensureTestTemplates()
		ensureS3BucketClaimTemplate()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-sbc-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})

	Context("s3BucketClaimTemplateRef set", func() {
		It("creates S3BucketClaim right after Application, blocks until phase=Ready, then proceeds", func() {
			claim := newTestClusterClaim("sbc-happy", ns.Name)
			claim.Spec.S3BucketClaimTemplateRef = &clusterclaimv1alpha1.TemplateRef{Name: "default-s3-bucket"}
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}
			sbcKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Created before infra is driven ready — the step runs right after Application.
			Eventually(func(g Gomega) {
				sbc := &unstructured.Unstructured{}
				sbc.SetGroupVersionKind(S3BucketClaimGVK)
				g.Expect(k8sClient.Get(ctx, sbcKey, sbc)).To(Succeed())
				g.Expect(sbc.GetLabels()).To(HaveKeyWithValue(clusterclaimv1alpha1.LabelClaimName, claim.Name))
				g.Expect(sbc.GetOwnerReferences()).To(HaveLen(1))
				g.Expect(sbc.GetOwnerReferences()[0].Kind).To(Equal("ClusterClaim"))
				login, _, _ := unstructured.NestedString(sbc.Object, "spec", "customerLogin")
				g.Expect(login).To(Equal(ns.Name))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			driveInfraReady(claim.Name, ns.Name)

			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))

				createdSeen, readySeen := false, false
				for _, c := range fetched.Status.Conditions {
					if c.Type == clusterclaimv1alpha1.ConditionS3BucketClaimCreated {
						g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
						createdSeen = true
					}
					if c.Type == clusterclaimv1alpha1.ConditionS3BucketClaimReady {
						g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
						readySeen = true
					}
				}
				g.Expect(createdSeen).To(BeTrue())
				g.Expect(readySeen).To(BeTrue())

				g.Expect(fetched.Status.S3Bucket).NotTo(BeNil())
				g.Expect(fetched.Status.S3Bucket.Ready).To(BeFalse())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			patchS3BucketClaimPhase(claim.Name, ns.Name, clusterclaimv1alpha1.PhaseReady)

			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseReady))
				g.Expect(fetched.Status.S3Bucket).NotTo(BeNil())
				g.Expect(fetched.Status.S3Bucket.Ready).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})

		It("deletes the S3BucketClaim during claim deletion", func() {
			claim := newTestClusterClaim("sbc-del", ns.Name)
			claim.Spec.S3BucketClaimTemplateRef = &clusterclaimv1alpha1.TemplateRef{Name: "default-s3-bucket"}
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			sbcKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}
			Eventually(func(g Gomega) {
				sbc := &unstructured.Unstructured{}
				sbc.SetGroupVersionKind(S3BucketClaimGVK)
				g.Expect(k8sClient.Get(ctx, sbcKey, sbc)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

			Eventually(func(g Gomega) {
				sbc := &unstructured.Unstructured{}
				sbc.SetGroupVersionKind(S3BucketClaimGVK)
				err := k8sClient.Get(ctx, sbcKey, sbc)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				err := k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name, Namespace: ns.Name}, &fetched)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})
	})
})
