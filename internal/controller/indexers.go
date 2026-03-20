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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
)

const (
	indexObserveTemplateRef         = "spec.observeTemplateRef.name"
	indexCertSetInfraTemplateRef    = "spec.certificateSetTemplateRef.infra.name"
	indexCertSetClientTemplateRef   = "spec.certificateSetTemplateRef.client.name"
	indexClusterInfraTemplateRef    = "spec.clusterTemplateRef.infra.name"
	indexClusterClientTemplateRef   = "spec.clusterTemplateRef.client.name"
	indexCcmCsrTemplateRef          = "spec.ccmCsrTemplateRef.name"
	indexConfigMapInfraTemplateRef  = "spec.configMapTemplateRef.infra.name"
	indexConfigMapClientTemplateRef = "spec.configMapTemplateRef.client.name"
)

// allIndexFields returns the list of all templateRef index field names.
func allIndexFields() []string {
	return []string{
		indexObserveTemplateRef,
		indexCertSetInfraTemplateRef,
		indexCertSetClientTemplateRef,
		indexClusterInfraTemplateRef,
		indexClusterClientTemplateRef,
		indexCcmCsrTemplateRef,
		indexConfigMapInfraTemplateRef,
		indexConfigMapClientTemplateRef,
	}
}

// SetupIndexers registers all field indexers for ClusterClaim templateRef fields.
// These indexers enable efficient lookup of ClusterClaims by template name when
// a ClusterClaimObserveResourceTemplate changes.
func SetupIndexers(mgr ctrl.Manager) error {
	indexer := mgr.GetFieldIndexer()

	indexes := []struct {
		field string
		fn    client.IndexerFunc
	}{
		{
			field: indexObserveTemplateRef,
			fn: func(obj client.Object) []string {
				claim := obj.(*clusterclaimv1alpha1.ClusterClaim)
				return []string{claim.Spec.ObserveTemplateRef.Name}
			},
		},
		{
			field: indexCertSetInfraTemplateRef,
			fn: func(obj client.Object) []string {
				claim := obj.(*clusterclaimv1alpha1.ClusterClaim)
				return []string{claim.Spec.CertificateSetTemplateRef.Infra.Name}
			},
		},
		{
			field: indexCertSetClientTemplateRef,
			fn: func(obj client.Object) []string {
				claim := obj.(*clusterclaimv1alpha1.ClusterClaim)
				if claim.Spec.CertificateSetTemplateRef.Client == nil {
					return nil
				}
				return []string{claim.Spec.CertificateSetTemplateRef.Client.Name}
			},
		},
		{
			field: indexClusterInfraTemplateRef,
			fn: func(obj client.Object) []string {
				claim := obj.(*clusterclaimv1alpha1.ClusterClaim)
				return []string{claim.Spec.ClusterTemplateRef.Infra.Name}
			},
		},
		{
			field: indexClusterClientTemplateRef,
			fn: func(obj client.Object) []string {
				claim := obj.(*clusterclaimv1alpha1.ClusterClaim)
				if claim.Spec.ClusterTemplateRef.Client == nil {
					return nil
				}
				return []string{claim.Spec.ClusterTemplateRef.Client.Name}
			},
		},
		{
			field: indexCcmCsrTemplateRef,
			fn: func(obj client.Object) []string {
				claim := obj.(*clusterclaimv1alpha1.ClusterClaim)
				return []string{claim.Spec.CcmCsrTemplateRef.Name}
			},
		},
		{
			field: indexConfigMapInfraTemplateRef,
			fn: func(obj client.Object) []string {
				claim := obj.(*clusterclaimv1alpha1.ClusterClaim)
				if claim.Spec.ConfigMapTemplateRef == nil {
					return nil
				}
				return []string{claim.Spec.ConfigMapTemplateRef.Infra.Name}
			},
		},
		{
			field: indexConfigMapClientTemplateRef,
			fn: func(obj client.Object) []string {
				claim := obj.(*clusterclaimv1alpha1.ClusterClaim)
				if claim.Spec.ConfigMapTemplateRef == nil || claim.Spec.ConfigMapTemplateRef.Client == nil {
					return nil
				}
				return []string{claim.Spec.ConfigMapTemplateRef.Client.Name}
			},
		},
	}

	for _, idx := range indexes {
		if err := indexer.IndexField(context.Background(), &clusterclaimv1alpha1.ClusterClaim{}, idx.field, idx.fn); err != nil {
			return err
		}
	}
	return nil
}
