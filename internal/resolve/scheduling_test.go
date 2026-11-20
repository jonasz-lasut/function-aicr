package resolve

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
	"github.com/jonasz-lasut/function-aicr/internal/compose"
)

func TestNodeScheduling(t *testing.T) {
	toleration := corev1.Toleration{Key: "nvidia.com/gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}

	type args struct {
		in *v1beta1.Input
	}
	type want struct {
		s   compose.NodeScheduling
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Unset": {
			reason: "An Input without node scheduling places nothing.",
			args:   args{in: &v1beta1.Input{}},
		},
		"BothKindsOfWorkload": {
			reason: "Each placement is passed on as it is stated.",
			args: args{in: &v1beta1.Input{NodeScheduling: &v1beta1.NodeScheduling{
				System:      &v1beta1.NodePlacement{NodeSelector: map[string]string{"nodeGroup": "system-pool"}},
				Accelerated: &v1beta1.NodePlacement{NodeSelector: map[string]string{"node-role.kubernetes.io/gpu": ""}, Tolerations: []corev1.Toleration{toleration}},
			}}},
			want: want{s: compose.NodeScheduling{
				System:      compose.NodePlacement{NodeSelector: map[string]string{"nodeGroup": "system-pool"}},
				Accelerated: compose.NodePlacement{NodeSelector: map[string]string{"node-role.kubernetes.io/gpu": ""}, Tolerations: []corev1.Toleration{toleration}},
			}},
		},
		"InvalidLabel": {
			reason: "A node selector that is not made of valid labels would be refused by the API server on every pod.",
			args: args{in: &v1beta1.Input{NodeScheduling: &v1beta1.NodeScheduling{
				System: &v1beta1.NodePlacement{NodeSelector: map[string]string{"node group": "system"}},
			}}},
			want: want{err: cmpopts.AnyError},
		},
		"InvalidToleration": {
			reason: "A toleration with a value needs the Equal operator.",
			args: args{in: &v1beta1.Input{NodeScheduling: &v1beta1.NodeScheduling{
				Accelerated: &v1beta1.NodePlacement{Tolerations: []corev1.Toleration{{Key: "nvidia.com/gpu", Operator: corev1.TolerationOpExists, Value: "present"}}},
			}}},
			want: want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := NodeScheduling(tc.args.in)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nNodeScheduling(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.s, got); diff != "" {
				t.Errorf("%s\nNodeScheduling(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
