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
