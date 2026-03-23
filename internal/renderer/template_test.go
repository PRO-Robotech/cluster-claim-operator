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

package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRender_SimpleClusterClaimFields(t *testing.T) {
	claim := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "clusterclaim.in-cloud.io/v1alpha1",
			"kind":       "ClusterClaim",
			"metadata": map[string]interface{}{
				"name":      "ec8a00",
				"namespace": "dlputi1u",
			},
			"spec": map[string]interface{}{
				"infra": map[string]interface{}{
					"role": "customer/infra",
				},
			},
		},
	}
	ctx := BuildContext(claim, nil, nil)

	tmpl := `metadata:
  labels:
    cluster.x-k8s.io/cluster-name: {{ .ClusterClaim.metadata.name }}-infra
spec:
  environment: {{ .ClusterClaim.spec.infra.role }}`

	result, err := Render(tmpl, ctx)
	require.NoError(t, err)

	metadata, ok := result["metadata"].(map[string]interface{})
	require.True(t, ok)
	labels, ok := metadata["labels"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ec8a00-infra", labels["cluster.x-k8s.io/cluster-name"])

	spec, ok := result["spec"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "customer/infra", spec["environment"])
}

func TestRender_WithInfraControlPlaneEndpoint(t *testing.T) {
	claim := newTestClaim()
	infraCluster := newTestInfraCluster()
	ctx := BuildContext(claim, infraCluster, nil)

	tmpl := `spec:
  controlPlaneHost: {{ .InfraControlPlaneEndpoint.Host }}
  controlPlanePort: {{ .InfraControlPlaneEndpoint.Port }}
  initialized: {{ .InfraControlPlaneInitialized }}`

	result, err := Render(tmpl, ctx)
	require.NoError(t, err)

	spec, ok := result["spec"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "10.0.0.1", spec["controlPlaneHost"])
	// YAML unmarshalling turns numeric values into float64 or int64
	assert.EqualValues(t, 6443, spec["controlPlanePort"])
	assert.Equal(t, true, spec["initialized"])
}

func TestRender_MissingKeyError(t *testing.T) {
	ctx := TemplateContext{
		ClusterClaim: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test",
			},
		},
	}

	tmpl := `name: {{ .ClusterClaim.spec.nonExistentField }}`

	_, err := Render(tmpl, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execute template")
}

func TestRender_SprigFunctions(t *testing.T) {
	ctx := TemplateContext{
		ClusterClaim: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test-claim",
			},
		},
		// InfraControlPlaneEndpoint is nil, so default can be tested
		// against a nil struct field.
	}

	tmpl := `spec:
  defaultEndpoint: {{ default "no-endpoint" .InfraControlPlaneEndpoint }}
  quoted: {{ .ClusterClaim.metadata.name | quote }}
  upper: {{ .ClusterClaim.metadata.name | upper }}
  trimmed: {{ "  hello  " | trim }}`

	result, err := Render(tmpl, ctx)
	require.NoError(t, err)

	spec, ok := result["spec"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "no-endpoint", spec["defaultEndpoint"])
	assert.Equal(t, "test-claim", spec["quoted"])
	assert.Equal(t, "TEST-CLAIM", spec["upper"])
	assert.Equal(t, "hello", spec["trimmed"])
}

func TestRender_ToYaml(t *testing.T) {
	ctx := TemplateContext{
		ClusterClaim: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test",
			},
			"spec": map[string]interface{}{
				"extraEnvs": map[string]interface{}{
					"region":      "ru-1",
					"environment": "production",
				},
			},
		},
	}

	tmpl := `spec:
  envs:
{{ .ClusterClaim.spec.extraEnvs | toYaml | indent 4 }}`

	result, err := Render(tmpl, ctx)
	require.NoError(t, err)

	spec, ok := result["spec"].(map[string]interface{})
	require.True(t, ok)
	envs, ok := spec["envs"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ru-1", envs["region"])
	assert.Equal(t, "production", envs["environment"])
}

func TestRender_ToYamlWithList(t *testing.T) {
	ctx := TemplateContext{
		ClusterClaim: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test",
			},
			"spec": map[string]interface{}{
				"items": []interface{}{"one", "two", "three"},
			},
		},
	}

	tmpl := `spec:
  items:
{{ .ClusterClaim.spec.items | toYaml | indent 4 }}`

	result, err := Render(tmpl, ctx)
	require.NoError(t, err)

	spec, ok := result["spec"].(map[string]interface{})
	require.True(t, ok)
	items, ok := spec["items"].([]interface{})
	require.True(t, ok)
	assert.Len(t, items, 3)
	assert.Equal(t, "one", items[0])
}

func TestRender_InvalidTemplateSyntax(t *testing.T) {
	ctx := TemplateContext{
		ClusterClaim: map[string]interface{}{},
	}

	tmpl := `spec:
  value: {{ .Unclosed`

	_, err := Render(tmpl, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse template")
}

func TestRender_InvalidYAMLOutput(t *testing.T) {
	ctx := TemplateContext{
		ClusterClaim: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test",
			},
		},
	}

	// Produces invalid YAML (tab characters can cause issues, but a more
	// reliable way to produce invalid YAML for map[string]interface{} is
	// to produce something that isn't a map at the top level).
	tmpl := `- {{ .ClusterClaim.metadata.name }}
- second`

	_, err := Render(tmpl, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal rendered YAML")
}

func TestRender_CertificateSetTemplate(t *testing.T) {
	claim := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "clusterclaim.in-cloud.io/v1alpha1",
			"kind":       "ClusterClaim",
			"metadata": map[string]interface{}{
				"name":      "ec8a00",
				"namespace": "dlputi1u",
			},
			"spec": map[string]interface{}{
				"infra": map[string]interface{}{
					"role": "customer/infra",
				},
			},
		},
	}
	ctx := BuildContext(claim, nil, nil)

	tmpl := `metadata:
  labels:
    cluster.x-k8s.io/cluster-name: {{ .ClusterClaim.metadata.name }}-infra
  annotations:
    secret-copy.in-cloud.io/dstClusterKubeconfig: {{ .ClusterClaim.metadata.namespace }}/{{ .ClusterClaim.metadata.name }}-infra-kubeconfig
spec:
  environment: {{ .ClusterClaim.spec.infra.role }}
  issuerRef:
    kind: ClusterIssuer
    name: selfsigned`

	result, err := Render(tmpl, ctx)
	require.NoError(t, err)

	metadata := result["metadata"].(map[string]interface{})
	labels := metadata["labels"].(map[string]interface{})
	assert.Equal(t, "ec8a00-infra", labels["cluster.x-k8s.io/cluster-name"])

	annotations := metadata["annotations"].(map[string]interface{})
	assert.Equal(t, "dlputi1u/ec8a00-infra-kubeconfig",
		annotations["secret-copy.in-cloud.io/dstClusterKubeconfig"])

	spec := result["spec"].(map[string]interface{})
	assert.Equal(t, "customer/infra", spec["environment"])
	issuerRef := spec["issuerRef"].(map[string]interface{})
	assert.Equal(t, "ClusterIssuer", issuerRef["kind"])
	assert.Equal(t, "selfsigned", issuerRef["name"])
}

func TestRender_ConfigMapTemplateWithEndpoint(t *testing.T) {
	claim := newTestClaim()
	infraCluster := newTestInfraCluster()
	ctx := BuildContext(claim, infraCluster, nil)

	tmpl := `data:
  KUBERNETES_API_HOST: {{ .InfraControlPlaneEndpoint.Host }}
  KUBERNETES_API_PORT: "{{ .InfraControlPlaneEndpoint.Port }}"
  CP_REPLICAS: "{{ .InfraControlPlaneAvailableReplicas }}/{{ .InfraControlPlaneDesiredReplicas }}"
  CP_INITIALIZED: "{{ .InfraControlPlaneInitialized }}"`

	result, err := Render(tmpl, ctx)
	require.NoError(t, err)

	data, ok := result["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "10.0.0.1", data["KUBERNETES_API_HOST"])
	assert.Equal(t, "6443", data["KUBERNETES_API_PORT"])
	assert.Equal(t, "3/3", data["CP_REPLICAS"])
	assert.Equal(t, "true", data["CP_INITIALIZED"])
}

func TestRender_Ternary(t *testing.T) {
	ctx := TemplateContext{
		ClusterClaim: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test",
			},
		},
		InfraControlPlaneInitialized: true,
	}

	tmpl := `spec:
  status: {{ ternary "ready" "pending" .InfraControlPlaneInitialized }}`

	result, err := Render(tmpl, ctx)
	require.NoError(t, err)

	spec, ok := result["spec"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ready", spec["status"])
}

func TestToYaml_ErrorReturnsEmpty(t *testing.T) {
	// Channels cannot be marshalled to YAML
	result := toYaml(make(chan int))
	assert.Equal(t, "", result)
}

func TestToYaml_RemovesTrailingNewline(t *testing.T) {
	result := toYaml("hello")
	assert.Equal(t, "hello", result)
	assert.NotContains(t, result, "\n")
}
