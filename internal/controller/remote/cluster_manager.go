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
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultTTL      = 5 * time.Minute
	cleanupInterval = 1 * time.Minute
)

type cachedClient struct {
	client   client.Client
	hash     string
	lastUsed time.Time
}

// ClusterManager maintains a cache of Kubernetes clients for remote clusters.
// It implements manager.Runnable so the controller-runtime manager handles
// its lifecycle (background cleanup goroutine).
type ClusterManager struct {
	mu      sync.RWMutex
	clients map[string]*cachedClient
	scheme  *runtime.Scheme
	ttl     time.Duration
}

// NewClusterManager creates a new ClusterManager with the default TTL.
func NewClusterManager(scheme *runtime.Scheme) *ClusterManager {
	return &ClusterManager{
		clients: make(map[string]*cachedClient),
		scheme:  scheme,
		ttl:     defaultTTL,
	}
}

// GetClient returns a cached or new client for the given kubeconfig Secret.
// The Secret must contain a "value" key with the kubeconfig data.
func (cm *ClusterManager) GetClient(ctx context.Context, secret *corev1.Secret) (client.Client, error) {
	kubeconfigData := secret.Data["value"]
	if len(kubeconfigData) == 0 {
		return nil, fmt.Errorf("kubeconfig not found in secret %s/%s", secret.Namespace, secret.Name)
	}

	key := secret.Namespace + "/" + secret.Name
	hash := computeHash(kubeconfigData)

	// Check cache with read lock.
	cm.mu.RLock()
	if cached, ok := cm.clients[key]; ok && cached.hash == hash {
		cached.lastUsed = time.Now()
		cm.mu.RUnlock()
		return cached.client, nil
	}
	cm.mu.RUnlock()

	// Create new client under write lock.
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Double-check after acquiring write lock.
	if cached, ok := cm.clients[key]; ok && cached.hash == hash {
		cached.lastUsed = time.Now()
		return cached.client, nil
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	c, err := client.New(restConfig, client.Options{Scheme: cm.scheme})
	if err != nil {
		return nil, fmt.Errorf("create remote client: %w", err)
	}

	cm.clients[key] = &cachedClient{
		client:   c,
		hash:     hash,
		lastUsed: time.Now(),
	}

	return c, nil
}

// Start begins the background cleanup goroutine. Implements manager.Runnable.
func (cm *ClusterManager) Start(ctx context.Context) error {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			cm.cleanup()
			return nil
		case <-ticker.C:
			cm.removeExpired()
		}
	}
}

func (cm *ClusterManager) removeExpired() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	now := time.Now()
	for key, cached := range cm.clients {
		if now.Sub(cached.lastUsed) > cm.ttl {
			delete(cm.clients, key)
		}
	}
}

func (cm *ClusterManager) cleanup() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.clients = make(map[string]*cachedClient)
}

func computeHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8])
}
