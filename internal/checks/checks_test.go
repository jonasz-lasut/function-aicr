package checks

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"

	"aicr.run/pkg/defaults"
	"aicr.run/pkg/recipe"

	"github.com/jonasz-lasut/function-aicr/internal/compose"
)

func TestRun(t *testing.T) {
	const customizations = "nodewright-customizations"

	dp := recipe.NewEmbeddedDataProvider(recipe.GetEmbeddedFS(), ".")
	registry, err := recipe.GetComponentRegistryFor(dp)
	if err != nil {
		t.Fatalf("recipe.GetComponentRegistryFor(...): %v", err)
	}

	// Two recipes AICR's bundler refuses without the right placement.
	aks := &recipe.Criteria{
		Accelerator: recipe.CriteriaAcceleratorH100,
		Service:     recipe.CriteriaServiceAKS,
		OS:          recipe.CriteriaOSUbuntu,
		Intent:      recipe.CriteriaIntentTraining,
		Platform:    recipe.CriteriaPlatformKubeflow,
	}
	oke := &recipe.Criteria{
		Accelerator: recipe.CriteriaAcceleratorGB300,
		Service:     recipe.CriteriaServiceOKE,
		OS:          recipe.CriteriaOSUbuntu,
		Intent:      recipe.CriteriaIntentTraining,
	}
	keyed := []corev1.Toleration{{Key: "nvidia.com/gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}

	type args struct {
		criteria *recipe.Criteria
		request  Request
	}
	type want struct {
		findings []Finding
		mentions string
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"WildcardTolerationOnAKS": {
			reason: "Without accelerated tolerations the tuning manifest renders one without a key, which deadlocks NodeWright's packages on AKS; AICR refuses, and the message names the Input field.",
			args:   args{criteria: aks},
			want: want{
				findings: []Finding{{Component: customizations, Blocking: true}},
				mentions: "nodeScheduling.accelerated.tolerations",
			},
		},
		"ExplicitWildcardOnAKS": {
			reason: "A toleration without a key that the Input states itself is the same hazard.",
			args: args{criteria: aks, request: Request{Scheduling: compose.NodeScheduling{
				Accelerated: compose.NodePlacement{Tolerations: []corev1.Toleration{{Operator: corev1.TolerationOpExists}}},
			}}},
			want: want{
				findings: []Finding{{Component: customizations, Blocking: true}},
				mentions: "nodeScheduling.accelerated.tolerations",
			},
		},
		"KeyedTolerationOnAKS": {
			reason: "Tolerations that name a key pass the check.",
			args: args{criteria: aks, request: Request{Scheduling: compose.NodeScheduling{
				Accelerated: compose.NodePlacement{Tolerations: keyed},
			}}},
		},
		"SkippedComponent": {
			reason: "A component the Input skips renders nothing, as with AICR's enabled=false, so its checks pass.",
			args:   args{criteria: aks, request: Request{Skipped: []string{customizations}}},
		},
		"UnscopedTuningOnOKE": {
			reason: "The OKE GB300 tuning resource reboots every node it matches, so AICR refuses it without an accelerated node selector.",
			args:   args{criteria: oke},
			want: want{
				findings: []Finding{{Component: customizations, Blocking: true}},
				mentions: "nodeScheduling.accelerated.nodeSelector",
			},
		},
		"ScopedTuningOnOKE": {
			reason: "With the selector the tuning resource is scoped to the accelerated pool.",
			args: args{criteria: oke, request: Request{Scheduling: compose.NodeScheduling{
				Accelerated: compose.NodePlacement{NodeSelector: map[string]string{"pool": "gb300"}},
			}}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			res, err := recipe.NewBuilder(recipe.WithDataProvider(dp)).BuildFromCriteria(ctx, tc.args.criteria)
			if err != nil {
				t.Fatalf("BuildFromCriteria(%s): %v", tc.args.criteria, err)
			}
			var components []string
			for _, c := range res.ComponentRefs {
				if c.IsEnabled() {
					components = append(components, c.Name)
				}
			}

			got := Run(ctx, res, Config(tc.args.request), registry, components)

			if diff := cmp.Diff(tc.want.findings, got, cmpopts.IgnoreFields(Finding{}, "Message"), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("%s\nRun(...): -want, +got:\n%s", tc.reason, diff)
			}
			for _, f := range got {
				if !strings.Contains(f.Message, tc.want.mentions) {
					t.Errorf("%s\nRun(...): the message does not mention %q: %s", tc.reason, tc.want.mentions, f.Message)
				}
				if strings.Contains(f.Message, "--accelerated-node") || strings.Contains(f.Message, "--system-node") {
					t.Errorf("%s\nRun(...): the message names a flag of AICR's command line, which the reader cannot pass: %s", tc.reason, f.Message)
				}
			}
		})
	}
}

func TestConfig(t *testing.T) {
	toleration := corev1.Toleration{Key: "nvidia.com/gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}

	type bundle struct {
		SystemSelector         map[string]string
		SystemTolerations      []corev1.Toleration
		AcceleratedSelector    map[string]string
		AcceleratedTolerations []corev1.Toleration
		Disabled               map[string]map[string]string
		DRAEvictionKey         string
	}
	type args struct {
		request Request
	}
	type want struct {
		bundle bundle
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Empty": {
			reason: "Nothing is defaulted: where AICR's command line would tolerate every taint, the checks see no toleration, which is what the function writes.",
			want:   want{bundle: bundle{}},
		},
		"Everything": {
			reason: "The placement is passed on as stated, a skipped component is disabled, and the DRA eviction label is AICR's default.",
			args: args{request: Request{
				Scheduling: compose.NodeScheduling{
					System:      compose.NodePlacement{NodeSelector: map[string]string{"pool": "system"}},
					Accelerated: compose.NodePlacement{NodeSelector: map[string]string{"pool": "gpu"}, Tolerations: []corev1.Toleration{toleration}},
				},
				Skipped:     []string{"nktp"},
				DRAEviction: true,
			}},
			want: want{bundle: bundle{
				SystemSelector:         map[string]string{"pool": "system"},
				AcceleratedSelector:    map[string]string{"pool": "gpu"},
				AcceleratedTolerations: []corev1.Toleration{toleration},
				Disabled:               map[string]map[string]string{"nktp": {"enabled": "false"}},
				DRAEvictionKey:         defaults.DRAEvictionNodeLabelKey,
			}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := Config(tc.args.request)

			got := bundle{
				SystemSelector:         cfg.SystemNodeSelector(),
				SystemTolerations:      cfg.SystemNodeTolerations(),
				AcceleratedSelector:    cfg.AcceleratedNodeSelector(),
				AcceleratedTolerations: cfg.AcceleratedNodeTolerations(),
				Disabled:               cfg.ValueOverrides(),
				DRAEvictionKey:         cfg.DRAEvictionNodeLabel().Key,
			}
			if diff := cmp.Diff(tc.want.bundle, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("%s\nConfig(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
