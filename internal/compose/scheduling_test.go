package compose

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"

	"aicr.run/pkg/recipe"
)

func TestApplyNodeScheduling(t *testing.T) {
	system := NodePlacement{
		NodeSelector: map[string]string{"pool": "system"},
		Tolerations:  []corev1.Toleration{{Key: "dedicated", Operator: corev1.TolerationOpEqual, Value: "system", Effect: corev1.TaintEffectNoSchedule}},
	}
	accelerated := NodePlacement{
		NodeSelector: map[string]string{"pool": "gpu"},
		Tolerations:  []corev1.Toleration{{Key: "nvidia.com/gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}},
	}
	systemToleration := map[string]any{"key": "dedicated", "operator": "Equal", "value": "system", "effect": "NoSchedule"}
	acceleratedToleration := map[string]any{"key": "nvidia.com/gpu", "operator": "Exists", "effect": "NoSchedule"}

	// A component in the shape of gpu-operator's registry entry: the operator
	// is a system workload and the daemonsets run on accelerated nodes.
	operator := &recipe.ComponentConfig{
		Name: "gpu-operator",
		NodeScheduling: recipe.NodeSchedulingConfig{
			System: recipe.SchedulingPaths{
				NodeSelectorPaths: []string{"operator.nodeSelector"},
				TolerationPaths:   []string{"operator.tolerations"},
			},
			Accelerated: recipe.SchedulingPaths{
				NodeSelectorPaths: []string{"daemonsets.nodeSelector"},
				TolerationPaths:   []string{"daemonsets.tolerations"},
			},
		},
	}
	// A component whose workload runs on every node, as nfd's worker does.
	everywhere := &recipe.ComponentConfig{
		Name: "nfd",
		NodeScheduling: recipe.NodeSchedulingConfig{
			System:      recipe.SchedulingPaths{TolerationPaths: []string{"worker.tolerations"}},
			Accelerated: recipe.SchedulingPaths{TolerationPaths: []string{"worker.tolerations"}},
		},
	}

	type args struct {
		values     map[string]any
		comp       *recipe.ComponentConfig
		ref        *recipe.ComponentRef
		scheduling NodeScheduling
	}
	type want struct {
		values   map[string]any
		optedOut map[string]bool
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NothingSet": {
			reason: "Without an input the values are the recipe's.",
			args: args{
				values: map[string]any{"operator": map[string]any{"replicas": 1}},
				comp:   operator,
				ref:    &recipe.ComponentRef{Name: "gpu-operator"},
			},
			want: want{values: map[string]any{"operator": map[string]any{"replicas": 1}}},
		},
		"EachKindOfWorkloadGetsItsPlacement": {
			reason: "The system placement is written to the registry's system paths and the accelerated one to its accelerated paths, replacing what the values file has there.",
			args: args{
				values: map[string]any{
					"operator":   map[string]any{"replicas": 1, "tolerations": []any{map[string]any{"operator": "Exists"}}},
					"daemonsets": map[string]any{"nodeSelector": map[string]any{"from": "the-chart"}},
				},
				comp:       operator,
				ref:        &recipe.ComponentRef{Name: "gpu-operator"},
				scheduling: NodeScheduling{System: system, Accelerated: accelerated},
			},
			want: want{values: map[string]any{
				"operator": map[string]any{
					"replicas":     1,
					"nodeSelector": map[string]any{"pool": "system"},
					"tolerations":  []any{systemToleration},
				},
				"daemonsets": map[string]any{
					"nodeSelector": map[string]any{"pool": "gpu"},
					"tolerations":  []any{acceleratedToleration},
				},
			}},
		},
		"RecipeOptsAPathOut": {
			reason: "A recipe that overrides a path with an empty value keeps the chart's default there, as the kind overlay does to keep GPU operands off the control plane.",
			args: args{
				values: map[string]any{"daemonsets": map[string]any{"tolerations": []any{}}},
				comp:   operator,
				ref: &recipe.ComponentRef{Name: "gpu-operator", Overrides: map[string]any{
					"daemonsets": map[string]any{"tolerations": []any{}},
				}},
				scheduling: NodeScheduling{Accelerated: accelerated},
			},
			want: want{
				values: map[string]any{"daemonsets": map[string]any{
					"nodeSelector": map[string]any{"pool": "gpu"},
					"tolerations":  []any{},
				}},
				optedOut: map[string]bool{"daemonsets.tolerations": true},
			},
		},
		"RecipeTolerationsAreKept": {
			reason: "A recipe that overrides a path with tolerations of its own means them as well: the input's are appended.",
			args: args{
				values: map[string]any{"daemonsets": map[string]any{"tolerations": []any{map[string]any{"key": "from-the-recipe", "operator": "Exists"}}}},
				comp:   operator,
				ref: &recipe.ComponentRef{Name: "gpu-operator", Overrides: map[string]any{
					"daemonsets": map[string]any{"tolerations": []any{map[string]any{"key": "from-the-recipe", "operator": "Exists"}}},
				}},
				scheduling: NodeScheduling{Accelerated: NodePlacement{Tolerations: accelerated.Tolerations}},
			},
			want: want{values: map[string]any{"daemonsets": map[string]any{
				"tolerations": []any{map[string]any{"key": "from-the-recipe", "operator": "Exists"}, acceleratedToleration},
			}}},
		},
		"SharedPathGetsBoth": {
			reason: "A path the registry names for both kinds of workload belongs to something that runs on every node, so it tolerates both.",
			args: args{
				values:     map[string]any{},
				comp:       everywhere,
				ref:        &recipe.ComponentRef{Name: "nfd"},
				scheduling: NodeScheduling{System: system, Accelerated: accelerated},
			},
			want: want{values: map[string]any{"worker": map[string]any{
				"tolerations": []any{systemToleration, acceleratedToleration},
			}}},
		},
		"UnknownComponent": {
			reason: "A component the registry does not know has no paths to write to.",
			args: args{
				values:     map[string]any{"replicas": 1},
				ref:        &recipe.ComponentRef{Name: "custom"},
				scheduling: NodeScheduling{System: system, Accelerated: accelerated},
			},
			want: want{values: map[string]any{"replicas": 1}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			optedOut := ApplyNodeScheduling(tc.args.values, tc.args.comp, tc.args.ref, tc.args.scheduling)

			if diff := cmp.Diff(tc.want.values, tc.args.values); diff != "" {
				t.Errorf("%s\nApplyNodeScheduling(...): -want values, +got values:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.optedOut, optedOut, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("%s\nApplyNodeScheduling(...): -want opted out, +got opted out:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestRequireNodeSelectors(t *testing.T) {
	// A component in the shape of slinky-slurm's registry entry, whose
	// StatefulSets are tied to the zone of their volumes.
	required := &recipe.ComponentConfig{
		Name: "slinky-slurm",
		NodeScheduling: recipe.NodeSchedulingConfig{
			System:      recipe.SchedulingPaths{NodeSelectorPaths: []string{"controller.nodeSelector"}, RequireNodeSelector: true},
			Accelerated: recipe.SchedulingPaths{NodeSelectorPaths: []string{"nodesets.gpu.nodeSelector"}, RequireNodeSelector: true},
		},
	}
	optional := &recipe.ComponentConfig{
		Name:           "cert-manager",
		NodeScheduling: recipe.NodeSchedulingConfig{System: recipe.SchedulingPaths{NodeSelectorPaths: []string{"nodeSelector"}}},
	}
	selector := map[string]any{"pool": "system"}

	type args struct {
		values   map[string]any
		comp     *recipe.ComponentConfig
		optedOut map[string]bool
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NotRequired": {
			reason: "Most components deploy without a selector.",
			args:   args{values: map[string]any{}, comp: optional},
		},
		"Satisfied": {
			reason: "A selector at every required path satisfies the registry.",
			args: args{
				values: map[string]any{
					"controller": map[string]any{"nodeSelector": selector},
					"nodesets":   map[string]any{"gpu": map[string]any{"nodeSelector": selector}},
				},
				comp: required,
			},
		},
		"MissingSystemSelector": {
			reason: "A required selector that is absent is an error naming the Input field that sets it.",
			args: args{
				values: map[string]any{"nodesets": map[string]any{"gpu": map[string]any{"nodeSelector": selector}}},
				comp:   required,
			},
			want: want{err: cmpopts.AnyError},
		},
		"EmptyAcceleratedSelector": {
			reason: "An empty selector, as an override can leave behind, is no selector.",
			args: args{
				values: map[string]any{
					"controller": map[string]any{"nodeSelector": selector},
					"nodesets":   map[string]any{"gpu": map[string]any{"nodeSelector": map[string]any{}}},
				},
				comp: required,
			},
			want: want{err: cmpopts.AnyError},
		},
		"OptedOutByTheRecipe": {
			reason: "A path the recipe opts out is the recipe author's decision to deploy without a selector there.",
			args: args{
				values:   map[string]any{},
				comp:     required,
				optedOut: map[string]bool{"controller.nodeSelector": true, "nodesets.gpu.nodeSelector": true},
			},
		},
		"UnknownComponent": {
			reason: "A component the registry does not know requires nothing.",
			args:   args{values: map[string]any{}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := RequireNodeSelectors("component", tc.args.values, tc.args.comp, tc.args.optedOut)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nRequireNodeSelectors(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}
