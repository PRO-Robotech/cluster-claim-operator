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

// ensureVaultClaimTemplate creates the default-vault template used by VaultClaim integration tests.
func ensureVaultClaimTemplate() {
	tmpl := newTestTemplate("default-vault", "vault.in-cloud.io/v1alpha1", "VaultClaim", `
spec:
  vaultConfigRef:
    name: default
  clusterRef:
    name: "{{ index .ClusterClaim.metadata "name" }}"
    kubeconfigSecret: "{{ index .ClusterClaim.metadata "name" }}-infra-kubeconfig"
`)
	existing := &clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: tmpl.Name}, existing); apierrors.IsNotFound(err) {
		Expect(k8sClient.Create(ctx, tmpl)).To(Succeed())
	}
}

// patchVaultClaimPhase sets VaultClaim.status.phase to the given value.
func patchVaultClaimPhase(name, namespace, phase string) {
	vc := &unstructured.Unstructured{}
	vc.SetGroupVersionKind(VaultClaimGVK)
	ExpectWithOffset(1, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, vc)).To(Succeed())

	patch, _ := json.Marshal(map[string]interface{}{
		"status": map[string]interface{}{
			"phase": phase,
		},
	})
	ExpectWithOffset(1, k8sClient.Status().Patch(ctx, vc,
		client.RawPatch(types.MergePatchType, patch))).To(Succeed())
}

// driveInfraReady advances a fresh claim through Steps 1–7 by patching dependent resources.
func driveInfraReady(claimName, namespace string) {
	Eventually(func(g Gomega) {
		certSet := &unstructured.Unstructured{}
		certSet.SetGroupVersionKind(CertificateSetGVK)
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claimName + "-infra", Namespace: namespace}, certSet)).To(Succeed())
	}).WithTimeout(15 * time.Second).WithPolling(250 * time.Millisecond).Should(Succeed())
	patchCertificateSetReady(claimName+"-infra", namespace)

	Eventually(func(g Gomega) {
		cluster := &unstructured.Unstructured{}
		cluster.SetGroupVersionKind(ClusterGVK)
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claimName + "-infra", Namespace: namespace}, cluster)).To(Succeed())
	}).WithTimeout(15 * time.Second).WithPolling(250 * time.Millisecond).Should(Succeed())
	patchClusterCPInitialized(claimName+"-infra", namespace)
}

var _ = Describe("ClusterClaim VaultClaim integration", func() {
	const (
		timeout = 15 * time.Second
		polling = 250 * time.Millisecond
	)

	var ns *corev1.Namespace

	BeforeEach(func() {
		ensureTestTemplates()
		ensureVaultClaimTemplate()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-vc-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})

	Context("vaultClaimTemplateRef set", func() {
		It("creates VaultClaim after infra CP, blocks at Step 14 until phase=Ready, then proceeds", func() {
			claim := newTestClusterClaim("vc-happy", ns.Name)
			claim.Spec.VaultClaimTemplateRef = &clusterclaimv1alpha1.TemplateRef{Name: "default-vault"}
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			driveInfraReady(claim.Name, ns.Name)

			vcKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}
			Eventually(func(g Gomega) {
				vc := &unstructured.Unstructured{}
				vc.SetGroupVersionKind(VaultClaimGVK)
				g.Expect(k8sClient.Get(ctx, vcKey, vc)).To(Succeed())
				g.Expect(vc.GetLabels()).To(HaveKeyWithValue(clusterclaimv1alpha1.LabelClaimName, claim.Name))
				g.Expect(vc.GetOwnerReferences()).To(HaveLen(1))
				g.Expect(vc.GetOwnerReferences()[0].Kind).To(Equal("ClusterClaim"))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))

				createdSeen, readySeen := false, false
				for _, c := range fetched.Status.Conditions {
					if c.Type == clusterclaimv1alpha1.ConditionVaultClaimCreated {
						g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
						createdSeen = true
					}
					if c.Type == clusterclaimv1alpha1.ConditionVaultClaimReady {
						g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
						readySeen = true
					}
				}
				g.Expect(createdSeen).To(BeTrue())
				g.Expect(readySeen).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Vault).NotTo(BeNil())
				g.Expect(fetched.Status.Vault.Name).To(Equal(claim.Name))
				g.Expect(fetched.Status.Vault.Ready).To(BeFalse())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			patchVaultClaimPhase(claim.Name, ns.Name, clusterclaimv1alpha1.PhaseReady)

			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseReady))
				g.Expect(fetched.Status.Vault).NotTo(BeNil())
				g.Expect(fetched.Status.Vault.Phase).To(Equal(clusterclaimv1alpha1.PhaseReady))
				g.Expect(fetched.Status.Vault.Ready).To(BeTrue())

				readyCondTrue := false
				for _, c := range fetched.Status.Conditions {
					if c.Type == clusterclaimv1alpha1.ConditionVaultClaimReady {
						g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
						readyCondTrue = true
					}
				}
				g.Expect(readyCondTrue).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})

		It("returns to WaitingDependency when VaultClaim drifts away from Ready", func() {
			claim := newTestClusterClaim("vc-drift", ns.Name)
			claim.Spec.VaultClaimTemplateRef = &clusterclaimv1alpha1.TemplateRef{Name: "default-vault"}
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}
			driveInfraReady(claim.Name, ns.Name)

			Eventually(func(g Gomega) {
				vc := &unstructured.Unstructured{}
				vc.SetGroupVersionKind(VaultClaimGVK)
				g.Expect(k8sClient.Get(ctx, claimKey, vc)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			patchVaultClaimPhase(claim.Name, ns.Name, clusterclaimv1alpha1.PhaseReady)
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseReady))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			patchVaultClaimPhase(claim.Name, ns.Name, clusterclaimv1alpha1.PhaseProvisioning)
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
				g.Expect(fetched.Status.Vault).NotTo(BeNil())
				g.Expect(fetched.Status.Vault.Ready).To(BeFalse())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})
	})

	Context("vaultClaimTemplateRef not set", func() {
		It("skips VaultClaim steps entirely and reaches Ready", func() {
			claim := newTestClusterClaim("vc-skip", ns.Name)
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}
			driveInfraReady(claim.Name, ns.Name)

			Consistently(func(g Gomega) bool {
				vc := &unstructured.Unstructured{}
				vc.SetGroupVersionKind(VaultClaimGVK)
				err := k8sClient.Get(ctx, claimKey, vc)
				return apierrors.IsNotFound(err)
			}).WithTimeout(3 * time.Second).WithPolling(polling).Should(BeTrue())

			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseReady))
				g.Expect(fetched.Status.Vault).To(BeNil())

				for _, c := range fetched.Status.Conditions {
					Expect(c.Type).NotTo(Equal(clusterclaimv1alpha1.ConditionVaultClaimCreated))
					Expect(c.Type).NotTo(Equal(clusterclaimv1alpha1.ConditionVaultClaimReady))
				}
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})
	})

	Context("deletion", func() {
		It("deletes VaultClaim before CcmCsrc and Clusters when vaultClaimTemplateRef is set", func() {
			claim := newTestClusterClaim("vc-del", ns.Name)
			claim.Spec.VaultClaimTemplateRef = &clusterclaimv1alpha1.TemplateRef{Name: "default-vault"}
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}
			driveInfraReady(claim.Name, ns.Name)

			Eventually(func(g Gomega) {
				vc := &unstructured.Unstructured{}
				vc.SetGroupVersionKind(VaultClaimGVK)
				g.Expect(k8sClient.Get(ctx, claimKey, vc)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Expect(k8sClient.Delete(ctx, &clusterclaimv1alpha1.ClusterClaim{ObjectMeta: metav1.ObjectMeta{Name: claim.Name, Namespace: ns.Name}})).To(Succeed())

			// Strip the controller's finalizer to simulate vault-operator finishing cleanup.
			Eventually(func(g Gomega) {
				vc := &unstructured.Unstructured{}
				vc.SetGroupVersionKind(VaultClaimGVK)
				if err := k8sClient.Get(ctx, claimKey, vc); apierrors.IsNotFound(err) {
					return
				} else {
					g.Expect(err).NotTo(HaveOccurred())
				}
				vc.SetFinalizers(nil)
				g.Expect(k8sClient.Update(ctx, vc)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				err := k8sClient.Get(ctx, claimKey, &fetched)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(30 * time.Second).WithPolling(polling).Should(Succeed())
		})
	})
})
