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
	"context"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
)

// findClaimsForTemplate returns reconcile requests for all ClusterClaims that
// reference the given ClusterClaimObserveResourceTemplate. It queries each of
// the 8 templateRef field indexes and deduplicates results.
func (r *ClusterClaimReconciler) findClaimsForTemplate(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	templateName := obj.GetName()

	seen := make(map[string]struct{})
	var requests []reconcile.Request

	for _, field := range allIndexFields() {
		var claims clusterclaimv1alpha1.ClusterClaimList
		if err := r.List(ctx, &claims, client.MatchingFields{field: templateName}); err != nil {
			logger.Error(err, "failed to list ClusterClaims by index", "field", field, "template", templateName)
			continue
		}
		for i := range claims.Items {
			claim := &claims.Items[i]
			key := claim.Namespace + "/" + claim.Name
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(claim),
			})
		}
	}

	if len(requests) > 0 {
		logger.Info("template changed, enqueuing referencing claims",
			"template", templateName, "count", len(requests))
	}

	return requests
}

// findClaimForSecret returns a reconcile request for the ClusterClaim that owns
// the kubeconfig Secret. Kubeconfig secrets follow the naming pattern
// "{claimName}-infra-kubeconfig".
func (r *ClusterClaimReconciler) findClaimForSecret(_ context.Context, obj client.Object) []reconcile.Request {
	secretName := obj.GetName()
	const suffix = "-infra-kubeconfig"
	if !strings.HasSuffix(secretName, suffix) {
		return nil
	}
	claimName := strings.TrimSuffix(secretName, suffix)
	return []reconcile.Request{
		{NamespacedName: client.ObjectKey{Name: claimName, Namespace: obj.GetNamespace()}},
	}
}
