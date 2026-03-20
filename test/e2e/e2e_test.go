//go:build e2e
// +build e2e

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

package e2e

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PRO-Robotech/cluster-claim-operator/test/utils"
)

// namespace where the project is deployed in
const namespace = "cluster-claim-operator-system"

// serviceAccountName created for the project
const serviceAccountName = "cluster-claim-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "cluster-claim-operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "cluster-claim-operator-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("installing external dependency CRDs (Application, CertificateSet, Cluster, CcmCsrc)")
		for _, crdFile := range []string{
			"testdata/crds/applications.argoproj.io.yaml",
			"testdata/crds/certificatesets.in-cloud.io.yaml",
			"testdata/crds/clusters.cluster.x-k8s.io.yaml",
			"testdata/crds/ccmcsrcs.controller.in-cloud.io.yaml",
		} {
			cmd = exec.Command("kubectl", "apply", "-f", crdFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to install external CRD: "+crdFile)
		}

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("disabling webhook in controller-manager (no cert-manager certs in e2e)")
		cmd = exec.Command("kubectl", "patch", "deployment",
			"cluster-claim-operator-controller-manager", "-n", namespace,
			"--type=json", "-p",
			`[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--enable-webhook=false"}]`)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to patch deployment to disable webhook")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=cluster-claim-operator-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("ensuring the controller pod is ready")
			verifyControllerPodReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Controller pod not ready")
			}
			Eventually(verifyControllerPodReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks
	})

	Context("ClusterClaim happy path (infra-only)", func() {
		const (
			testNs    = "e2e-test"
			claimName = "e2e-infra"
		)

		BeforeAll(func() {
			By("creating test namespace")
			cmd := exec.Command("kubectl", "create", "ns", testNs)
			_, _ = utils.Run(cmd) // ignore if exists

			By("applying e2e templates")
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/templates.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply e2e templates")
		})

		AfterAll(func() {
			cmd := exec.Command("kubectl", "delete", "ns", testNs, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should set Phase=Failed when template does not exist", func() {
			By("creating ClusterClaim referencing a nonexistent template")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/clusterclaim-missing-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Phase=Failed")
			Eventually(func(g Gomega) {
				phase, err := kubectlGetJSONPath(testNs, "clusterclaim", "e2e-missing-tmpl", "{.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("Failed"))
			}).Should(Succeed())

			By("verifying Ready=False with StepFailed reason")
			reason, err := kubectlGetJSONPath(testNs, "clusterclaim", "e2e-missing-tmpl",
				`{.status.conditions[?(@.type=="Ready")].reason}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal("StepFailed"))

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "clusterclaim", "e2e-missing-tmpl", "-n", testNs, "--timeout=30s")
			_, _ = utils.Run(cmd)
		})

		It("should set Phase=Failed when template has bad syntax", func() {
			By("applying the bad-syntax template")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/template-bad-syntax.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating ClusterClaim referencing the bad template")
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/clusterclaim-bad-template.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Phase=Failed")
			Eventually(func(g Gomega) {
				phase, err := kubectlGetJSONPath(testNs, "clusterclaim", "e2e-bad-tmpl", "{.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("Failed"))
			}).Should(Succeed())

			By("verifying Ready=False with StepFailed reason")
			reason, err := kubectlGetJSONPath(testNs, "clusterclaim", "e2e-bad-tmpl",
				`{.status.conditions[?(@.type=="Ready")].reason}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal("StepFailed"))

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "clusterclaim", "e2e-bad-tmpl", "-n", testNs, "--timeout=30s")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "clusterclaimobserveresourcetemplate", "e2e-bad-syntax", "--timeout=10s")
			_, _ = utils.Run(cmd)
		})

		It("should apply remote ConfigMaps when configMapTemplateRef is set", func() {
			By("creating beget-system namespace for remote ConfigMaps")
			cmd := exec.Command("kubectl", "create", "ns", "beget-system")
			_, _ = utils.Run(cmd) // ignore if exists

			By("applying ConfigMap template")
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/template-cm-infra.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating ServiceAccount and RBAC for remote kubeconfig")
			cmd = exec.Command("kubectl", "create", "sa", "e2e-remote", "-n", testNs)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "create", "clusterrolebinding", "e2e-remote",
				"--clusterrole=cluster-admin", "--serviceaccount="+testNs+":e2e-remote")
			_, _ = utils.Run(cmd)

			By("creating ClusterClaim with configMapTemplateRef (kubeconfig Secret not yet created)")
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/clusterclaim-with-configmaps.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			claimName := "e2e-cm"
			certSetName := claimName + "-infra"
			clusterName := claimName + "-infra"

			By("progressing pipeline: patching CertificateSet Ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificateset", certSetName, "-n", testNs)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())

			patchJSON := `{"status":{"conditions":[{"type":"Ready","status":"True"}]}}`
			cmd = exec.Command("kubectl", "patch", "certificateset", certSetName,
				"-n", testNs, "--subresource=status", "--type=merge", "-p", patchJSON)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("patching Cluster[infra] spec.controlPlaneEndpoint first")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "cluster.cluster.x-k8s.io", clusterName, "-n", testNs)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())

			epPatch := `{"spec":{"controlPlaneEndpoint":{"host":"10.0.0.1","port":6443}}}`
			cmd = exec.Command("kubectl", "patch", "cluster.cluster.x-k8s.io", clusterName,
				"-n", testNs, "--type=merge", "-p", epPatch)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("patching Cluster[infra] status: provisioned + CP initialized")
			provPatch := `{"status":{"initialization":{"infrastructureProvisioned":true}}}`
			cmd = exec.Command("kubectl", "patch", "cluster.cluster.x-k8s.io", clusterName,
				"-n", testNs, "--subresource=status", "--type=merge", "-p", provPatch)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cpPatch := `{"status":{"initialization":{"infrastructureProvisioned":true,"controlPlaneInitialized":true},"controlPlane":{"availableReplicas":1,"desiredReplicas":1}}}`
			cmd = exec.Command("kubectl", "patch", "cluster.cluster.x-k8s.io", clusterName,
				"-n", testNs, "--subresource=status", "--type=merge", "-p", cpPatch)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for pipeline to reach Step 9 (WaitingDependency for kubeconfig Secret)")
			Eventually(func(g Gomega) {
				cond, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName,
					`{.status.conditions[?(@.type=="RemoteConfigApplied")].reason}`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(cond).To(Equal("WaitingDependency"))
			}).Should(Succeed())

			By("creating kubeconfig Secret to unblock Step 9")
			err = createLoopbackKubeconfigSecret(testNs, "e2e-cm-infra-kubeconfig", testNs, "e2e-remote")
			Expect(err).NotTo(HaveOccurred(), "Failed to create kubeconfig secret")

			By("waiting for Phase=Ready")
			Eventually(func(g Gomega) {
				phase, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName, "{.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("Ready"))
			}).Should(Succeed())

			By("verifying RemoteConfigApplied condition is True")
			cond, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName,
				`{.status.conditions[?(@.type=="RemoteConfigApplied")].status}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(cond).To(Equal("True"))

			By("verifying parameters-infra ConfigMap was created in beget-system namespace")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "configmap", "parameters-infra", "-n", "beget-system")
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())

			By("verifying ConfigMap data contains expected values")
			cmData, err := kubectlGetJSONPath("beget-system", "configmap", "parameters-infra", "{.data.environment}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmData).To(Equal("infra"))

			cmCluster, err := kubectlGetJSONPath("beget-system", "configmap", "parameters-infra", "{.data.clusterName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmCluster).To(Equal("e2e-cm-infra"))

			cmHost, err := kubectlGetJSONPath("beget-system", "configmap", "parameters-infra", "{.data.clusterHost}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmHost).To(Equal("10.0.0.1"))

			cmReplicas, err := kubectlGetJSONPath("beget-system", "configmap", "parameters-infra", "{.data.controlPlaneDesiredReplicas}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmReplicas).To(Equal("1"))

			By("verifying ConfigMap has standard labels")
			lblName, err := kubectlGetJSONPath("beget-system", "configmap", "parameters-infra",
				`{.metadata.labels.clusterclaim\.in-cloud\.io/claim-name}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(lblName).To(Equal(claimName))

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "clusterclaim", claimName, "-n", testNs, "--timeout=60s")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "clusterrolebinding", "e2e-remote", "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "ns", "beget-system", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should create parameters-infra and parameters-client when client.enabled=true", func() {
			By("setting up beget-system, SA, templates")
			cmd := exec.Command("kubectl", "create", "ns", "beget-system")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/template-cm-infra.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/template-cm-client.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			cmd = exec.Command("kubectl", "create", "sa", "e2e-remote", "-n", testNs)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "create", "clusterrolebinding", "e2e-remote-full",
				"--clusterrole=cluster-admin", "--serviceaccount="+testNs+":e2e-remote")
			_, _ = utils.Run(cmd)

			By("creating ClusterClaim with client.enabled=true and configMapTemplateRef infra+client")
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/clusterclaim-full-with-configmaps.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			claimName := "e2e-full-cm"

			By("progressing infra pipeline to Step 9")
			progressToStep9(testNs, claimName)

			By("waiting for Step 9 to need kubeconfig, then creating it")
			Eventually(func(g Gomega) {
				cond, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName,
					`{.status.conditions[?(@.type=="RemoteConfigApplied")].reason}`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(cond).To(Equal("WaitingDependency"))
			}).Should(Succeed())

			err = createLoopbackKubeconfigSecret(testNs, claimName+"-infra-kubeconfig", testNs, "e2e-remote")
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Cluster[client] to be created (Step 10)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "cluster.cluster.x-k8s.io", claimName+"-client", "-n", testNs)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())

			By("patching Cluster[client] CP initialized (Step 11)")
			cmd = exec.Command("kubectl", "patch", "cluster.cluster.x-k8s.io", claimName+"-client",
				"-n", testNs, "--subresource=status", "--type=merge",
				"-p", `{"status":{"initialization":{"infrastructureProvisioned":true,"controlPlaneInitialized":true},"controlPlane":{"availableReplicas":1,"desiredReplicas":1}}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Phase=Ready")
			Eventually(func(g Gomega) {
				phase, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName, "{.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("Ready"))
			}).Should(Succeed())

			By("verifying parameters-infra exists with correct data")
			cmEnv, err := kubectlGetJSONPath("beget-system", "configmap", "parameters-infra", "{.data.environment}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmEnv).To(Equal("infra"))

			By("verifying parameters-client exists with correct data")
			cmEnv, err = kubectlGetJSONPath("beget-system", "configmap", "parameters-client", "{.data.environment}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmEnv).To(Equal("client"))

			cmCluster, err := kubectlGetJSONPath("beget-system", "configmap", "parameters-client", "{.data.clusterName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmCluster).To(Equal(claimName + "-client"))

			cmPort, err := kubectlGetJSONPath("beget-system", "configmap", "parameters-client", "{.data.clusterPort}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmPort).To(Equal("26443"))

			By("verifying parameters-system does NOT exist (role is customer/infra, not system)")
			cmd = exec.Command("kubectl", "get", "configmap", "parameters-system", "-n", "beget-system")
			_, err = utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "parameters-system should not exist for customer/infra role")

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "clusterclaim", claimName, "-n", testNs, "--timeout=60s")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "clusterrolebinding", "e2e-remote-full", "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "configmap", "parameters-infra", "parameters-client", "-n", "beget-system", "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "ns", "beget-system", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should create parameters-infra and parameters-system when role contains system", func() {
			By("setting up beget-system, SA, templates")
			cmd := exec.Command("kubectl", "create", "ns", "beget-system")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/template-cm-infra.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			cmd = exec.Command("kubectl", "create", "sa", "e2e-remote", "-n", testNs)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "create", "clusterrolebinding", "e2e-remote-sys",
				"--clusterrole=cluster-admin", "--serviceaccount="+testNs+":e2e-remote")
			_, _ = utils.Run(cmd)

			By("creating ClusterClaim with system role and configMapTemplateRef")
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/clusterclaim-system-with-configmaps.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			claimName := "e2e-sys-cm"

			By("progressing pipeline to Step 9")
			progressToStep9(testNs, claimName)

			By("waiting for Step 9 to need kubeconfig, then creating it")
			Eventually(func(g Gomega) {
				cond, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName,
					`{.status.conditions[?(@.type=="RemoteConfigApplied")].reason}`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(cond).To(Equal("WaitingDependency"))
			}).Should(Succeed())

			err = createLoopbackKubeconfigSecret(testNs, claimName+"-infra-kubeconfig", testNs, "e2e-remote")
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Phase=Ready")
			Eventually(func(g Gomega) {
				phase, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName, "{.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("Ready"))
			}).Should(Succeed())

			By("verifying parameters-infra exists")
			cmEnv, err := kubectlGetJSONPath("beget-system", "configmap", "parameters-infra", "{.data.environment}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmEnv).To(Equal("infra"))

			By("verifying parameters-system exists (role=system/infra)")
			cmEnv, err = kubectlGetJSONPath("beget-system", "configmap", "parameters-system", "{.data.environment}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmEnv).To(Equal("infra"))

			cmLbl, err := kubectlGetJSONPath("beget-system", "configmap", "parameters-system",
				`{.metadata.labels.clusterclaim\.in-cloud\.io/claim-name}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(cmLbl).To(Equal(claimName))

			By("verifying parameters-client does NOT exist (client.enabled=false)")
			cmd = exec.Command("kubectl", "get", "configmap", "parameters-client", "-n", "beget-system")
			_, err = utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "parameters-client should not exist when client.enabled=false")

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "clusterclaim", claimName, "-n", testNs, "--timeout=60s")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "clusterrolebinding", "e2e-remote-sys", "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "configmap", "parameters-infra", "parameters-system", "-n", "beget-system", "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "ns", "beget-system", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should progress through the full infra-only pipeline to Ready", func() {
			By("creating the ClusterClaim")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/clusterclaim-infra-only.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterClaim")

			By("waiting for Phase=WaitingDependency (blocked at Step 3: CertificateSet not Ready)")
			Eventually(func(g Gomega) {
				phase, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName, "{.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("WaitingDependency"))
			}).Should(Succeed())

			By("verifying Application was created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "application", claimName, "-n", testNs)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())

			By("verifying CertificateSet[infra] was created")
			certSetName := claimName + "-infra"
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificateset", certSetName, "-n", testNs)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())

			By("patching CertificateSet[infra] status to Ready=True")
			patchJSON := `{"status":{"conditions":[{"type":"Ready","status":"True"}]}}`
			cmd = exec.Command("kubectl", "patch", "certificateset", certSetName,
				"-n", testNs, "--subresource=status", "--type=merge", "-p", patchJSON)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to patch CertificateSet status")

			By("waiting for Cluster[infra] to be created (Step 4)")
			clusterName := claimName + "-infra"
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "cluster.cluster.x-k8s.io", clusterName, "-n", testNs)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())

			By("patching Cluster[infra] spec.controlPlaneEndpoint")
			epPatch := `{"spec":{"controlPlaneEndpoint":{"host":"10.0.0.1","port":6443}}}`
			cmd = exec.Command("kubectl", "patch", "cluster.cluster.x-k8s.io", clusterName,
				"-n", testNs, "--type=merge", "-p", epPatch)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to patch Cluster endpoint")

			By("patching Cluster[infra] status: infrastructureProvisioned=true")
			provPatch := `{"status":{"initialization":{"infrastructureProvisioned":true}}}`
			cmd = exec.Command("kubectl", "patch", "cluster.cluster.x-k8s.io", clusterName,
				"-n", testNs, "--subresource=status", "--type=merge", "-p", provPatch)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to patch Cluster provisioned status")

			By("waiting for pipeline to pass Step 5 (WaitInfraProvisioned) — still WaitingDependency at Step 7")
			Eventually(func(g Gomega) {
				phase, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName, "{.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("WaitingDependency"))

				// Check InfraProvisioned condition is True.
				cond, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName,
					`{.status.conditions[?(@.type=="InfraProvisioned")].status}`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(cond).To(Equal("True"))
			}).Should(Succeed())

			By("patching Cluster[infra] status: controlPlaneInitialized=true + replicas")
			cpPatch := `{"status":{"initialization":{"infrastructureProvisioned":true,"controlPlaneInitialized":true},"controlPlane":{"availableReplicas":1,"desiredReplicas":1}}}`
			cmd = exec.Command("kubectl", "patch", "cluster.cluster.x-k8s.io", clusterName,
				"-n", testNs, "--subresource=status", "--type=merge", "-p", cpPatch)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to patch Cluster CP initialized status")

			By("waiting for Phase=Ready")
			Eventually(func(g Gomega) {
				phase, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName, "{.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("Ready"))
			}).Should(Succeed())

			By("verifying CcmCsrc was created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "ccmcsrc", claimName, "-n", testNs)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())

			By("verifying all expected conditions are True")
			for _, condType := range []string{
				"ApplicationCreated",
				"InfraCertificateReady",
				"InfraProvisioned",
				"InfraCPReady",
				"CcmCsrcCreated",
				"Ready",
			} {
				cond, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName,
					fmt.Sprintf(`{.status.conditions[?(@.type=="%s")].status}`, condType))
				Expect(err).NotTo(HaveOccurred(), "Failed to get condition %s", condType)
				Expect(cond).To(Equal("True"), "Condition %s should be True", condType)
			}

			By("verifying no-op reconcile stability (resourceVersion does not change)")
			rv, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName, "{.metadata.resourceVersion}")
			Expect(err).NotTo(HaveOccurred())
			Consistently(func(g Gomega) {
				currentRV, err := kubectlGetJSONPath(testNs, "clusterclaim", claimName, "{.metadata.resourceVersion}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(currentRV).To(Equal(rv))
			}, 10*time.Second, time.Second).Should(Succeed())

			By("deleting the ClusterClaim and verifying cleanup")
			cmd = exec.Command("kubectl", "delete", "clusterclaim", claimName, "-n", testNs, "--timeout=60s")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete ClusterClaim")

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "clusterclaim", claimName, "-n", testNs)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "ClusterClaim should be deleted")
			}).Should(Succeed())
		})
	})
})

// progressToStep9 drives the pipeline from creation through to Step 9 (waiting for kubeconfig).
// It patches CertificateSet[infra] Ready, Cluster[infra] endpoint+provisioned+CP initialized.
func progressToStep9(ns, claimName string) {
	certSetName := claimName + "-infra"
	clusterName := claimName + "-infra"

	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "certificateset", certSetName, "-n", ns)
		_, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
	}).Should(Succeed())

	cmd := exec.Command("kubectl", "patch", "certificateset", certSetName,
		"-n", ns, "--subresource=status", "--type=merge",
		"-p", `{"status":{"conditions":[{"type":"Ready","status":"True"}]}}`)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "cluster.cluster.x-k8s.io", clusterName, "-n", ns)
		_, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
	}).Should(Succeed())

	cmd = exec.Command("kubectl", "patch", "cluster.cluster.x-k8s.io", clusterName,
		"-n", ns, "--type=merge",
		"-p", `{"spec":{"controlPlaneEndpoint":{"host":"10.0.0.1","port":6443}}}`)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	cmd = exec.Command("kubectl", "patch", "cluster.cluster.x-k8s.io", clusterName,
		"-n", ns, "--subresource=status", "--type=merge",
		"-p", `{"status":{"initialization":{"infrastructureProvisioned":true}}}`)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	cmd = exec.Command("kubectl", "patch", "cluster.cluster.x-k8s.io", clusterName,
		"-n", ns, "--subresource=status", "--type=merge",
		"-p", `{"status":{"initialization":{"infrastructureProvisioned":true,"controlPlaneInitialized":true},"controlPlane":{"availableReplicas":1,"desiredReplicas":1}}}`)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

// createLoopbackKubeconfigSecret builds a kubeconfig pointing to the same cluster's
// internal API server and stores it in a Secret with key "value".
// This allows the operator's remote client to apply resources "remotely" to the same cluster.
func createLoopbackKubeconfigSecret(secretNs, secretName, saNs, saName string) error {
	// Get cluster CA from kube-root-ca.crt ConfigMap.
	cmd := exec.Command("kubectl", "get", "configmap", "kube-root-ca.crt",
		"-n", "default", "-o", "jsonpath={.data.ca\\.crt}")
	caCert, err := utils.Run(cmd)
	if err != nil {
		return fmt.Errorf("get CA cert: %w", err)
	}

	// Get SA token.
	cmd = exec.Command("kubectl", "create", "token", saName, "-n", saNs, "--duration=1h")
	token, err := utils.Run(cmd)
	if err != nil {
		return fmt.Errorf("create token: %w", err)
	}
	token = strings.TrimSpace(token)

	// Build kubeconfig YAML.
	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: %s
    server: https://kubernetes.default.svc
  name: remote
contexts:
- context:
    cluster: remote
    user: remote
  name: remote
current-context: remote
users:
- name: remote
  user:
    token: %s
`, base64Encode(caCert), token)

	// Create Secret via kubectl with --from-literal.
	cmd = exec.Command("kubectl", "create", "secret", "generic", secretName,
		"-n", secretNs, "--from-literal=value="+kubeconfig)
	_, err = utils.Run(cmd)
	return err
}

// base64Encode encodes a string to base64.
func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// kubectlGetJSONPath runs kubectl get with a jsonpath output and returns the trimmed result.
func kubectlGetJSONPath(namespace, resource, name, jsonpath string) (string, error) {
	cmd := exec.Command("kubectl", "get", resource, name,
		"-n", namespace, "-o", fmt.Sprintf("jsonpath=%s", jsonpath))
	output, err := utils.Run(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
