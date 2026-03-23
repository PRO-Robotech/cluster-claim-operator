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

package naming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplicationName(t *testing.T) {
	tests := []struct {
		name      string
		claimName string
		want      string
	}{
		{"simple name", "ec8a00", "ec8a00"},
		{"longer name", "my-cluster-claim", "my-cluster-claim"},
		{"empty name", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ApplicationName(tt.claimName))
		})
	}
}

func TestCertificateSetName(t *testing.T) {
	tests := []struct {
		name      string
		claimName string
		role      string
		want      string
	}{
		{"infra role", "ec8a00", "infra", "ec8a00-infra"},
		{"client role", "ec8a00", "client", "ec8a00-client"},
		{"longer claim name", "my-cluster", "infra", "my-cluster-infra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CertificateSetName(tt.claimName, tt.role))
		})
	}
}

func TestClusterName(t *testing.T) {
	tests := []struct {
		name      string
		claimName string
		role      string
		want      string
	}{
		{"infra role", "ec8a00", "infra", "ec8a00-infra"},
		{"client role", "ec8a00", "client", "ec8a00-client"},
		{"longer claim name", "my-cluster", "client", "my-cluster-client"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClusterName(tt.claimName, tt.role))
		})
	}
}

func TestCcmCsrcName(t *testing.T) {
	tests := []struct {
		name      string
		claimName string
		want      string
	}{
		{"simple name", "ec8a00", "ec8a00"},
		{"longer name", "my-cluster-claim", "my-cluster-claim"},
		{"empty name", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CcmCsrcName(tt.claimName))
		})
	}
}

func TestConfigMapName(t *testing.T) {
	tests := []struct {
		name       string
		configType string
		want       string
	}{
		{"infra type", "infra", "parameters-infra"},
		{"system type", "system", "parameters-system"},
		{"client type", "client", "parameters-client"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ConfigMapName(tt.configType))
		})
	}
}

func TestKubeconfigSecretName(t *testing.T) {
	tests := []struct {
		name      string
		claimName string
		want      string
	}{
		{"simple name", "ec8a00", "ec8a00-infra-kubeconfig"},
		{"longer name", "my-cluster", "my-cluster-infra-kubeconfig"},
		{"empty name", "", "-infra-kubeconfig"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, KubeconfigSecretName(tt.claimName))
		})
	}
}
