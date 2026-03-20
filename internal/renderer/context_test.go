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

func newTestClaim() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "clusterclaim.in-cloud.io/v1alpha1",
			"kind":       "ClusterClaim",
			"metadata": map[string]interface{}{
				"name":      "test-claim",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"infra": map[string]interface{}{
					"role": "customer/infra",
				},
			},
		},
	}
}

func newTestInfraCluster() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta2",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      "test-claim-infra",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"controlPlaneEndpoint": map[string]interface{}{
					"host": "10.0.0.1",
					"port": int64(6443),
				},
			},
			"status": map[string]interface{}{
				"initialization": map[string]interface{}{
					"controlPlaneInitialized": true,
				},
				"controlPlane": map[string]interface{}{
					"availableReplicas": int64(3),
					"desiredReplicas":   int64(3),
				},
			},
		},
	}
}

func newTestClientCluster() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta2",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      "test-claim-client",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"controlPlaneEndpoint": map[string]interface{}{
					"host": "10.0.0.2",
					"port": int64(6443),
				},
			},
			"status": map[string]interface{}{
				"initialization": map[string]interface{}{
					"controlPlaneInitialized": true,
				},
				"controlPlane": map[string]interface{}{
					"availableReplicas": int64(1),
					"desiredReplicas":   int64(1),
				},
			},
		},
	}
}

func TestBuildContext_ClaimOnly(t *testing.T) {
	claim := newTestClaim()
	ctx := BuildContext(claim, nil, nil)

	require.NotNil(t, ctx.ClusterClaim)
	assert.Equal(t, claim.Object, ctx.ClusterClaim)

	assert.Nil(t, ctx.InfraControlPlaneEndpoint)
	assert.False(t, ctx.InfraControlPlaneInitialized)
	assert.Equal(t, int32(0), ctx.InfraControlPlaneAvailableReplicas)
	assert.Equal(t, int32(0), ctx.InfraControlPlaneDesiredReplicas)

	assert.Nil(t, ctx.ClientControlPlaneEndpoint)
	assert.False(t, ctx.ClientControlPlaneInitialized)
	assert.Equal(t, int32(0), ctx.ClientControlPlaneAvailableReplicas)
	assert.Equal(t, int32(0), ctx.ClientControlPlaneDesiredReplicas)
}

func TestBuildContext_WithInfraCluster(t *testing.T) {
	claim := newTestClaim()
	infraCluster := newTestInfraCluster()
	ctx := BuildContext(claim, infraCluster, nil)

	require.NotNil(t, ctx.InfraControlPlaneEndpoint)
	assert.Equal(t, "10.0.0.1", ctx.InfraControlPlaneEndpoint.Host)
	assert.Equal(t, int64(6443), ctx.InfraControlPlaneEndpoint.Port)
	assert.True(t, ctx.InfraControlPlaneInitialized)
	assert.Equal(t, int32(3), ctx.InfraControlPlaneAvailableReplicas)
	assert.Equal(t, int32(3), ctx.InfraControlPlaneDesiredReplicas)

	assert.Nil(t, ctx.ClientControlPlaneEndpoint)
	assert.False(t, ctx.ClientControlPlaneInitialized)
}

func TestBuildContext_WithBothClusters(t *testing.T) {
	claim := newTestClaim()
	infraCluster := newTestInfraCluster()
	clientCluster := newTestClientCluster()
	ctx := BuildContext(claim, infraCluster, clientCluster)

	require.NotNil(t, ctx.InfraControlPlaneEndpoint)
	assert.Equal(t, "10.0.0.1", ctx.InfraControlPlaneEndpoint.Host)
	assert.Equal(t, int64(6443), ctx.InfraControlPlaneEndpoint.Port)
	assert.True(t, ctx.InfraControlPlaneInitialized)
	assert.Equal(t, int32(3), ctx.InfraControlPlaneAvailableReplicas)
	assert.Equal(t, int32(3), ctx.InfraControlPlaneDesiredReplicas)

	require.NotNil(t, ctx.ClientControlPlaneEndpoint)
	assert.Equal(t, "10.0.0.2", ctx.ClientControlPlaneEndpoint.Host)
	assert.Equal(t, int64(6443), ctx.ClientControlPlaneEndpoint.Port)
	assert.True(t, ctx.ClientControlPlaneInitialized)
	assert.Equal(t, int32(1), ctx.ClientControlPlaneAvailableReplicas)
	assert.Equal(t, int32(1), ctx.ClientControlPlaneDesiredReplicas)
}

func TestBuildContext_NoEndpointWhenHostEmpty(t *testing.T) {
	claim := newTestClaim()
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta2",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      "test-claim-infra",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"controlPlaneEndpoint": map[string]interface{}{
					"host": "",
					"port": int64(6443),
				},
			},
			"status": map[string]interface{}{},
		},
	}
	ctx := BuildContext(claim, cluster, nil)

	assert.Nil(t, ctx.InfraControlPlaneEndpoint)
}

func TestBuildContext_MissingStatusFields(t *testing.T) {
	claim := newTestClaim()
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta2",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      "test-claim-infra",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{},
		},
	}
	ctx := BuildContext(claim, cluster, nil)

	assert.Nil(t, ctx.InfraControlPlaneEndpoint)
	assert.False(t, ctx.InfraControlPlaneInitialized)
	assert.Equal(t, int32(0), ctx.InfraControlPlaneAvailableReplicas)
	assert.Equal(t, int32(0), ctx.InfraControlPlaneDesiredReplicas)
}
