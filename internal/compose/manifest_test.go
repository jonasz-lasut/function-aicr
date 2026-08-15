package compose

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/NVIDIA/aicr/pkg/recipe"
)

func TestRenderManifest(t *testing.T) {
	// The shape AICR's own manifest files use: values under the component's
	// key, release/chart data, and Helm's toYaml | nindent.
	const tmpl = `{{- $c := index .Values "nodewright-customizations" }}
{{- if ne (toString (index $c "enabled")) "false" }}
---
apiVersion: skyhook.nvidia.com/v1alpha1
kind: Skyhook
metadata:
  name: tuning
  namespace: {{ .Release.Namespace }}
  labels:
    app.kubernetes.io/managed-by: {{ .Release.Service }}
    helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
spec:
  accelerator: {{ $c.accelerator }}
{{- if $c.tolerations }}
  tolerations:
  {{- toYaml $c.tolerations | nindent 4 }}
{{- end }}
{{- end }}
`
	component := &recipe.ComponentRef{Name: "nodewright-customizations", Namespace: "skyhook", Type: recipe.ComponentTypeHelm, ManifestFiles: []string{"post/tuning.yaml"}}
	values := map[string]any{"accelerator": "h100", "tolerations": []any{map[string]any{"operator": "Exists"}}}

	type args struct {
		c      *recipe.ComponentRef
		values map[string]any
		ov     *Override
		data   string
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
		"RendersWithComponentValuesAndReleaseData": {
			reason: "Values are exposed under the component's key, .Release.Namespace is the component namespace, .Chart.Name falls back to the component name and .Chart.Version to AICR's default when unset.",
			args:   args{c: component, values: values, data: tmpl},
			want: want{docs: []map[string]any{{
				"apiVersion": "skyhook.nvidia.com/v1alpha1",
				"kind":       "Skyhook",
				"metadata": map[string]any{
					"name":      "tuning",
					"namespace": "skyhook",
					"labels": map[string]any{
						"app.kubernetes.io/managed-by": "Helm",
						"helm.sh/chart":                "nodewright-customizations-0.1.0",
					},
				},
				"spec": map[string]any{
					"accelerator": "h100",
					"tolerations": []any{map[string]any{"operator": "Exists"}},
				},
			}}},
		},
		"OverrideAppliesToNamespaceVersionAndValues": {
			reason: "Manifests render from the same effective namespace, version and values as the Release, so the two cannot disagree.",
			args: args{
				c:      component,
				values: values,
				ov:     &Override{Namespace: "tuning", Version: "v1.2.3", Values: map[string]any{"accelerator": "b200", "tolerations": nil}},
				data:   tmpl,
			},
			want: want{docs: []map[string]any{{
				"apiVersion": "skyhook.nvidia.com/v1alpha1",
				"kind":       "Skyhook",
				"metadata": map[string]any{
					"name":      "tuning",
					"namespace": "tuning",
					"labels": map[string]any{
						"app.kubernetes.io/managed-by": "Helm",
						"helm.sh/chart":                "nodewright-customizations-1.2.3",
					},
				},
				"spec": map[string]any{"accelerator": "b200"},
			}}},
		},
		"GatedOffRendersNoDocument": {
			reason: "A template whose condition is false renders to nothing; that is a legitimate empty result, not an error.",
			args:   args{c: component, values: map[string]any{"enabled": false}, data: tmpl},
			want:   want{docs: nil},
		},
		"PlainYAMLPassesThrough": {
			reason: "A manifest without template syntax renders to itself.",
			args:   args{c: component, values: values, data: "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: skyhook\n"},
			want:   want{docs: []map[string]any{{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "skyhook"}}}},
		},
		"BrokenTemplateErrors": {
			reason: "A template AICR cannot render is an error to surface, never a silently missing manifest.",
			args:   args{c: component, values: values, data: "{{ include \"nope\" . }}"},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rendered, err := RenderManifest(tc.args.c, tc.args.values, tc.args.ov, []byte(tc.args.data))
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("%s\nRenderManifest(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if err != nil {
				return
			}
			docs, err := SplitManifests(rendered)
			if err != nil {
				t.Fatalf("SplitManifests(rendered): %v", err)
			}
			if diff := cmp.Diff(tc.want.docs, docs); diff != "" {
				t.Errorf("%s\nRenderManifest(...) then SplitManifests: -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
