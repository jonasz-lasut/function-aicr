package resolve

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
	"github.com/jonasz-lasut/function-aicr/internal/compose"
)

func TestProviderConfigRef(t *testing.T) {
	type args struct {
		in *v1beta1.Input
	}
	type want struct {
		ref compose.ProviderConfigRef
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"KindAndName": {
			reason: "The kind and name the Input states are used as they are.",
			args:   args{in: &v1beta1.Input{ProviderConfigRef: &v1beta1.ProviderConfigRef{Kind: "ProviderConfig", Name: "workload-a"}}},
			want:   want{ref: compose.ProviderConfigRef{Kind: "ProviderConfig", Name: "workload-a"}},
		},
		"KindDefaults": {
			reason: "An unset kind defaults to ClusterProviderConfig: the overlay bypasses the Input schema's default.",
			args:   args{in: &v1beta1.Input{ProviderConfigRef: &v1beta1.ProviderConfigRef{Name: "gpu-cluster"}}},
			want:   want{ref: compose.ProviderConfigRef{Kind: "ClusterProviderConfig", Name: "gpu-cluster"}},
		},
		"UnknownKindErrors": {
			reason: "A kind the providers do not accept is rejected: a value the overlay read off the composite resource bypasses the Input schema's enum.",
			args:   args{in: &v1beta1.Input{ProviderConfigRef: &v1beta1.ProviderConfigRef{Kind: "Secret", Name: "gpu-cluster"}}},
			want:   want{err: cmpopts.AnyError},
		},
		"MissingNameErrors": {
			reason: "A provider config name is required; without one no composed resource could reach a cluster.",
			args:   args{in: &v1beta1.Input{ProviderConfigRef: &v1beta1.ProviderConfigRef{Kind: "ProviderConfig"}}},
			want:   want{err: cmpopts.AnyError},
		},
		"MissingRefErrors": {
			reason: "An Input without a provider config reference at all has no name either.",
			args:   args{in: &v1beta1.Input{}},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ProviderConfigRef(tc.args.in)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nProviderConfigRef(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.ref, got); diff != "" {
				t.Errorf("%s\nProviderConfigRef(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
