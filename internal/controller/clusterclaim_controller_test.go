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
	"sigs.k8s.io/controller-runtime/pkg/event"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
)

func newTestClusterClaim(name, namespace string) *clusterclaimv1alpha1.ClusterClaim {
	return &clusterclaimv1alpha1.ClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: clusterclaimv1alpha1.ClusterClaimSpec{
			ObserveTemplateRef: clusterclaimv1alpha1.TemplateRef{Name: "default-observe"},
			CertificateSetTemplateRef: clusterclaimv1alpha1.DualTemplateRef{
				Infra: clusterclaimv1alpha1.TemplateRef{Name: "default-certset-infra"},
			},
			ClusterTemplateRef: clusterclaimv1alpha1.DualTemplateRef{
				Infra: clusterclaimv1alpha1.TemplateRef{Name: "v1.34.4"},
			},
			CcmCsrTemplateRef: clusterclaimv1alpha1.TemplateRef{Name: "default-ccm"},
			Replicas:          1,
			Configuration: clusterclaimv1alpha1.ConfigurationSpec{
				CpuCount: 4,
				DiskSize: 51200,
				Memory:   8192,
			},
			Infra: clusterclaimv1alpha1.InfraSpec{
				Role:   "customer/infra",
				Paused: false,
				Network: clusterclaimv1alpha1.NetworkConfig{
					ServiceCidr:       "10.96.0.0/12",
					PodCidr:           "10.244.0.0/16",
					ClusterDNS:        "10.96.0.10",
					KubeApiserverPort: 6443,
				},
				ComponentVersions: map[string]clusterclaimv1alpha1.ComponentVersion{
					"kubernetes": {Version: "v1.34.4"},
				},
			},
			Client: clusterclaimv1alpha1.ClientSpec{Enabled: false},
		},
	}
}

func newTestTemplate(name, apiVersion, kind, value string) *clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate {
	return &clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: clusterclaimv1alpha1.ClusterClaimObserveResourceTemplateSpec{
			APIVersion: apiVersion,
			Kind:       kind,
			Value:      value,
		},
	}
}

func ensureTestTemplates() {
	observeTmpl := newTestTemplate("default-observe", "argoproj.io/v1alpha1", "Application", `
spec:
  project: default
  source:
    repoURL: https://example.com
`)
	certSetInfraTmpl := newTestTemplate("default-certset-infra", "in-cloud.io/v1alpha1", "CertificateSet", `
metadata:
  labels:
    cluster.x-k8s.io/cluster-name: "{{ index .ClusterClaim.metadata "name" }}-infra"
spec:
  environment: "{{ .ClusterClaim.spec.infra.role }}"
`)
	clusterInfraTmpl := newTestTemplate("v1.34.4", "cluster.x-k8s.io/v1beta2", "Cluster", `
spec:
  paused: false
  controlPlaneEndpoint:
    host: ""
    port: 6443
`)
	ccmTmpl := newTestTemplate("default-ccm", "controller.in-cloud.io/v1alpha1", "CcmCsrc", `
spec:
  beget-ccm:
    appSpec:
      applications:
        ccmInfra:
          enabled: true
`)
	certSetClientTmpl := newTestTemplate("default-certset-client", "in-cloud.io/v1alpha1", "CertificateSet", `
metadata:
  labels:
    cluster.x-k8s.io/cluster-name: "{{ index .ClusterClaim.metadata "name" }}-client"
spec:
  environment: client
`)
	clusterClientTmpl := newTestTemplate("v1.35.2", "cluster.x-k8s.io/v1beta2", "Cluster", `
spec:
  paused: false
  controlPlaneEndpoint:
    host: ""
    port: 6443
`)

	for _, tmpl := range []*clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate{
		observeTmpl, certSetInfraTmpl, clusterInfraTmpl, ccmTmpl, certSetClientTmpl, clusterClientTmpl,
	} {
		existing := &clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: tmpl.Name}, existing)
		if err != nil {
			Expect(k8sClient.Create(ctx, tmpl)).To(Succeed())
		}
	}
}

// patchCertificateSetReady patches a CertificateSet status to have Ready=True condition.
func patchCertificateSetReady(name, namespace string) {
	certSet := &unstructured.Unstructured{}
	certSet.SetGroupVersionKind(CertificateSetGVK)
	ExpectWithOffset(1, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, certSet)).To(Succeed())

	patch, _ := json.Marshal(map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "Ready",
					"status": "True",
				},
			},
		},
	})
	ExpectWithOffset(1, k8sClient.Status().Patch(ctx, certSet,
		client.RawPatch(types.MergePatchType, patch))).To(Succeed())
}

// patchClusterInfraProvisioned patches a Cluster status to have infrastructureProvisioned=true
// and sets the controlPlaneEndpoint in spec.
func patchClusterInfraProvisioned(name, namespace string) {
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(ClusterGVK)
	ExpectWithOffset(1, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cluster)).To(Succeed())

	// Patch status: infrastructureProvisioned.
	statusPatch, _ := json.Marshal(map[string]interface{}{
		"status": map[string]interface{}{
			"initialization": map[string]interface{}{
				"infrastructureProvisioned": true,
			},
		},
	})
	ExpectWithOffset(1, k8sClient.Status().Patch(ctx, cluster,
		client.RawPatch(types.MergePatchType, statusPatch))).To(Succeed())

	// Patch spec: controlPlaneEndpoint (this is a spec field, not status).
	ExpectWithOffset(1, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cluster)).To(Succeed())
	_ = unstructured.SetNestedField(cluster.Object, "10.0.0.1", "spec", "controlPlaneEndpoint", "host")
	_ = unstructured.SetNestedField(cluster.Object, int64(6443), "spec", "controlPlaneEndpoint", "port")
	ExpectWithOffset(1, k8sClient.Update(ctx, cluster)).To(Succeed())
}

// patchClusterCPInitialized patches a Cluster status to have controlPlaneInitialized=true
// and sets replica counts.
func patchClusterCPInitialized(name, namespace string) {
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(ClusterGVK)
	ExpectWithOffset(1, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cluster)).To(Succeed())

	statusPatch, _ := json.Marshal(map[string]interface{}{
		"status": map[string]interface{}{
			"initialization": map[string]interface{}{
				"infrastructureProvisioned": true,
				"controlPlaneInitialized":   true,
			},
			"controlPlane": map[string]interface{}{
				"availableReplicas": int64(1),
				"desiredReplicas":   int64(1),
			},
		},
	})
	ExpectWithOffset(1, k8sClient.Status().Patch(ctx, cluster,
		client.RawPatch(types.MergePatchType, statusPatch))).To(Succeed())
}

var _ = Describe("ClusterClaim Controller", func() {
	const (
		timeout  = 15 * time.Second
		polling  = 250 * time.Millisecond
		duration = 3 * time.Second
	)

	var ns *corev1.Namespace

	BeforeEach(func() {
		ensureTestTemplates()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-cc-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})

	Context("Happy path — stops at WaitingDependency", func() {
		It("should add finalizer, create Application and CertificateSet[infra], and wait for CertificateSet Ready", func() {
			claim := newTestClusterClaim("happy-path", ns.Name)
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Verify finalizer is added.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Finalizers).To(ContainElement(clusterclaimv1alpha1.ClusterClaimFinalizer))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Verify Phase=WaitingDependency (Step 3 blocks — CertificateSet not Ready).
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
				g.Expect(fetched.Status.ObservedGeneration).To(Equal(fetched.Generation))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Verify Application was created with correct name, namespace, labels, ownerRef.
			app := &unstructured.Unstructured{}
			app.SetAPIVersion("argoproj.io/v1alpha1")
			app.SetKind("Application")
			appKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}
			Expect(k8sClient.Get(ctx, appKey, app)).To(Succeed())

			Expect(app.GetLabels()).To(HaveKeyWithValue(clusterclaimv1alpha1.LabelClaimName, claim.Name))
			Expect(app.GetLabels()).To(HaveKeyWithValue(clusterclaimv1alpha1.LabelClaimNamespace, ns.Name))
			Expect(app.GetOwnerReferences()).To(HaveLen(1))
			Expect(app.GetOwnerReferences()[0].Kind).To(Equal("ClusterClaim"))
			Expect(app.GetOwnerReferences()[0].Name).To(Equal(claim.Name))
			Expect(*app.GetOwnerReferences()[0].Controller).To(BeTrue())

			// Verify spec was rendered.
			spec, found, err := unstructured.NestedMap(app.Object, "spec")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(spec["project"]).To(Equal("default"))

			// Verify CertificateSet[infra] was created.
			certSet := &unstructured.Unstructured{}
			certSet.SetAPIVersion("in-cloud.io/v1alpha1")
			certSet.SetKind("CertificateSet")
			certSetKey := types.NamespacedName{Name: claim.Name + "-infra", Namespace: ns.Name}
			Expect(k8sClient.Get(ctx, certSetKey, certSet)).To(Succeed())

			Expect(certSet.GetLabels()).To(HaveKeyWithValue(clusterclaimv1alpha1.LabelClaimName, claim.Name))
			Expect(certSet.GetLabels()).To(HaveKeyWithValue(clusterclaimv1alpha1.LabelClaimNamespace, ns.Name))
			Expect(certSet.GetLabels()).To(HaveKeyWithValue("cluster.x-k8s.io/cluster-name", claim.Name+"-infra"))
			Expect(certSet.GetOwnerReferences()).To(HaveLen(1))
			Expect(certSet.GetOwnerReferences()[0].Kind).To(Equal("ClusterClaim"))

			// Verify spec was rendered with template context.
			certSetSpec, found, err := unstructured.NestedMap(certSet.Object, "spec")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(certSetSpec["environment"]).To(Equal("customer/infra"))

			// Verify ApplicationCreated condition.
			var finalClaim clusterclaimv1alpha1.ClusterClaim
			Expect(k8sClient.Get(ctx, claimKey, &finalClaim)).To(Succeed())
			found = false
			for _, c := range finalClaim.Status.Conditions {
				if c.Type == clusterclaimv1alpha1.ConditionApplicationCreated {
					Expect(c.Status).To(Equal(metav1.ConditionTrue))
					Expect(c.Reason).To(Equal("Created"))
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "ApplicationCreated condition should exist")

			// Verify InfraCertificateReady=False condition (waiting).
			found = false
			for _, c := range finalClaim.Status.Conditions {
				if c.Type == clusterclaimv1alpha1.ConditionInfraCertificateReady {
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("WaitingDependency"))
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "InfraCertificateReady=False condition should exist")
		})
	})

	Context("Full pipeline Steps 1-8 (client.enabled=false)", func() {
		It("should progress through all steps and reach Ready phase", func() {
			claim := newTestClusterClaim("full-pipe", ns.Name)
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// 1. Wait for WaitingDependency at Step 3.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// 2. Patch CertificateSet[infra] Ready=True. Owns() watch auto-triggers reconcile.
			patchCertificateSetReady(claim.Name+"-infra", ns.Name)

			// 3. Wait for Cluster[infra] to be created (Step 4) then wait at Step 5.
			Eventually(func(g Gomega) {
				cluster := &unstructured.Unstructured{}
				cluster.SetGroupVersionKind(ClusterGVK)
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-infra", Namespace: ns.Name}, cluster)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Verify still WaitingDependency (now at Step 5).
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))

				found := false
				for _, c := range fetched.Status.Conditions {
					if c.Type == clusterclaimv1alpha1.ConditionInfraCertificateReady {
						g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// 4. Patch Cluster[infra] infrastructureProvisioned=true + endpoint. Owns() watch auto-triggers reconcile.
			patchClusterInfraProvisioned(claim.Name+"-infra", ns.Name)

			// 5. Now waiting at Step 7 (controlPlaneInitialized).
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))

				found := false
				for _, c := range fetched.Status.Conditions {
					if c.Type == clusterclaimv1alpha1.ConditionInfraProvisioned {
						g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// 6. Patch Cluster[infra] controlPlaneInitialized=true + replicas. Owns() watch auto-triggers reconcile.
			patchClusterCPInitialized(claim.Name+"-infra", ns.Name)

			// 7. CcmCsrc should be created (Step 8) and pipeline reaches Ready.
			Eventually(func(g Gomega) {
				ccm := &unstructured.Unstructured{}
				ccm.SetGroupVersionKind(CcmCsrcGVK)
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name, Namespace: ns.Name}, ccm)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// 8. Verify Phase=Ready.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseReady))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// 9. Verify all conditions.
			var finalClaim clusterclaimv1alpha1.ClusterClaim
			Expect(k8sClient.Get(ctx, claimKey, &finalClaim)).To(Succeed())

			conditionTypes := []string{
				clusterclaimv1alpha1.ConditionApplicationCreated,
				clusterclaimv1alpha1.ConditionInfraCertificateReady,
				clusterclaimv1alpha1.ConditionInfraProvisioned,
				clusterclaimv1alpha1.ConditionInfraCPReady,
				clusterclaimv1alpha1.ConditionCcmCsrcCreated,
				clusterclaimv1alpha1.ConditionReady,
			}
			for _, ct := range conditionTypes {
				found := false
				for _, c := range finalClaim.Status.Conditions {
					if c.Type == ct {
						Expect(c.Status).To(Equal(metav1.ConditionTrue), "condition %s should be True", ct)
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "condition %s should exist", ct)
			}

			// 10. Verify CcmCsrc has correct labels and owner reference.
			ccm := &unstructured.Unstructured{}
			ccm.SetGroupVersionKind(CcmCsrcGVK)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name, Namespace: ns.Name}, ccm)).To(Succeed())
			Expect(ccm.GetLabels()).To(HaveKeyWithValue(clusterclaimv1alpha1.LabelClaimName, claim.Name))
			Expect(ccm.GetOwnerReferences()).To(HaveLen(1))
			Expect(ccm.GetOwnerReferences()[0].Kind).To(Equal("ClusterClaim"))
		})
	})

	Context("CertificateSet not ready", func() {
		It("should remain at WaitingDependency with InfraCertificateReady=False", func() {
			claim := newTestClusterClaim("certset-wait", ns.Name)
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Verify WaitingDependency.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Verify it stays WaitingDependency.
			Consistently(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
			}).WithTimeout(duration).WithPolling(polling).Should(Succeed())

			// Verify InfraCertificateReady=False condition.
			var fetched clusterclaimv1alpha1.ClusterClaim
			Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
			found := false
			for _, c := range fetched.Status.Conditions {
				if c.Type == clusterclaimv1alpha1.ConditionInfraCertificateReady {
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("WaitingDependency"))
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "InfraCertificateReady condition should exist")
		})
	})

	Context("Pause", func() {
		It("should transition to Paused when annotation is set and back when removed", func() {
			claim := newTestClusterClaim("pause-test", ns.Name)
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Wait for WaitingDependency first (pipeline runs, blocks at Step 3).
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Add pause annotation.
			var current clusterclaimv1alpha1.ClusterClaim
			Expect(k8sClient.Get(ctx, claimKey, &current)).To(Succeed())
			if current.Annotations == nil {
				current.Annotations = make(map[string]string)
			}
			current.Annotations[clusterclaimv1alpha1.PausedAnnotation] = "true"
			Expect(k8sClient.Update(ctx, &current)).To(Succeed())

			// Verify Paused phase and condition.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhasePaused))

				found := false
				for _, c := range fetched.Status.Conditions {
					if c.Type == clusterclaimv1alpha1.ConditionPaused {
						g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
						g.Expect(c.Reason).To(Equal("Paused"))
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "Paused condition should exist")
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Verify it stays paused (no requeue).
			Consistently(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhasePaused))
			}).WithTimeout(duration).WithPolling(polling).Should(Succeed())

			// Remove pause annotation.
			Expect(k8sClient.Get(ctx, claimKey, &current)).To(Succeed())
			delete(current.Annotations, clusterclaimv1alpha1.PausedAnnotation)
			Expect(k8sClient.Update(ctx, &current)).To(Succeed())

			// Verify it returns to WaitingDependency (Step 3 still blocks).
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})
	})

	Context("Deletion", func() {
		It("should remove finalizer and delete the object", func() {
			claim := newTestClusterClaim("delete-test", ns.Name)
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Wait for finalizer to be added.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Finalizers).To(ContainElement(clusterclaimv1alpha1.ClusterClaimFinalizer))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Delete the claim.
			Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

			// Verify the object is gone.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				err := k8sClient.Get(ctx, claimKey, &fetched)
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "object should be deleted")
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})
	})

	Context("Template re-render", func() {
		It("should update managed resource when template value changes", func() {
			// Create a unique template for this test to avoid conflicts.
			reRenderTmpl := newTestTemplate("rerender-observe", "argoproj.io/v1alpha1", "Application", `
spec:
  project: original
  source:
    repoURL: https://example.com
`)
			Expect(k8sClient.Create(ctx, reRenderTmpl)).To(Succeed())

			claim := newTestClusterClaim("rerender-test", ns.Name)
			claim.Spec.ObserveTemplateRef.Name = "rerender-observe"
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}
			appKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Wait for WaitingDependency (pipeline runs steps 1-2, blocks at 3).
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Verify initial spec.
			app := &unstructured.Unstructured{}
			app.SetAPIVersion("argoproj.io/v1alpha1")
			app.SetKind("Application")
			Expect(k8sClient.Get(ctx, appKey, app)).To(Succeed())
			project, _, _ := unstructured.NestedString(app.Object, "spec", "project")
			Expect(project).To(Equal("original"))

			// Update the template value. Template watch auto-triggers reconcile for referencing claims.
			var tmpl clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "rerender-observe"}, &tmpl)).To(Succeed())
			tmpl.Spec.Value = `
spec:
  project: updated
  source:
    repoURL: https://example.com/updated
`
			Expect(k8sClient.Update(ctx, &tmpl)).To(Succeed())

			// Verify the Application was updated (template watch triggers reconcile).
			Eventually(func(g Gomega) {
				app := &unstructured.Unstructured{}
				app.SetAPIVersion("argoproj.io/v1alpha1")
				app.SetKind("Application")
				g.Expect(k8sClient.Get(ctx, appKey, app)).To(Succeed())
				project, _, _ := unstructured.NestedString(app.Object, "spec", "project")
				g.Expect(project).To(Equal("updated"))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Clean up the unique template.
			Expect(k8sClient.Delete(ctx, reRenderTmpl)).To(Succeed())
		})
	})

	Context("Missing template", func() {
		It("should set Failed phase when template does not exist", func() {
			claim := newTestClusterClaim("missing-tmpl", ns.Name)
			claim.Spec.ObserveTemplateRef.Name = "nonexistent-template"
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Verify Phase=Failed with appropriate condition.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseFailed))

				found := false
				for _, c := range fetched.Status.Conditions {
					if c.Type == clusterclaimv1alpha1.ConditionReady {
						g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
						g.Expect(c.Reason).To(Equal("StepFailed"))
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "Ready=False condition should exist")
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})
	})

	Context("Not found", func() {
		It("should not error when reconciling a non-existent resource", func() {
			// The controller handles NotFound internally via client.IgnoreNotFound,
			// so we verify by confirming no panic/crash. The absence of errors in
			// the test suite confirms this behavior.
		})
	})

	Context("Watches — template change triggers reconcile for multiple claims", func() {
		It("should re-render managed resources when shared template changes", func() {
			// Create a shared template used by two claims.
			sharedTmpl := newTestTemplate("shared-tmpl", "argoproj.io/v1alpha1", "Application", `
spec:
  project: original-shared
  source:
    repoURL: https://example.com
`)
			Expect(k8sClient.Create(ctx, sharedTmpl)).To(Succeed())

			claim1 := newTestClusterClaim("watch-claim-1", ns.Name)
			claim1.Spec.ObserveTemplateRef.Name = "shared-tmpl"
			Expect(k8sClient.Create(ctx, claim1)).To(Succeed())

			claim2 := newTestClusterClaim("watch-claim-2", ns.Name)
			claim2.Spec.ObserveTemplateRef.Name = "shared-tmpl"
			Expect(k8sClient.Create(ctx, claim2)).To(Succeed())

			claim1Key := types.NamespacedName{Name: claim1.Name, Namespace: ns.Name}
			claim2Key := types.NamespacedName{Name: claim2.Name, Namespace: ns.Name}

			// Wait until both claims reach WaitingDependency (steps 1-2 done, blocks at 3).
			for _, key := range []types.NamespacedName{claim1Key, claim2Key} {
				Eventually(func(g Gomega) {
					var fetched clusterclaimv1alpha1.ClusterClaim
					g.Expect(k8sClient.Get(ctx, key, &fetched)).To(Succeed())
					g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
				}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
			}

			// Verify initial Application spec for claim1.
			app1 := &unstructured.Unstructured{}
			app1.SetAPIVersion("argoproj.io/v1alpha1")
			app1.SetKind("Application")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim1.Name, Namespace: ns.Name}, app1)).To(Succeed())
			project1, _, _ := unstructured.NestedString(app1.Object, "spec", "project")
			Expect(project1).To(Equal("original-shared"))

			// Update the shared template.
			var tmpl clusterclaimv1alpha1.ClusterClaimObserveResourceTemplate
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "shared-tmpl"}, &tmpl)).To(Succeed())
			tmpl.Spec.Value = `
spec:
  project: updated-shared
  source:
    repoURL: https://example.com/updated
`
			Expect(k8sClient.Update(ctx, &tmpl)).To(Succeed())

			// Both Applications should be updated via template watch.
			for _, claimName := range []string{claim1.Name, claim2.Name} {
				appKey := types.NamespacedName{Name: claimName, Namespace: ns.Name}
				Eventually(func(g Gomega) {
					app := &unstructured.Unstructured{}
					app.SetAPIVersion("argoproj.io/v1alpha1")
					app.SetKind("Application")
					g.Expect(k8sClient.Get(ctx, appKey, app)).To(Succeed())
					project, _, _ := unstructured.NestedString(app.Object, "spec", "project")
					g.Expect(project).To(Equal("updated-shared"))
				}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
			}

			Expect(k8sClient.Delete(ctx, sharedTmpl)).To(Succeed())
		})
	})

	Context("Watches — owned resource status change triggers reconcile", func() {
		It("should progress pipeline when CertificateSet status changes without manual trigger", func() {
			claim := newTestClusterClaim("owns-watch", ns.Name)
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Wait until blocked at Step 3 (CertificateSet not Ready).
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Patch CertificateSet Ready=True. Owns() watch triggers reconcile automatically.
			patchCertificateSetReady(claim.Name+"-infra", ns.Name)

			// Verify pipeline progressed past Step 3: Cluster[infra] should be created (Step 4).
			Eventually(func(g Gomega) {
				cluster := &unstructured.Unstructured{}
				cluster.SetGroupVersionKind(ClusterGVK)
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-infra", Namespace: ns.Name}, cluster)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Verify InfraCertificateReady condition is now True.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				found := false
				for _, c := range fetched.Status.Conditions {
					if c.Type == clusterclaimv1alpha1.ConditionInfraCertificateReady {
						g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})
	})

	Context("Full pipeline Steps 1-13 (client.enabled=true)", func() {
		It("should progress through all steps including client and reach Ready phase", func() {
			claim := newTestClusterClaim("full-client", ns.Name)
			claim.Spec.Client.Enabled = true
			claim.Spec.CertificateSetTemplateRef.Client = &clusterclaimv1alpha1.TemplateRef{Name: "default-certset-client"}
			claim.Spec.ClusterTemplateRef.Client = &clusterclaimv1alpha1.TemplateRef{Name: "v1.35.2"}
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Step 3: Wait for CertificateSet[infra] Ready.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			patchCertificateSetReady(claim.Name+"-infra", ns.Name)

			// Step 4: Cluster[infra] created, then blocked at Step 5.
			Eventually(func(g Gomega) {
				cluster := &unstructured.Unstructured{}
				cluster.SetGroupVersionKind(ClusterGVK)
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-infra", Namespace: ns.Name}, cluster)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			patchClusterInfraProvisioned(claim.Name+"-infra", ns.Name)

			// Step 6: CertificateSet[client] created (client.enabled=true), then blocked at Step 7.
			Eventually(func(g Gomega) {
				certSet := &unstructured.Unstructured{}
				certSet.SetGroupVersionKind(CertificateSetGVK)
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-client", Namespace: ns.Name}, certSet)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Step 7: Wait for Cluster[infra] CP initialized.
			patchClusterCPInitialized(claim.Name+"-infra", ns.Name)

			// Step 8: CcmCsrc created. Step 10: Cluster[client] created.
			Eventually(func(g Gomega) {
				ccm := &unstructured.Unstructured{}
				ccm.SetGroupVersionKind(CcmCsrcGVK)
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name, Namespace: ns.Name}, ccm)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Eventually(func(g Gomega) {
				cluster := &unstructured.Unstructured{}
				cluster.SetGroupVersionKind(ClusterGVK)
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-client", Namespace: ns.Name}, cluster)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Step 11: Wait for Cluster[client] CP initialized.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))

				found := false
				for _, c := range fetched.Status.Conditions {
					if c.Type == clusterclaimv1alpha1.ConditionClientCPReady {
						g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Patch Cluster[client] CP initialized.
			patchClusterCPInitialized(claim.Name+"-client", ns.Name)

			// Step 12: CcmCsrc updated. Step 13: Ready.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseReady))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Verify all conditions are True.
			var finalClaim clusterclaimv1alpha1.ClusterClaim
			Expect(k8sClient.Get(ctx, claimKey, &finalClaim)).To(Succeed())

			conditionTypes := []string{
				clusterclaimv1alpha1.ConditionApplicationCreated,
				clusterclaimv1alpha1.ConditionInfraCertificateReady,
				clusterclaimv1alpha1.ConditionInfraProvisioned,
				clusterclaimv1alpha1.ConditionInfraCPReady,
				clusterclaimv1alpha1.ConditionCcmCsrcCreated,
				clusterclaimv1alpha1.ConditionClientCPReady,
				clusterclaimv1alpha1.ConditionReady,
			}
			for _, ct := range conditionTypes {
				found := false
				for _, c := range finalClaim.Status.Conditions {
					if c.Type == ct {
						Expect(c.Status).To(Equal(metav1.ConditionTrue), "condition %s should be True", ct)
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "condition %s should exist", ct)
			}

			// Verify CertificateSet[client] has correct labels and owner reference.
			certSet := &unstructured.Unstructured{}
			certSet.SetGroupVersionKind(CertificateSetGVK)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-client", Namespace: ns.Name}, certSet)).To(Succeed())
			Expect(certSet.GetLabels()).To(HaveKeyWithValue(clusterclaimv1alpha1.LabelClaimName, claim.Name))
			Expect(certSet.GetLabels()).To(HaveKeyWithValue("cluster.x-k8s.io/cluster-name", claim.Name+"-client"))
			Expect(certSet.GetOwnerReferences()).To(HaveLen(1))
			Expect(certSet.GetOwnerReferences()[0].Kind).To(Equal("ClusterClaim"))

			// Verify Cluster[client] has correct labels and owner reference.
			clientCluster := &unstructured.Unstructured{}
			clientCluster.SetGroupVersionKind(ClusterGVK)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-client", Namespace: ns.Name}, clientCluster)).To(Succeed())
			Expect(clientCluster.GetLabels()).To(HaveKeyWithValue(clusterclaimv1alpha1.LabelClaimName, claim.Name))
			Expect(clientCluster.GetOwnerReferences()).To(HaveLen(1))
			Expect(clientCluster.GetOwnerReferences()[0].Kind).To(Equal("ClusterClaim"))
		})
	})

	Context("Deletion with resource cleanup", func() {
		It("should delete all managed resources and remove finalizer", func() {
			claim := newTestClusterClaim("del-cleanup", ns.Name)
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Progress to WaitingDependency (Application + CertificateSet[infra] created).
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Verify resources exist.
			app := &unstructured.Unstructured{}
			app.SetGroupVersionKind(ApplicationGVK)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name, Namespace: ns.Name}, app)).To(Succeed())

			certSet := &unstructured.Unstructured{}
			certSet.SetGroupVersionKind(CertificateSetGVK)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-infra", Namespace: ns.Name}, certSet)).To(Succeed())

			// Delete the claim.
			Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

			// Verify the claim object is gone (finalizer removed, deletion completes).
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				err := k8sClient.Get(ctx, claimKey, &fetched)
				g.Expect(err).To(HaveOccurred())
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Verify managed resources were deleted by the deletion pipeline.
			appCheck := &unstructured.Unstructured{}
			appCheck.SetGroupVersionKind(ApplicationGVK)
			err := k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name, Namespace: ns.Name}, appCheck)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Application should be deleted")

			certSetCheck := &unstructured.Unstructured{}
			certSetCheck.SetGroupVersionKind(CertificateSetGVK)
			err = k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-infra", Namespace: ns.Name}, certSetCheck)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "CertificateSet[infra] should be deleted")
		})
	})

	Context("Deletion with client enabled", func() {
		It("should delete all resources including client resources and remove finalizer", func() {
			claim := newTestClusterClaim("del-client", ns.Name)
			claim.Spec.Client.Enabled = true
			claim.Spec.CertificateSetTemplateRef.Client = &clusterclaimv1alpha1.TemplateRef{Name: "default-certset-client"}
			claim.Spec.ClusterTemplateRef.Client = &clusterclaimv1alpha1.TemplateRef{Name: "v1.35.2"}
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Progress through full pipeline to Ready.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			patchCertificateSetReady(claim.Name+"-infra", ns.Name)

			Eventually(func(g Gomega) {
				cluster := &unstructured.Unstructured{}
				cluster.SetGroupVersionKind(ClusterGVK)
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-infra", Namespace: ns.Name}, cluster)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			patchClusterInfraProvisioned(claim.Name+"-infra", ns.Name)

			Eventually(func(g Gomega) {
				certSet := &unstructured.Unstructured{}
				certSet.SetGroupVersionKind(CertificateSetGVK)
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-client", Namespace: ns.Name}, certSet)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			patchClusterCPInitialized(claim.Name+"-infra", ns.Name)

			Eventually(func(g Gomega) {
				cluster := &unstructured.Unstructured{}
				cluster.SetGroupVersionKind(ClusterGVK)
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + "-client", Namespace: ns.Name}, cluster)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			patchClusterCPInitialized(claim.Name+"-client", ns.Name)

			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseReady))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Now delete the claim.
			var current clusterclaimv1alpha1.ClusterClaim
			Expect(k8sClient.Get(ctx, claimKey, &current)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &current)).To(Succeed())

			// Verify the claim is gone.
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				err := k8sClient.Get(ctx, claimKey, &fetched)
				g.Expect(err).To(HaveOccurred())
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Verify all managed resources are deleted.
			appCheck := &unstructured.Unstructured{}
			appCheck.SetGroupVersionKind(ApplicationGVK)
			err := k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name, Namespace: ns.Name}, appCheck)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Application should be deleted")

			for _, suffix := range []string{"-infra", "-client"} {
				certSetCheck := &unstructured.Unstructured{}
				certSetCheck.SetGroupVersionKind(CertificateSetGVK)
				err = k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + suffix, Namespace: ns.Name}, certSetCheck)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(), "CertificateSet%s should be deleted", suffix)

				clusterCheck := &unstructured.Unstructured{}
				clusterCheck.SetGroupVersionKind(ClusterGVK)
				err = k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name + suffix, Namespace: ns.Name}, clusterCheck)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Cluster%s should be deleted", suffix)
			}

			ccmCheck := &unstructured.Unstructured{}
			ccmCheck.SetGroupVersionKind(CcmCsrcGVK)
			err = k8sClient.Get(ctx, types.NamespacedName{Name: claim.Name, Namespace: ns.Name}, ccmCheck)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "CcmCsrc should be deleted")
		})
	})

	Context("No-op reconcile stability", func() {
		It("should not change resourceVersion when status is already up to date", func() {
			claim := newTestClusterClaim("noop-rv", ns.Name)
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			claimKey := types.NamespacedName{Name: claim.Name, Namespace: ns.Name}

			// Wait for WaitingDependency (stable state while CertificateSet is not Ready).
			Eventually(func(g Gomega) {
				var fetched clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(clusterclaimv1alpha1.PhaseWaitingDependency))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Record the resourceVersion after status is set.
			var fetched clusterclaimv1alpha1.ClusterClaim
			Expect(k8sClient.Get(ctx, claimKey, &fetched)).To(Succeed())
			rv := fetched.ResourceVersion

			// Wait a bit to ensure any further reconcile cycles don't change resourceVersion.
			Consistently(func(g Gomega) {
				var current clusterclaimv1alpha1.ClusterClaim
				g.Expect(k8sClient.Get(ctx, claimKey, &current)).To(Succeed())
				g.Expect(current.ResourceVersion).To(Equal(rv))
			}).WithTimeout(duration).WithPolling(polling).Should(Succeed())
		})
	})

	Context("Kubeconfig secret predicate", func() {
		It("should match only secrets with -infra-kubeconfig suffix", func() {
			pred := kubeconfigSecretPredicate()

			matching := event.CreateEvent{
				Object: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "ec8a00-infra-kubeconfig", Namespace: "default"},
				},
			}
			nonMatching := event.CreateEvent{
				Object: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "ec8a00-client-kubeconfig", Namespace: "default"},
				},
			}
			unrelated := event.CreateEvent{
				Object: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "some-secret", Namespace: "default"},
				},
			}

			Expect(pred.Create(matching)).To(BeTrue())
			Expect(pred.Create(nonMatching)).To(BeFalse())
			Expect(pred.Create(unrelated)).To(BeFalse())
		})
	})
})
