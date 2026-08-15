package compose

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestSplitManifests(t *testing.T) {
	type args struct {
		data []byte
	}
	type want struct {
		docs []map[string]any
		err  error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"MultipleDocuments": {
			reason: "Each YAML document becomes one map.",
			args:   args{data: []byte("kind: A\n---\nkind: B\n")},
			want:   want{docs: []map[string]any{{"kind": "A"}, {"kind": "B"}}},
		},
		"BlankDocumentsSkipped": {
			reason: "Leading, trailing and empty documents contribute nothing.",
			args:   args{data: []byte("---\nkind: A\n---\n\n---\n")},
			want:   want{docs: []map[string]any{{"kind": "A"}}},
		},
		"InvalidYAMLErrors": {
			reason: "An unparseable document is reported, not skipped silently.",
			args:   args{data: []byte("kind: [unclosed\n")},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := SplitManifests(tc.args.data)
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nSplitManifests(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.docs, got); diff != "" {
				t.Errorf("%s\nSplitManifests(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestManifestObjectName(t *testing.T) {
	type args struct {
		component string
		group     string
		doc       map[string]any
		index     int
	}
	type want struct {
		name string
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"KindAndName": {
			reason: "The primary form joins component, group, lowercased kind and slugged name.",
			args: args{
				component: "gpu-operator",
				group:     "pre",
				doc:       map[string]any{"kind": "Namespace", "metadata": map[string]any{"name": "GPU.Operator"}},
			},
			want: want{name: "gpu-operator-pre-namespace-gpu-operator"},
		},
		"MissingKindFallsBackToIndex": {
			reason: "Without a kind there is nothing stable to name after, so the index is used.",
			args: args{
				component: "gpu-operator",
				group:     "post",
				doc:       map[string]any{"metadata": map[string]any{"name": "x"}},
				index:     2,
			},
			want: want{name: "gpu-operator-post-2"},
		},
		"MissingNameFallsBackToIndex": {
			reason: "Without metadata.name there is nothing stable to name after.",
			args: args{
				component: "cert-manager",
				group:     "pre",
				doc:       map[string]any{"kind": "Namespace"},
				index:     0,
			},
			want: want{name: "cert-manager-pre-0"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := ManifestObjectName(tc.args.component, tc.args.group, tc.args.doc, tc.args.index)
			if diff := cmp.Diff(tc.want.name, got); diff != "" {
				t.Errorf("%s\nManifestObjectName(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestNewObject(t *testing.T) {
	type args struct {
		manifest map[string]any
		name     string
		pcRef    ProviderConfigRef
	}
	type want struct {
		obj map[string]any
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NamespacedObject": {
			reason: "The manifest is embedded verbatim and the provider config is stamped on.",
			args: args{
				manifest: map[string]any{"apiVersion": "v1", "kind": "Namespace"},
				name:     "cert-manager-pre-namespace-cert-manager",
				pcRef:    ProviderConfigRef{Kind: "ClusterProviderConfig", Name: "gpu-cluster"},
			},
			want: want{obj: map[string]any{
				"apiVersion": "kubernetes.m.crossplane.io/v1alpha1",
				"kind":       "Object",
				"metadata":   map[string]any{"name": "cert-manager-pre-namespace-cert-manager"},
				"spec": map[string]any{
					"forProvider": map[string]any{
						"manifest": map[string]any{"apiVersion": "v1", "kind": "Namespace"},
					},
					"providerConfigRef": map[string]any{
						"kind": "ClusterProviderConfig",
						"name": "gpu-cluster",
					},
				},
			}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := NewObject(tc.args.manifest, tc.args.name, tc.args.pcRef)
			if diff := cmp.Diff(tc.want.obj, got); diff != "" {
				t.Errorf("%s\nNewObject(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
