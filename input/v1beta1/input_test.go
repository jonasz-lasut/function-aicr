package v1beta1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	sigsyaml "sigs.k8s.io/yaml"
)

func TestInputUnmarshal(t *testing.T) {
	type args struct {
		yaml string
	}
	type want struct {
		in Input
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"FullySpecified": {
			reason: "Every documented field decodes into its Go counterpart.",
			args: args{yaml: `
apiVersion: aicr.fn.crossplane.io/v1beta1
kind: Input
criteria:
  accelerator: h100
  service: eks
  intent: training
  os: ubuntu
  platform: kubeflow
criteriaFrom:
  accelerator: spec.accelerator
  service: spec.service
  intent: spec.intent
  os: spec.os
  platform: spec.platform
providerConfigRef:
  kind: ClusterProviderConfig
  name: gpu-cluster
providerConfigRefFrom:
  kind: spec.providerConfigKind
  name: spec.providerConfigName
skipDeploymentOrder: true
skipComponents:
- name: cert-manager
componentOverrides:
  gpu-operator:
    version: v25.11.0
    namespace: gpu-operator
    values:
      driver:
        version: "535.129.03"
target:
  fieldPath: status.aicr
`},
			want: want{in: Input{
				TypeMeta: metaTypeMeta(),
				Criteria: &Criteria{
					Accelerator: "h100",
					Service:     "eks",
					Intent:      "training",
					OS:          "ubuntu",
					Platform:    "kubeflow",
				},
				CriteriaFrom: &CriteriaFrom{
					Accelerator: new("spec.accelerator"),
					Service:     new("spec.service"),
					Intent:      new("spec.intent"),
					OS:          new("spec.os"),
					Platform:    new("spec.platform"),
				},
				ProviderConfigRef: &ProviderConfigRef{
					Kind: "ClusterProviderConfig",
					Name: "gpu-cluster",
				},
				ProviderConfigRefFrom: &ProviderConfigRefFrom{
					Kind: new("spec.providerConfigKind"),
					Name: new("spec.providerConfigName"),
				},
				SkipDeploymentOrder: true,
				SkipComponents:      []ComponentRef{{Name: "cert-manager"}},
				ComponentOverrides: map[string]ComponentOverride{
					"gpu-operator": {
						Version:   "v25.11.0",
						Namespace: "gpu-operator",
						Values:    &runtime.RawExtension{Raw: []byte(`{"driver":{"version":"535.129.03"}}`)},
					},
				},
				Target: &Target{FieldPath: "status.aicr"},
			}},
		},
		"Minimal": {
			reason: "Nothing is structurally required; providerConfigRef.name (or a providerConfigRefFrom.name path) is enforced at run time.",
			args: args{yaml: `
apiVersion: aicr.fn.crossplane.io/v1beta1
kind: Input
providerConfigRef:
  name: gpu-cluster
`},
			want: want{in: Input{
				TypeMeta:          metaTypeMeta(),
				ProviderConfigRef: &ProviderConfigRef{Name: "gpu-cluster"},
			}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Input{}
			if err := sigsyaml.Unmarshal([]byte(tc.args.yaml), &got); err != nil {
				t.Fatalf("sigsyaml.Unmarshal(...): %v", err)
			}
			if diff := cmp.Diff(tc.want.in, got); diff != "" {
				t.Errorf("%s\nUnmarshal(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func metaTypeMeta() metav1.TypeMeta {
	return metav1.TypeMeta{APIVersion: "aicr.fn.crossplane.io/v1beta1", Kind: "Input"}
}
