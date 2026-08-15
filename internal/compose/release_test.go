package compose

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/NVIDIA/aicr/pkg/recipe"
)

func TestSplitChart(t *testing.T) {
	type args struct {
		source string
		chart  string
	}
	type want struct {
		repository string
		name       string
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"HTTPRepository": {
			reason: "An HTTP source is the repository as-is.",
			args:   args{source: "https://charts.jetstack.io", chart: "cert-manager"},
			want:   want{repository: "https://charts.jetstack.io", name: "cert-manager"},
		},
		"OCIRepositoryRepeatingChart": {
			reason: "provider-helm appends the chart name, so an OCI path ending in it must be trimmed.",
			args:   args{source: "oci://ghcr.io/nvidia/kai-scheduler", chart: "kai-scheduler"},
			want:   want{repository: "oci://ghcr.io/nvidia", name: "kai-scheduler"},
		},
		"OCIRepositoryWithoutChart": {
			reason: "An OCI path not ending in the chart name is left alone.",
			args:   args{source: "oci://ghcr.io/nvidia", chart: "nvsentinel"},
			want:   want{repository: "oci://ghcr.io/nvidia", name: "nvsentinel"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gotRepo, gotName := SplitChart(tc.args.source, tc.args.chart)
			if diff := cmp.Diff(tc.want.repository, gotRepo); diff != "" {
				t.Errorf("%s\nSplitChart(...): -want repository, +got repository:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.name, gotName); diff != "" {
				t.Errorf("%s\nSplitChart(...): -want name, +got name:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestMergeValues(t *testing.T) {
	type args struct {
		base     map[string]any
		override map[string]any
	}
	type want struct {
		values map[string]any
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NestedMapsMergeRecursively": {
			reason: "Override wins per leaf; sibling keys survive.",
			args: args{
				base:     map[string]any{"driver": map[string]any{"enabled": true, "version": "1"}},
				override: map[string]any{"driver": map[string]any{"version": "2"}},
			},
			want: want{values: map[string]any{"driver": map[string]any{"enabled": true, "version": "2"}}},
		},
		"NonMapTypesReplace": {
			reason: "A scalar override replaces a map, and vice versa.",
			args: args{
				base:     map[string]any{"driver": map[string]any{"version": "1"}},
				override: map[string]any{"driver": "off"},
			},
			want: want{values: map[string]any{"driver": "off"}},
		},
		"BaseIsNotMutated": {
			reason: "MergeValues returns a new map.",
			args: args{
				base:     map[string]any{"a": 1},
				override: map[string]any{"b": 2},
			},
			want: want{values: map[string]any{"a": 1, "b": 2}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := MergeValues(tc.args.base, tc.args.override)
			if diff := cmp.Diff(tc.want.values, got); diff != "" {
				t.Errorf("%s\nMergeValues(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestNewRelease(t *testing.T) {
	pcRef := ProviderConfigRef{Kind: "ClusterProviderConfig", Name: "gpu-cluster"}

	type args struct {
		c      *recipe.ComponentRef
		values map[string]any
		ov     *Override
	}
	type want struct {
		rel map[string]any
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NoOverride": {
			reason: "The component's own namespace, version and values are used, and wait is always set.",
			args: args{
				c: &recipe.ComponentRef{
					Name:      "cert-manager",
					Namespace: "cert-manager",
					Chart:     "cert-manager",
					Source:    "https://charts.jetstack.io",
					Version:   "v1.20.2",
				},
				values: map[string]any{"crds": map[string]any{"enabled": true}},
			},
			want: want{rel: map[string]any{
				"apiVersion": "helm.m.crossplane.io/v1beta1",
				"kind":       "Release",
				"metadata":   map[string]any{"name": "cert-manager"},
				"spec": map[string]any{
					"forProvider": map[string]any{
						"chart": map[string]any{
							"name":       "cert-manager",
							"repository": "https://charts.jetstack.io",
							"version":    "v1.20.2",
						},
						"namespace": "cert-manager",
						"wait":      true,
						"values":    map[string]any{"crds": map[string]any{"enabled": true}},
					},
					"providerConfigRef": map[string]any{
						"kind": "ClusterProviderConfig",
						"name": "gpu-cluster",
					},
				},
			}},
		},
		"OverrideWins": {
			reason: "An override retargets namespace and version and deep-merges values.",
			args: args{
				c: &recipe.ComponentRef{
					Name:      "gpu-operator",
					Namespace: "nvidia",
					Chart:     "gpu-operator",
					Source:    "https://helm.ngc.nvidia.com/nvidia",
					Version:   "v25.3.3",
				},
				values: map[string]any{"driver": map[string]any{"enabled": true, "version": "1"}},
				ov: &Override{
					Version:   "v99.0.0",
					Namespace: "gpu-operator",
					Values:    map[string]any{"driver": map[string]any{"version": "999"}},
				},
			},
			want: want{rel: map[string]any{
				"apiVersion": "helm.m.crossplane.io/v1beta1",
				"kind":       "Release",
				"metadata":   map[string]any{"name": "gpu-operator"},
				"spec": map[string]any{
					"forProvider": map[string]any{
						"chart": map[string]any{
							"name":       "gpu-operator",
							"repository": "https://helm.ngc.nvidia.com/nvidia",
							"version":    "v99.0.0",
						},
						"namespace": "gpu-operator",
						"wait":      true,
						"values":    map[string]any{"driver": map[string]any{"enabled": true, "version": "999"}},
					},
					"providerConfigRef": map[string]any{
						"kind": "ClusterProviderConfig",
						"name": "gpu-cluster",
					},
				},
			}},
		},
		"SourceOnlyRefUsesComponentName": {
			reason: "AICR lets a Helm ref omit chart when it equals the component name (EffectiveChart); the Release must not carry an empty chart name.",
			args: args{
				c: &recipe.ComponentRef{
					Name:      "nvsentinel",
					Namespace: "nvsentinel",
					Source:    "oci://ghcr.io/nvidia",
					Version:   "v0.6.0",
				},
			},
			want: want{rel: map[string]any{
				"apiVersion": "helm.m.crossplane.io/v1beta1",
				"kind":       "Release",
				"metadata":   map[string]any{"name": "nvsentinel"},
				"spec": map[string]any{
					"forProvider": map[string]any{
						"chart": map[string]any{
							"name":       "nvsentinel",
							"repository": "oci://ghcr.io/nvidia",
							"version":    "v0.6.0",
						},
						"namespace": "nvsentinel",
						"wait":      true,
					},
					"providerConfigRef": map[string]any{
						"kind": "ClusterProviderConfig",
						"name": "gpu-cluster",
					},
				},
			}},
		},
		"NoValuesOrVersion": {
			reason: "Empty values and version are omitted entirely rather than emitted as empty fields.",
			args: args{
				c: &recipe.ComponentRef{
					Name:      "kai-scheduler",
					Namespace: "kai",
					Chart:     "kai-scheduler",
					Source:    "oci://ghcr.io/nvidia/kai-scheduler",
				},
			},
			want: want{rel: map[string]any{
				"apiVersion": "helm.m.crossplane.io/v1beta1",
				"kind":       "Release",
				"metadata":   map[string]any{"name": "kai-scheduler"},
				"spec": map[string]any{
					"forProvider": map[string]any{
						"chart": map[string]any{
							"name":       "kai-scheduler",
							"repository": "oci://ghcr.io/nvidia",
						},
						"namespace": "kai",
						"wait":      true,
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
			got := NewRelease(tc.args.c, tc.args.values, tc.args.ov, pcRef)
			if diff := cmp.Diff(tc.want.rel, got); diff != "" {
				t.Errorf("%s\nNewRelease(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
