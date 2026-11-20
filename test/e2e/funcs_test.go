package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// The namespace the features create their composite resources in, and so the
// namespace of everything function-aicr composes for them.
const xrNamespace = "default"

// release returns the provider-helm Release function-aicr composes for a
// component; it has the component's name.
func release(component string) *unstructured.Unstructured {
	return composed(schema.GroupVersionKind{Group: "helm.m.crossplane.io", Version: "v1beta1", Kind: "Release"}, component)
}

// object returns a provider-kubernetes Object function-aicr composes for a
// document of a component's manifest files.
func object(name string) *unstructured.Unstructured {
	return composed(schema.GroupVersionKind{Group: "kubernetes.m.crossplane.io", Version: "v1alpha1", Kind: "Object"}, name)
}

func composed(gvk schema.GroupVersionKind, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetNamespace(xrNamespace)
	u.SetName(name)
	return u
}

// configMap returns a ConfigMap of the cluster the recipe is deployed to,
// which in the suite is the cluster Crossplane runs in.
func configMap(namespace, name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}
}

// deployment returns a Deployment of the cluster the recipe is deployed to.
func deployment(namespace, name string) *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}
}

// ResourceIsAbsent fails if the supplied resource exists.
func ResourceIsAbsent(o k8s.Object) features.Func {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		t.Helper()

		err := c.Client().Resources().Get(ctx, o.GetName(), o.GetNamespace(), o)
		switch {
		case kerrors.IsNotFound(err):
			t.Logf("%s %s/%s is absent", o.GetObjectKind().GroupVersionKind().Kind, o.GetNamespace(), o.GetName())
		case err != nil:
			t.Fatalf("cannot get %s %s/%s: %v", o.GetObjectKind().GroupVersionKind().Kind, o.GetNamespace(), o.GetName(), err)
		default:
			t.Errorf("%s %s/%s exists, but must not", o.GetObjectKind().GroupVersionKind().Kind, o.GetNamespace(), o.GetName())
		}

		return ctx
	}
}

// LabelNodes sets a label on every node of the cluster.
func LabelNodes(key, value string) features.Func {
	return patchNodeLabel(key, value)
}

// UnlabelNodes removes a label from every node of the cluster.
func UnlabelNodes(key string) features.Func {
	return patchNodeLabel(key, nil)
}

// patchNodeLabel merges a label into every node; a nil value removes it.
func patchNodeLabel(key string, value any) features.Func {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		t.Helper()

		patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"labels": map[string]any{key: value}}})
		if err != nil {
			t.Fatalf("cannot marshal the label patch: %v", err)
		}

		nodes := &corev1.NodeList{}
		if err := c.Client().Resources().List(ctx, nodes); err != nil {
			t.Fatalf("cannot list nodes: %v", err)
		}

		for i := range nodes.Items {
			node := &nodes.Items[i]
			if err := c.Client().Resources().Patch(ctx, node, k8s.Patch{PatchType: types.MergePatchType, Data: patch}); err != nil {
				t.Fatalf("cannot patch the labels of node %s: %v", node.GetName(), err)
			}

			t.Logf("node %s: label %s set to %v", node.GetName(), key, value)
		}

		return ctx
	}
}

// DeleteNamespaces deletes the named namespaces, with everything in them, and
// waits until they are gone. A namespace that does not exist is skipped.
func DeleteNamespaces(d time.Duration, names ...string) features.Func {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		t.Helper()

		for _, name := range names {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}

			err := c.Client().Resources().Delete(ctx, ns)
			if kerrors.IsNotFound(err) {
				continue
			}

			if err != nil {
				t.Fatalf("cannot delete namespace %s: %v", name, err)
			}

			if err := wait.For(conditions.New(c.Client().Resources()).ResourceDeleted(ns), wait.WithTimeout(d), wait.WithContext(ctx)); err != nil {
				t.Fatalf("namespace %s not deleted within %s: %v", name, d, err)
			}

			t.Logf("namespace %s is deleted", name)
		}

		return ctx
	}
}
