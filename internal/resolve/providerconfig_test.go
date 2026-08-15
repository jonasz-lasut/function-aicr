package resolve

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
	"github.com/jonasz-lasut/function-aicr/internal/compose"
)

func TestProviderConfigRef(t *testing.T) {
	xr := map[string]any{
		"apiVersion": "example.org/v1",
		"kind":       "AnyKind",
		"spec": map[string]any{
			"cluster":     "workload-a",
			"kind":        "ProviderConfig",
			"badKind":     "ClusterConfig",
			"emptyName":   "",
			"notAString":  int64(4),
			"unsetTarget": nil,
		},
	}

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
		"LiteralOnly": {
			reason: "Without providerConfigRefFrom the literal is used as-is.",
			args:   args{in: &v1beta1.Input{ProviderConfigRef: &v1beta1.ProviderConfigRef{Kind: "ProviderConfig", Name: "gpu-cluster"}}},
			want:   want{ref: compose.ProviderConfigRef{Kind: "ProviderConfig", Name: "gpu-cluster"}},
		},
		"KindDefaultsToClusterProviderConfig": {
			reason: "An unset kind is the cluster-scoped provider config, as before.",
			args:   args{in: &v1beta1.Input{ProviderConfigRef: &v1beta1.ProviderConfigRef{Name: "gpu-cluster"}}},
			want:   want{ref: compose.ProviderConfigRef{Kind: "ClusterProviderConfig", Name: "gpu-cluster"}},
		},
		"FieldPathsWinOverLiterals": {
			reason: "A providerConfigRefFrom path that resolves to a non-empty string wins over the literal, so one Composition can target the cluster each XR names.",
			args: args{in: &v1beta1.Input{
				ProviderConfigRef:     &v1beta1.ProviderConfigRef{Name: "gpu-cluster"},
				ProviderConfigRefFrom: &v1beta1.ProviderConfigRefFrom{Kind: new("spec.kind"), Name: new("spec.cluster")},
			}},
			want: want{ref: compose.ProviderConfigRef{Kind: "ProviderConfig", Name: "workload-a"}},
		},
		"AbsentOrEmptyPathFallsBackToLiteral": {
			reason: "A path that is absent or empty on the composite falls back to the literal, per Kubernetes' empty-equals-omitted convention.",
			args: args{in: &v1beta1.Input{
				ProviderConfigRef:     &v1beta1.ProviderConfigRef{Name: "gpu-cluster"},
				ProviderConfigRefFrom: &v1beta1.ProviderConfigRefFrom{Kind: new("spec.missing"), Name: new("spec.emptyName")},
			}},
			want: want{ref: compose.ProviderConfigRef{Kind: "ClusterProviderConfig", Name: "gpu-cluster"}},
		},
		"PathOnlyWithoutLiteral": {
			reason: "providerConfigRef may be omitted entirely when a providerConfigRefFrom.name path resolves.",
			args:   args{in: &v1beta1.Input{ProviderConfigRefFrom: &v1beta1.ProviderConfigRefFrom{Name: new("spec.cluster")}}},
			want:   want{ref: compose.ProviderConfigRef{Kind: "ClusterProviderConfig", Name: "workload-a"}},
		},
		"MissingNameErrors": {
			reason: "A name must come from somewhere; composed resources without a provider config would never reconcile.",
			args:   args{in: &v1beta1.Input{}},
			want:   want{err: cmpopts.AnyError},
		},
		"UnknownKindErrors": {
			reason: "A kind pulled off the composite bypasses the Input schema's enum, so it is checked here.",
			args: args{in: &v1beta1.Input{
				ProviderConfigRef:     &v1beta1.ProviderConfigRef{Name: "gpu-cluster"},
				ProviderConfigRefFrom: &v1beta1.ProviderConfigRefFrom{Kind: new("spec.badKind")},
			}},
			want: want{err: cmpopts.AnyError},
		},
		"WrongTypeAtPathErrors": {
			reason: "A non-string value at a path is a configuration mistake, never a silent fallback.",
			args: args{in: &v1beta1.Input{
				ProviderConfigRef:     &v1beta1.ProviderConfigRef{Name: "gpu-cluster"},
				ProviderConfigRefFrom: &v1beta1.ProviderConfigRefFrom{Name: new("spec.notAString")},
			}},
			want: want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ProviderConfigRef(tc.args.in, fieldpath.Pave(xr))

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nProviderConfigRef(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.ref, got); diff != "" {
				t.Errorf("%s\nProviderConfigRef(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
