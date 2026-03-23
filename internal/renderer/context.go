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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ControlPlaneEndpoint holds host and port for a control plane.
type ControlPlaneEndpoint struct {
	Host string
	Port int64
}

// TemplateContext is passed to Go templates during rendering.
type TemplateContext struct {
	// ClusterClaim is the full unstructured representation of the ClusterClaim object.
	// Templates access it as .ClusterClaim.metadata.name, .ClusterClaim.spec.infra.role, etc.
	ClusterClaim map[string]interface{}

	// Computed fields populated as the pipeline progresses.
	InfraControlPlaneEndpoint          *ControlPlaneEndpoint
	InfraControlPlaneInitialized       bool
	InfraControlPlaneAvailableReplicas int32
	InfraControlPlaneDesiredReplicas   int32

	ClientControlPlaneEndpoint          *ControlPlaneEndpoint
	ClientControlPlaneInitialized       bool
	ClientControlPlaneAvailableReplicas int32
	ClientControlPlaneDesiredReplicas   int32
}

// BuildContext creates a TemplateContext from unstructured resources.
// infraCluster and clientCluster may be nil if not yet created.
func BuildContext(claim, infraCluster, clientCluster *unstructured.Unstructured) TemplateContext {
	ctx := TemplateContext{
		ClusterClaim: claim.Object,
	}

	if infraCluster != nil {
		populateClusterFields(infraCluster, &ctx.InfraControlPlaneEndpoint,
			&ctx.InfraControlPlaneInitialized,
			&ctx.InfraControlPlaneAvailableReplicas,
			&ctx.InfraControlPlaneDesiredReplicas)
	}

	if clientCluster != nil {
		populateClusterFields(clientCluster, &ctx.ClientControlPlaneEndpoint,
			&ctx.ClientControlPlaneInitialized,
			&ctx.ClientControlPlaneAvailableReplicas,
			&ctx.ClientControlPlaneDesiredReplicas)
	}

	return ctx
}

func populateClusterFields(
	cluster *unstructured.Unstructured,
	endpoint **ControlPlaneEndpoint,
	cpInitialized *bool,
	availableReplicas *int32,
	desiredReplicas *int32,
) {
	host, _, _ := unstructured.NestedString(cluster.Object, "spec", "controlPlaneEndpoint", "host")
	port, _, _ := unstructured.NestedInt64(cluster.Object, "spec", "controlPlaneEndpoint", "port")
	if host != "" {
		*endpoint = &ControlPlaneEndpoint{Host: host, Port: port}
	}

	cpInit, _, _ := unstructured.NestedBool(cluster.Object, "status", "initialization", "controlPlaneInitialized")
	*cpInitialized = cpInit

	available, _, _ := unstructured.NestedInt64(cluster.Object, "status", "controlPlane", "availableReplicas")
	desired, _, _ := unstructured.NestedInt64(cluster.Object, "status", "controlPlane", "desiredReplicas")
	*availableReplicas = int32(available)
	*desiredReplicas = int32(desired)
}
