package resolve

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
)

func TestProfile(t *testing.T) {
	type args struct {
		in *v1beta1.Input
	}
	type want struct {
		profile string
		err     error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Unset": {
			reason: "Without a profile AICR is given no selection and deploys the recipe's default variant.",
			args:   args{in: &v1beta1.Input{}},
			want:   want{profile: ""},
		},
		"OneSelection": {
			reason: "A selection is passed in the name=value form AICR accepts.",
			args:   args{in: &v1beta1.Input{Profile: map[string]string{"gpuStack": "dra"}}},
			want:   want{profile: "gpuStack=dra"},
		},
		"EmptyValueIsUnset": {
			reason: "A profile name with an empty value selects nothing.",
			args:   args{in: &v1beta1.Input{Profile: map[string]string{"gpuStack": ""}}},
			want:   want{profile: ""},
		},
		"TwoSelectionsError": {
			reason: "AICR accepts one selection per recipe; picking one of two silently would deploy a variant nobody chose.",
			args:   args{in: &v1beta1.Input{Profile: map[string]string{"gpuStack": "dra", "other": "x"}}},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Profile(tc.args.in)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nProfile(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.profile, got); diff != "" {
				t.Errorf("%s\nProfile(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestBuildOptions(t *testing.T) {
	mapping := []v1beta1.NetworkInterfaceMapping{{InterfaceName: "eth1", Network: "gpu-nic-0"}}

	type args struct {
		in *v1beta1.Input
	}
	type want struct {
		options int
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NoConfiguration": {
			reason: "Without configuration nothing is passed, so a recipe that takes no such choice is unaffected.",
			args:   args{in: &v1beta1.Input{}},
			want:   want{options: 0},
		},
		"EmptyConfiguration": {
			reason: "Empty choices are not passed either: AICR rejects one on a recipe it does not apply to.",
			args: args{in: &v1beta1.Input{Configuration: &v1beta1.Configuration{
				GKE:              &v1beta1.GKEConfiguration{},
				RuntimeInventory: &v1beta1.RuntimeInventoryConfiguration{},
			}}},
			want: want{options: 0},
		},
		"EveryChoice": {
			reason: "Each stated choice becomes one build option.",
			args: args{in: &v1beta1.Input{Configuration: &v1beta1.Configuration{
				GKE:              &v1beta1.GKEConfiguration{TCPXOInterfaces: mapping},
				RuntimeInventory: &v1beta1.RuntimeInventoryConfiguration{Mode: "enabled"},
			}}},
			want: want{options: 2},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := BuildOptions(tc.args.in)

			if diff := cmp.Diff(tc.want.options, len(got)); diff != "" {
				t.Errorf("%s\nlen(BuildOptions(...)): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
