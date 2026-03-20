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

package remote

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestComputeHash(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantSame bool
		compare  []byte
	}{
		{
			name: "consistent hash for same data",
			data: []byte("kubeconfig-content"),
		},
		{
			name: "non-empty for empty data",
			data: []byte{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := computeHash(tt.data)
			hash2 := computeHash(tt.data)
			assert.NotEmpty(t, hash1)
			assert.Equal(t, hash1, hash2, "same data should produce same hash")
		})
	}

	t.Run("different hash for different data", func(t *testing.T) {
		hash1 := computeHash([]byte("data-1"))
		hash2 := computeHash([]byte("data-2"))
		assert.NotEqual(t, hash1, hash2, "different data should produce different hash")
	})
}

func TestNewClusterManager(t *testing.T) {
	scheme := runtime.NewScheme()
	cm := NewClusterManager(scheme)

	assert.NotNil(t, cm)
	assert.NotNil(t, cm.clients)
	assert.Equal(t, scheme, cm.scheme)
	assert.Equal(t, defaultTTL, cm.ttl)
}

func TestGetClient_MissingKubeconfig(t *testing.T) {
	cm := NewClusterManager(runtime.NewScheme())

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{},
	}

	_, err := cm.GetClient(context.Background(), secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig not found")
}

func TestGetClient_NilData(t *testing.T) {
	cm := NewClusterManager(runtime.NewScheme())

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
	}

	_, err := cm.GetClient(context.Background(), secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig not found")
}

func TestGetClient_InvalidKubeconfig(t *testing.T) {
	cm := NewClusterManager(runtime.NewScheme())

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"value": []byte("not-a-valid-kubeconfig"),
		},
	}

	_, err := cm.GetClient(context.Background(), secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse kubeconfig")
}

func TestRemoveExpired(t *testing.T) {
	cm := &ClusterManager{
		clients: make(map[string]*cachedClient),
		scheme:  runtime.NewScheme(),
		ttl:     1 * time.Millisecond,
	}

	// Add an entry that is already expired.
	cm.clients["ns/old"] = &cachedClient{
		hash:     "abc",
		lastUsed: time.Now().Add(-1 * time.Hour),
	}
	// Add a fresh entry.
	cm.clients["ns/fresh"] = &cachedClient{
		hash:     "def",
		lastUsed: time.Now(),
	}

	cm.removeExpired()

	assert.NotContains(t, cm.clients, "ns/old", "expired entry should be removed")
	assert.Contains(t, cm.clients, "ns/fresh", "fresh entry should remain")
}

func TestCleanup(t *testing.T) {
	cm := &ClusterManager{
		clients: make(map[string]*cachedClient),
		scheme:  runtime.NewScheme(),
		ttl:     defaultTTL,
	}

	cm.clients["ns/a"] = &cachedClient{hash: "a", lastUsed: time.Now()}
	cm.clients["ns/b"] = &cachedClient{hash: "b", lastUsed: time.Now()}

	cm.cleanup()

	assert.Empty(t, cm.clients, "cleanup should remove all entries")
}

func TestStart_StopsOnContextCancel(t *testing.T) {
	cm := NewClusterManager(runtime.NewScheme())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- cm.Start(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err, "Start should return nil on context cancellation")
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

func TestCacheHit_ReturnsSameClient(t *testing.T) {
	cm := &ClusterManager{
		clients: make(map[string]*cachedClient),
		scheme:  runtime.NewScheme(),
		ttl:     defaultTTL,
	}

	// Pre-populate cache with a mock client entry.
	kubeconfigData := []byte("some-kubeconfig")
	hash := computeHash(kubeconfigData)
	// We can't easily create a real client.Client here without a real cluster,
	// but we can verify the caching logic by placing a sentinel value.
	type fakeClient struct{ client.Client }
	sentinel := &fakeClient{}

	cm.clients["default/test"] = &cachedClient{
		client:   sentinel,
		hash:     hash,
		lastUsed: time.Now().Add(-1 * time.Minute),
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"value": kubeconfigData,
		},
	}

	c, err := cm.GetClient(context.Background(), secret)
	require.NoError(t, err)
	assert.Equal(t, sentinel, c, "cache hit should return the same client pointer")

	// Verify lastUsed was updated.
	cached := cm.clients["default/test"]
	assert.WithinDuration(t, time.Now(), cached.lastUsed, 2*time.Second, "lastUsed should be updated on cache hit")
}

func TestCacheMiss_HashChange(t *testing.T) {
	cm := &ClusterManager{
		clients: make(map[string]*cachedClient),
		scheme:  runtime.NewScheme(),
		ttl:     defaultTTL,
	}

	// Pre-populate cache with old hash.
	type fakeClient struct{ client.Client }
	sentinel := &fakeClient{}

	cm.clients["default/test"] = &cachedClient{
		client:   sentinel,
		hash:     "old-hash",
		lastUsed: time.Now(),
	}

	// New kubeconfig with different hash. It will fail because it's not a real
	// kubeconfig, but the important thing is that the hash check detects the change.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"value": []byte("new-kubeconfig-data"),
		},
	}

	// This will fail at clientcmd.RESTConfigFromKubeConfig since the data is
	// not valid, but the cache miss path is exercised (hash mismatch).
	_, err := cm.GetClient(context.Background(), secret)
	require.Error(t, err, "should fail with invalid kubeconfig data")
	assert.Contains(t, err.Error(), "parse kubeconfig", "error should come from kubeconfig parsing, not from cache")
}
