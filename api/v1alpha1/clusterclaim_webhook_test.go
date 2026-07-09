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

package v1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateUpdate_ClientEnabledImmutable(t *testing.T) {
	v := &clusterClaimValidator{}
	old := &ClusterClaim{Spec: ClusterClaimSpec{Client: ClientSpec{Enabled: false}}}
	newClaim := &ClusterClaim{Spec: ClusterClaimSpec{Client: ClientSpec{Enabled: true}}}
	_, err := v.ValidateUpdate(context.Background(), old, newClaim)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.client.enabled is immutable")
}

func TestValidateUpdate_NoChange(t *testing.T) {
	v := &clusterClaimValidator{}
	old := &ClusterClaim{Spec: ClusterClaimSpec{Client: ClientSpec{Enabled: true}}}
	newClaim := &ClusterClaim{Spec: ClusterClaimSpec{Client: ClientSpec{Enabled: true}}}
	_, err := v.ValidateUpdate(context.Background(), old, newClaim)
	require.NoError(t, err)
}

func infraClaim(ipam string) *ClusterClaim {
	return &ClusterClaim{Spec: ClusterClaimSpec{
		Infra: InfraSpec{Network: NetworkConfig{IpamMode: ipam}},
	}}
}

func clientClaim(network *NetworkConfig) *ClusterClaim {
	return &ClusterClaim{Spec: ClusterClaimSpec{
		Client: ClientSpec{Enabled: true, Network: network},
	}}
}

func TestValidateUpdate_InfraIpamModeImmutable(t *testing.T) {
	v := &clusterClaimValidator{}
	_, err := v.ValidateUpdate(context.Background(), infraClaim("cluster-pool"), infraClaim("multi-pool"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.infra.network.ipamMode is immutable")
}

func TestValidateUpdate_InfraIpamModeSetToUnsetRejected(t *testing.T) {
	v := &clusterClaimValidator{}
	_, err := v.ValidateUpdate(context.Background(), infraClaim("multi-pool"), infraClaim(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.infra.network.ipamMode is immutable")
}

func TestValidateUpdate_IpamModeUnsetToSetAllowed(t *testing.T) {
	v := &clusterClaimValidator{}
	_, err := v.ValidateUpdate(context.Background(), infraClaim(""), infraClaim("cluster-pool"))
	require.NoError(t, err)
}

func TestValidateUpdate_IpamModeSameValueAllowed(t *testing.T) {
	v := &clusterClaimValidator{}
	_, err := v.ValidateUpdate(context.Background(), infraClaim("multi-pool"), infraClaim("multi-pool"))
	require.NoError(t, err)
}

func TestValidateUpdate_ClientIpamModeImmutable(t *testing.T) {
	v := &clusterClaimValidator{}
	old := clientClaim(&NetworkConfig{IpamMode: "cluster-pool"})
	newClaim := clientClaim(&NetworkConfig{IpamMode: "multi-pool"})
	_, err := v.ValidateUpdate(context.Background(), old, newClaim)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.client.network.ipamMode is immutable")
}

func TestValidateUpdate_ClientNilNetworkSafe(t *testing.T) {
	v := &clusterClaimValidator{}
	_, err := v.ValidateUpdate(context.Background(), clientClaim(nil), clientClaim(&NetworkConfig{IpamMode: "multi-pool"}))
	require.NoError(t, err)
}

func TestValidateCreate_AlwaysAccepts(t *testing.T) {
	v := &clusterClaimValidator{}
	_, err := v.ValidateCreate(context.Background(), &ClusterClaim{})
	require.NoError(t, err)
}

func TestValidateDelete_AlwaysAccepts(t *testing.T) {
	v := &clusterClaimValidator{}
	_, err := v.ValidateDelete(context.Background(), &ClusterClaim{})
	require.NoError(t, err)
}
