package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestWireDRAEviction(t *testing.T) {
	const label = "nvidia.com/dra-kubelet-plugin"
	evictionEnv := map[string]any{"name": "NODE_LABEL_FOR_GPU_POD_EVICTION", "value": label}

	type args struct {
		operator map[string]any
		driver   map[string]any
		labeler  map[string]any
	}
	type want struct {
		operator map[string]any
		driver   map[string]any
		labeler  map[string]any
		err      error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"EmptyValues": {
			reason: "With nothing set yet, the three halves of the contract are created: the kubelet plugin's node selector, the Driver Manager's environment variable, and the enabled labeler.",
			args:   args{},
			want: want{
				operator: map[string]any{"driver": map[string]any{"manager": map[string]any{"env": []any{evictionEnv}}}},
				driver:   map[string]any{"kubeletPlugin": map[string]any{"nodeSelector": map[string]any{label: "true"}}},
				labeler:  map[string]any{"labelKey": label, "labelValue": "true", "enabled": true},
			},
		},
		"KeepsWhatIsThere": {
			reason: "Other node selectors, environment entries and values are kept; an existing eviction variable is rewritten in place and a duplicate of it dropped.",
			args: args{
				operator: map[string]any{"driver": map[string]any{
					"version": "580",
					"manager": map[string]any{"env": []any{
						map[string]any{"name": "ENABLE_AUTO_DRAIN", "value": "true"},
						map[string]any{"name": "NODE_LABEL_FOR_GPU_POD_EVICTION", "valueFrom": map[string]any{"configMapKeyRef": "x"}},
						map[string]any{"name": "NODE_LABEL_FOR_GPU_POD_EVICTION", "value": "stale"},
					}},
				}},
				driver:  map[string]any{"kubeletPlugin": map[string]any{"priorityClassName": "", "nodeSelector": map[string]any{"pool": "gpu"}}},
				labeler: map[string]any{"enabled": false, "labelKey": "old"},
			},
			want: want{
				operator: map[string]any{"driver": map[string]any{
					"version": "580",
					"manager": map[string]any{"env": []any{
						map[string]any{"name": "ENABLE_AUTO_DRAIN", "value": "true"},
						evictionEnv,
					}},
				}},
				driver:  map[string]any{"kubeletPlugin": map[string]any{"priorityClassName": "", "nodeSelector": map[string]any{"pool": "gpu", label: "true"}}},
				labeler: map[string]any{"labelKey": label, "labelValue": "true", "enabled": true},
			},
		},
		"EnvNotAListErrors": {
			reason: "A driver.manager.env that is not a list cannot take the variable; guessing would drop what the user set.",
			args:   args{operator: map[string]any{"driver": map[string]any{"manager": map[string]any{"env": "ENABLE_AUTO_DRAIN=true"}}}},
			want:   want{err: cmpopts.AnyError},
		},
		"NodeSelectorNotAnObjectErrors": {
			reason: "A kubeletPlugin.nodeSelector that is not an object cannot take the label.",
			args:   args{driver: map[string]any{"kubeletPlugin": map[string]any{"nodeSelector": "pool=gpu"}}},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			before := []map[string]any{deepCopy(tc.args.operator), deepCopy(tc.args.driver), deepCopy(tc.args.labeler)}

			operator, driver, labeler, err := WireDRAEviction(tc.args.operator, tc.args.driver, tc.args.labeler)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nWireDRAEviction(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.operator, operator); diff != "" {
				t.Errorf("%s\nWireDRAEviction(...): -want operator, +got operator:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.driver, driver); diff != "" {
				t.Errorf("%s\nWireDRAEviction(...): -want driver, +got driver:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.labeler, labeler); diff != "" {
				t.Errorf("%s\nWireDRAEviction(...): -want labeler, +got labeler:\n%s", tc.reason, diff)
			}
			after := []map[string]any{tc.args.operator, tc.args.driver, tc.args.labeler}
			if diff := cmp.Diff(before, after); diff != "" {
				t.Errorf("WireDRAEviction(...) mutated its arguments: -before, +after:\n%s", diff)
			}
		})
	}
}

func deepCopy(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch tv := v.(type) {
		case map[string]any:
			out[k] = deepCopy(tv)
		case []any:
			list := make([]any, len(tv))
			for i, e := range tv {
				if em, ok := e.(map[string]any); ok {
					list[i] = deepCopy(em)
				} else {
					list[i] = e
				}
			}
			out[k] = list
		default:
			out[k] = v
		}
	}
	return out
}

// TestDRAEvictionConstants compares the component names and value paths
// WireDRAEviction writes with the constants AICR's bundler declares for its
// own wiring. AICR keeps them unexported, so they are read from the source of
// the aicr.run module the function is built with. A failure after an AICR
// bump means the bundler's contract moved: re-read injectDRAEvictionLabel and
// its helpers in pkg/bundler/bundler.go and bring WireDRAEviction in line.
func TestDRAEvictionConstants(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-m", "-f", "{{.Dir}}", "aicr.run").Output()
	if err != nil {
		t.Fatalf("go list -m aicr.run: %v", err)
	}
	path := filepath.Join(strings.TrimSpace(string(out)), "pkg", "bundler", "bundler.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("cannot parse AICR's bundler source: %v", err)
	}
	upstream := make(map[string]string)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if v, err := strconv.Unquote(lit.Value); err == nil {
					upstream[name.Name] = v
				}
			}
		}
	}

	cases := map[string]struct {
		reason string
		ours   string
	}{
		"gpuOperatorComponentName":      {reason: "The component whose Driver Manager evicts the plugin.", ours: GPUOperatorComponent},
		"draComponentName":              {reason: "The component whose kubelet plugin is evicted.", ours: DRADriverComponent},
		"draNodeLabelerComponentName":   {reason: "The component that applies the eviction label.", ours: DRANodeLabelerComponent},
		"draEvictionEnvName":            {reason: "The variable that tells the Driver Manager the label key.", ours: draEvictionEnvName},
		"draEvictionNodeSelectorPath":   {reason: "Where the DRA driver chart takes the kubelet plugin's node selector.", ours: draEvictionNodeSelectorPath},
		"gpuOperatorDRAEvictionEnvPath": {reason: "Where the GPU Operator chart takes the Driver Manager's environment.", ours: draEvictionEnvPath},
		"draNodeLabelerKeyPath":         {reason: "The labeler value that names the label key.", ours: draNodeLabelerKeyPath},
		"draNodeLabelerValuePath":       {reason: "The labeler value that names the label value.", ours: draNodeLabelerValuePath},
		"draNodeLabelerEnabledPath":     {reason: "The labeler's render gate.", ours: draNodeLabelerEnabledPath},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			theirs, ok := upstream[name]
			if !ok {
				t.Fatalf("%s\nAICR's bundler no longer declares the constant %s", tc.reason, name)
			}
			if diff := cmp.Diff(theirs, tc.ours); diff != "" {
				t.Errorf("%s\n%s: -AICR's bundler, +WireDRAEviction:\n%s", tc.reason, name, diff)
			}
		})
	}
}
