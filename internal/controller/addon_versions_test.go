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
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/renderer"
)

// The claim is read into a typed struct and only then converted to a map for
// templates, so a field missing from the Go types is silently dropped. This
// test walks the same path as the pipeline: YAML -> typed -> toUnstructured ->
// BuildContext -> Render.
func TestAddonVersionsReachTemplate(t *testing.T) {
	const claimYAML = `
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaim
metadata:
  name: c1
  namespace: ns1
spec:
  infra:
    role: infra
    componentVersions:
      kubernetes: {version: v1.36.4}
    addonVersions:
      coredns: {version: 1.30.0-1}
      cert-manager-csi-driver: {version: 0.10.4-6}
`
	const tmpl = `data:
  addonVersions: {{ dig "spec" "infra" "addonVersions" dict .ClusterClaim | toJson | quote }}
`

	var claim clusterclaimv1alpha1.ClusterClaim
	if err := yaml.Unmarshal([]byte(claimYAML), &claim); err != nil {
		t.Fatalf("unmarshal claim: %v", err)
	}

	scheme := runtime.NewScheme()
	if err := clusterclaimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add to scheme: %v", err)
	}
	u, err := toUnstructured(&claim, scheme)
	if err != nil {
		t.Fatalf("toUnstructured: %v", err)
	}

	out, err := renderer.Render(tmpl, renderer.BuildContext(u, nil, nil))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got := out["data"].(map[string]interface{})["addonVersions"]
	want := `{"cert-manager-csi-driver":{"version":"0.10.4-6"},"coredns":{"version":"1.30.0-1"}}`
	if got != want {
		t.Fatalf("addonVersions in rendered template:\n got: %v\nwant: %s", got, want)
	}
}
