package resolve

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/NVIDIA/aicr/pkg/recipe"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
)

func TestCriteria(t *testing.T) {
	xr := map[string]any{
		"apiVersion": "example.org/v1",
		"kind":       "AnyKind",
		"spec": map[string]any{
			"gpuType": "h100",
			"cloud":   "eks",
			"purpose": "training",
			"nodes":   int64(4),
			"os":      "",
		},
	}

	type args struct {
		in *v1beta1.Input
	}
	type want struct {
		c   *recipe.Criteria
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"LiteralsOnly": {
			reason: "With no criteriaFrom, the literal criteria are used verbatim.",
			args: args{in: &v1beta1.Input{Criteria: &v1beta1.Criteria{
				Accelerator: "h200",
				Service:     "gke",
				Intent:      "inference",
				OS:          "cos",
				Platform:    "dynamo",
			}}},
			want: want{c: &recipe.Criteria{
				Accelerator: recipe.CriteriaAcceleratorH200,
				Service:     recipe.CriteriaServiceGKE,
				Intent:      recipe.CriteriaIntentInference,
				OS:          recipe.CriteriaOSCOS,
				Platform:    recipe.CriteriaPlatformDynamo,
			}},
		},
		"FieldPathsWinOverLiterals": {
			reason: "A resolving criteriaFrom path overrides its literal.",
			args: args{in: &v1beta1.Input{
				Criteria: &v1beta1.Criteria{Accelerator: "a100", Service: "aks", Intent: "inference"},
				CriteriaFrom: &v1beta1.CriteriaFrom{
					Accelerator: new("spec.gpuType"),
					Service:     new("spec.cloud"),
					Intent:      new("spec.purpose"),
				},
			}},
			want: want{c: &recipe.Criteria{
				Accelerator: recipe.CriteriaAcceleratorH100,
				Service:     recipe.CriteriaServiceEKS,
				Intent:      recipe.CriteriaIntentTraining,
			}},
		},
		"AbsentPathFallsBackToLiteral": {
			reason: "An absent path is normal for an optional XR field; the literal is used.",
			args: args{in: &v1beta1.Input{
				Criteria: &v1beta1.Criteria{Accelerator: "h100", Service: "eks", Intent: "training", Platform: "kubeflow"},
				CriteriaFrom: &v1beta1.CriteriaFrom{
					Platform: new("spec.mlPlatform"),
				},
			}},
			want: want{c: &recipe.Criteria{
				Accelerator: recipe.CriteriaAcceleratorH100,
				Service:     recipe.CriteriaServiceEKS,
				Intent:      recipe.CriteriaIntentTraining,
				Platform:    recipe.CriteriaPlatformKubeflow,
			}},
		},
		"EmptyValueAtPathFallsBackToLiteral": {
			reason: "An empty string at a path is indistinguishable from an omitted optional field; the literal is used.",
			args: args{in: &v1beta1.Input{
				Criteria:     &v1beta1.Criteria{Accelerator: "h100", Service: "eks", Intent: "training", OS: "cos"},
				CriteriaFrom: &v1beta1.CriteriaFrom{OS: new("spec.os")},
			}},
			want: want{c: &recipe.Criteria{
				Accelerator: recipe.CriteriaAcceleratorH100,
				Service:     recipe.CriteriaServiceEKS,
				Intent:      recipe.CriteriaIntentTraining,
				OS:          recipe.CriteriaOSCOS,
			}},
		},
		"OSStaysUnstatedWhenUnresolved": {
			reason: "An os that neither the literal nor a path supplies is left unstated so AICR selects the OS-agnostic recipe tier; the function must not guess one.",
			args: args{in: &v1beta1.Input{
				Criteria:     &v1beta1.Criteria{Accelerator: "h100", Service: "kind", Intent: "training"},
				CriteriaFrom: &v1beta1.CriteriaFrom{OS: new("spec.os")},
			}},
			want: want{c: &recipe.Criteria{
				Accelerator: recipe.CriteriaAcceleratorH100,
				Service:     recipe.CriteriaServiceKind,
				Intent:      recipe.CriteriaIntentTraining,
			}},
		},
		"MalformedPathErrors": {
			reason: "A typo must not silently degrade to the literal.",
			args: args{in: &v1beta1.Input{
				Criteria:     &v1beta1.Criteria{Accelerator: "h100", Service: "eks", Intent: "training"},
				CriteriaFrom: &v1beta1.CriteriaFrom{Accelerator: new("spec[gpuType")},
			}},
			want: want{err: cmpopts.AnyError},
		},
		"WrongTypeAtPathErrors": {
			reason: "A non-string value at a criteria path is a configuration mistake.",
			args: args{in: &v1beta1.Input{
				Criteria:     &v1beta1.Criteria{Accelerator: "h100", Service: "eks", Intent: "training"},
				CriteriaFrom: &v1beta1.CriteriaFrom{Accelerator: new("spec.nodes")},
			}},
			want: want{err: cmpopts.AnyError},
		},
		"SingleDimensionSuffices": {
			reason: "AICR's catalog has recipes without an accelerator (ocp, bcm) and accelerator-only tiers, so no one dimension is required.",
			args:   args{in: &v1beta1.Input{Criteria: &v1beta1.Criteria{Service: "ocp", Intent: "training"}}},
			want: want{c: &recipe.Criteria{
				Service: recipe.CriteriaServiceOCP,
				Intent:  recipe.CriteriaIntentTraining,
			}},
		},
		"NothingResolvedErrors": {
			reason: "criteriaFrom paths that all miss the composite must not silently resolve the base recipe.",
			args: args{in: &v1beta1.Input{
				CriteriaFrom: &v1beta1.CriteriaFrom{Accelerator: new("spec.missing"), Service: new("spec.os")},
			}},
			want: want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Criteria(tc.args.in, fieldpath.Pave(xr))

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nCriteria(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.c, got); diff != "" {
				t.Errorf("%s\nCriteria(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
