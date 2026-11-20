package resolve

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"aicr.run/pkg/recipe"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
)

func TestCriteria(t *testing.T) {
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
		"AllDimensions": {
			reason: "The criteria the Input states are passed to AICR as they are.",
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
		"UnsetDimensionsStayUnstated": {
			reason: "A dimension the Input leaves unset is passed to AICR unstated, never defaulted: AICR's catalog decides which combinations exist.",
			args:   args{in: &v1beta1.Input{Criteria: &v1beta1.Criteria{Accelerator: "h100", Service: "eks", Intent: "training"}}},
			want: want{c: &recipe.Criteria{
				Accelerator: recipe.CriteriaAcceleratorH100,
				Service:     recipe.CriteriaServiceEKS,
				Intent:      recipe.CriteriaIntentTraining,
			}},
		},
		"SingleDimensionSuffices": {
			reason: "AICR's catalog has recipes without an accelerator (ocp, bcm) and accelerator-only tiers, so no one dimension is required.",
			args:   args{in: &v1beta1.Input{Criteria: &v1beta1.Criteria{Service: "ocp", Intent: "training"}}},
			want: want{c: &recipe.Criteria{
				Service: recipe.CriteriaServiceOCP,
				Intent:  recipe.CriteriaIntentTraining,
			}},
		},
		"NoCriteriaErrors": {
			reason: "An Input without criteria, as when its overlay misses the composite resource, must not silently resolve the base recipe.",
			args:   args{in: &v1beta1.Input{}},
			want:   want{err: cmpopts.AnyError},
		},
		"EmptyCriteriaErrors": {
			reason: "A criteria object with every dimension empty states nothing either.",
			args:   args{in: &v1beta1.Input{Criteria: &v1beta1.Criteria{}}},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Criteria(tc.args.in)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nCriteria(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.c, got); diff != "" {
				t.Errorf("%s\nCriteria(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
