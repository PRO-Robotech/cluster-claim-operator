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
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	clusterclaimv1alpha1 "github.com/PRO-Robotech/cluster-claim-operator/api/v1alpha1"
	"github.com/PRO-Robotech/cluster-claim-operator/internal/renderer"
)

func s3BucketClaimObj(name, ns string, status map[string]interface{}) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(S3BucketClaimGVK)
	u.SetName(name)
	u.SetNamespace(ns)
	if status != nil {
		u.Object["status"] = status
	}
	return u
}

func s3Claim(withRef bool) *clusterclaimv1alpha1.ClusterClaim {
	c := &clusterclaimv1alpha1.ClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "tenant"},
	}
	if withRef {
		c.Spec.S3BucketClaimTemplateRef = &clusterclaimv1alpha1.TemplateRef{Name: "s3"}
	}
	return c
}

func s3TestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	sch := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	if err := clusterclaimv1alpha1.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	return sch
}

func TestS3BucketForClaim(t *testing.T) {
	ready := map[string]interface{}{
		"phase":      "Ready",
		"bucketName": "c1-bucket",
		"endpoint":   "s3.example.com",
	}
	cases := []struct {
		name    string
		withRef bool
		objs    []client.Object
		want    renderer.S3Bucket
	}{
		{
			// Без ссылки на шаблон объект с тем же именем не учитывается.
			name:    "no template ref ignores a same-named object",
			withRef: false,
			objs:    []client.Object{s3BucketClaimObj("c1", "tenant", ready)},
			want:    renderer.S3Bucket{},
		},
		{
			name:    "ref set but S3BucketClaim not created yet",
			withRef: true,
			want:    renderer.S3Bucket{},
		},
		{
			name:    "S3BucketClaim without status",
			withRef: true,
			objs:    []client.Object{s3BucketClaimObj("c1", "tenant", nil)},
			want:    renderer.S3Bucket{},
		},
		{
			name:    "S3BucketClaim ready",
			withRef: true,
			objs:    []client.Object{s3BucketClaimObj("c1", "tenant", ready)},
			want:    renderer.S3Bucket{Name: "c1-bucket", Endpoint: "s3.example.com", Ready: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sch := s3TestScheme(t)
			r := &ClusterClaimReconciler{
				Client: ctrlfake.NewClientBuilder().WithScheme(sch).WithObjects(tc.objs...).Build(),
				Scheme: sch,
			}
			got, err := r.s3BucketForClaim(context.Background(), s3Claim(tc.withRef))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// Ошибка чтения, отличная от NotFound, обязана дойти до вызывающего, а не
// превратиться в пустой бакет.
func TestS3BucketForClaimPropagatesReadError(t *testing.T) {
	sch := s3TestScheme(t)
	boom := errors.New("apiserver unavailable")
	c := ctrlfake.NewClientBuilder().WithScheme(sch).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			return boom
		},
	}).Build()
	r := &ClusterClaimReconciler{Client: c, Scheme: sch}

	if _, err := r.s3BucketForClaim(context.Background(), s3Claim(true)); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

// Удалённый ConfigMap перерисовывается, когда меняется бакет в контексте, и не
// трогается, когда контекст тот же.
func TestApplyRemoteConfigMapFollowsS3Bucket(t *testing.T) {
	sch := s3TestScheme(t)
	tmpl := newTestTemplate("cm-infra", "v1", "ConfigMap", `
data:
  s3Bucket: "{{ .S3Bucket.Name }}"
  s3Endpoint: "{{ .S3Bucket.Endpoint }}"
`)
	r := &ClusterClaimReconciler{
		Client: ctrlfake.NewClientBuilder().WithScheme(sch).WithObjects(tmpl).Build(),
		Scheme: sch,
	}
	remote := ctrlfake.NewClientBuilder().WithScheme(sch).Build()
	claim := s3Claim(true)
	ctx := context.Background()
	key := client.ObjectKey{Name: "parameters-infra", Namespace: "remote-ns"}

	apply := func(b renderer.S3Bucket) *corev1.ConfigMap {
		t.Helper()
		tc := renderer.TemplateContext{ClusterClaim: map[string]interface{}{}, S3Bucket: b}
		if err := r.applyRemoteConfigMap(ctx, claim, remote, "cm-infra", key.Name, key.Namespace, tc); err != nil {
			t.Fatalf("applyRemoteConfigMap: %v", err)
		}
		cm := &corev1.ConfigMap{}
		if err := remote.Get(ctx, key, cm); err != nil {
			t.Fatalf("get remote ConfigMap: %v", err)
		}
		return cm
	}

	cm := apply(renderer.S3Bucket{})
	if cm.Data["s3Bucket"] != "" || cm.Data["s3Endpoint"] != "" {
		t.Fatalf("without bucket data = %v, want empty values", cm.Data)
	}

	cm = apply(renderer.S3Bucket{Name: "c1-bucket", Endpoint: "s3.example.com", Ready: true})
	if cm.Data["s3Bucket"] != "c1-bucket" || cm.Data["s3Endpoint"] != "s3.example.com" {
		t.Fatalf("after bucket appeared data = %v", cm.Data)
	}
	rv := cm.ResourceVersion

	cm = apply(renderer.S3Bucket{Name: "c1-bucket", Endpoint: "s3.example.com", Ready: true})
	if cm.ResourceVersion != rv {
		t.Errorf("unchanged context rewrote the ConfigMap: resourceVersion %s -> %s", rv, cm.ResourceVersion)
	}
}
