package resolve

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
)

func TestInput(t *testing.T) {
	typeMeta := metav1.TypeMeta{APIVersion: "aicr.fn.crossplane.io/v1beta1", Kind: "Input"}
	overlayFrom := &v1beta1.OverlayFrom{FieldPath: "spec.aicr"}

	static := func(fields map[string]any) map[string]any {
		in := map[string]any{"apiVersion": typeMeta.APIVersion, "kind": typeMeta.Kind}
		for k, v := range fields {
			in[k] = v
		}
		return in
	}
	xr := func(overlay any) map[string]any {
		return map[string]any{
			"apiVersion": "example.org/v1",
			"kind":       "AnyKind",
			"spec":       map[string]any{"aicr": overlay, "nodes": int64(4)},
		}
	}

	type args struct {
		static map[string]any
		xr     map[string]any
	}
	type want struct {
		in  *v1beta1.Input
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"StaticOnly": {
			reason: "Without overlayFrom the static Input is used as it is, whatever the composite resource holds.",
			args: args{
				static: static(map[string]any{"criteria": map[string]any{"accelerator": "h100"}}),
				xr:     xr(map[string]any{"criteria": map[string]any{"accelerator": "gb200"}}),
			},
			want: want{in: &v1beta1.Input{TypeMeta: typeMeta, Criteria: &v1beta1.Criteria{Accelerator: "h100"}}},
		},
		"OverlayWinsKeyByKey": {
			reason: "Maps merge key by key: the overlay's keys win and the static keys it does not set survive.",
			args: args{
				static: static(map[string]any{
					"overlayFrom":       map[string]any{"fieldPath": "spec.aicr"},
					"criteria":          map[string]any{"accelerator": "h100", "intent": "training"},
					"providerConfigRef": map[string]any{"kind": "ProviderConfig", "name": "default"},
				}),
				xr: xr(map[string]any{
					"criteria":          map[string]any{"accelerator": "gb200", "service": "eks"},
					"providerConfigRef": map[string]any{"name": "workload-a"},
				}),
			},
			want: want{in: &v1beta1.Input{
				TypeMeta:          typeMeta,
				OverlayFrom:       overlayFrom,
				Criteria:          &v1beta1.Criteria{Accelerator: "gb200", Service: "eks", Intent: "training"},
				ProviderConfigRef: &v1beta1.ProviderConfigRef{Kind: "ProviderConfig", Name: "workload-a"},
			}},
		},
		"OverlaySetsWhatStaticOmits": {
			reason: "The overlay may set a field the static Input does not mention at all.",
			args: args{
				static: static(map[string]any{"overlayFrom": map[string]any{"fieldPath": "spec.aicr"}}),
				xr:     xr(map[string]any{"criteria": map[string]any{"service": "kind"}, "skipDeploymentOrder": true}),
			},
			want: want{in: &v1beta1.Input{
				TypeMeta:            typeMeta,
				OverlayFrom:         overlayFrom,
				Criteria:            &v1beta1.Criteria{Service: "kind"},
				SkipDeploymentOrder: true,
			}},
		},
		"ListsReplace": {
			reason: "A list in the overlay replaces the static list whole; merging lists by position would be unpredictable.",
			args: args{
				static: static(map[string]any{
					"overlayFrom":    map[string]any{"fieldPath": "spec.aicr"},
					"skipComponents": []any{map[string]any{"name": "nfd"}, map[string]any{"name": "nvsentinel"}},
				}),
				xr: xr(map[string]any{"skipComponents": []any{map[string]any{"name": "kai-scheduler"}}}),
			},
			want: want{in: &v1beta1.Input{
				TypeMeta:       typeMeta,
				OverlayFrom:    overlayFrom,
				SkipComponents: []v1beta1.ComponentRef{{Name: "kai-scheduler"}},
			}},
		},
		"EmptyStringIsUnset": {
			reason: "Kubernetes cannot tell an empty optional string from an omitted one, so an empty string in the overlay keeps the static value.",
			args: args{
				static: static(map[string]any{
					"overlayFrom": map[string]any{"fieldPath": "spec.aicr"},
					"criteria":    map[string]any{"accelerator": "h100", "os": "ubuntu"},
				}),
				xr: xr(map[string]any{"criteria": map[string]any{"accelerator": "", "os": nil, "service": "eks"}}),
			},
			want: want{in: &v1beta1.Input{
				TypeMeta:    typeMeta,
				OverlayFrom: overlayFrom,
				Criteria:    &v1beta1.Criteria{Accelerator: "h100", Service: "eks", OS: "ubuntu"},
			}},
		},
		"ChartValuesKeepEmptyStrings": {
			reason: "Under componentOverrides.<component>.values an empty string is a chart value like any other; the values still merge key by key over the static ones.",
			args: args{
				static: static(map[string]any{
					"overlayFrom": map[string]any{"fieldPath": "spec.aicr"},
					"componentOverrides": map[string]any{"gpu-operator": map[string]any{
						"version": "v1",
						"values":  map[string]any{"driver": map[string]any{"enabled": true, "version": "580"}},
					}},
				}),
				xr: xr(map[string]any{"componentOverrides": map[string]any{"gpu-operator": map[string]any{
					"version": "",
					"values":  map[string]any{"driver": map[string]any{"version": ""}},
				}}}),
			},
			want: want{in: &v1beta1.Input{
				TypeMeta:    typeMeta,
				OverlayFrom: overlayFrom,
				ComponentOverrides: map[string]v1beta1.ComponentOverride{"gpu-operator": {
					Version: "v1",
					Values:  &runtime.RawExtension{Raw: []byte(`{"driver":{"enabled":true,"version":""}}`)},
				}},
			}},
		},
		"NodeSelectorsKeepEmptyStrings": {
			reason: "A node label's value may be the empty string, as with the node-role labels, so in a node selector it is a value; the selector still merges key by key over the static one.",
			args: args{
				static: static(map[string]any{
					"overlayFrom": map[string]any{"fieldPath": "spec.aicr"},
					"nodeScheduling": map[string]any{"system": map[string]any{
						"nodeSelector": map[string]any{"nodeGroup": "system-pool"},
					}},
				}),
				xr: xr(map[string]any{"nodeScheduling": map[string]any{"system": map[string]any{
					"nodeSelector": map[string]any{"node-role.kubernetes.io/system": ""},
				}}}),
			},
			want: want{in: &v1beta1.Input{
				TypeMeta:    typeMeta,
				OverlayFrom: overlayFrom,
				NodeScheduling: &v1beta1.NodeScheduling{System: &v1beta1.NodePlacement{
					NodeSelector: map[string]string{"nodeGroup": "system-pool", "node-role.kubernetes.io/system": ""},
				}},
			}},
		},
		"AbsentPathKeepsStatic": {
			reason: "A composite resource that does not carry the overlay object gets the static Input alone; that is not an error.",
			args: args{
				static: static(map[string]any{
					"overlayFrom": map[string]any{"fieldPath": "spec.missing"},
					"criteria":    map[string]any{"service": "kind"},
				}),
				xr: xr(map[string]any{"criteria": map[string]any{"service": "eks"}}),
			},
			want: want{in: &v1beta1.Input{
				TypeMeta:    typeMeta,
				OverlayFrom: &v1beta1.OverlayFrom{FieldPath: "spec.missing"},
				Criteria:    &v1beta1.Criteria{Service: "kind"},
			}},
		},
		"NonObjectOverlayErrors": {
			reason: "A field path that holds anything but an object is a mistake in the Composition, never a silent fallback to the static Input.",
			args: args{
				static: static(map[string]any{"overlayFrom": map[string]any{"fieldPath": "spec.nodes"}}),
				xr:     xr(map[string]any{}),
			},
			want: want{err: cmpopts.AnyError},
		},
		"MalformedPathErrors": {
			reason: "A field path that does not parse is an error.",
			args: args{
				static: static(map[string]any{"overlayFrom": map[string]any{"fieldPath": "spec.aicr["}}),
				xr:     xr(map[string]any{}),
			},
			want: want{err: cmpopts.AnyError},
		},
		"EmptyPathErrors": {
			reason: "overlayFrom without a field path would otherwise read the whole composite resource as the overlay.",
			args: args{
				static: static(map[string]any{"overlayFrom": map[string]any{"fieldPath": ""}}),
				xr:     xr(map[string]any{}),
			},
			want: want{err: cmpopts.AnyError},
		},
		"UnknownOverlayFieldErrors": {
			reason: "A field the Input does not have is rejected, so a typo on the composite resource is not silently dropped.",
			args: args{
				static: static(map[string]any{"overlayFrom": map[string]any{"fieldPath": "spec.aicr"}}),
				xr:     xr(map[string]any{"selector": map[string]any{"service": "eks"}}),
			},
			want: want{err: cmpopts.AnyError},
		},
		"OverlayMustNotMoveItself": {
			reason: "The overlay may set any field but overlayFrom: where the overlay is read from is the Composition author's decision.",
			args: args{
				static: static(map[string]any{"overlayFrom": map[string]any{"fieldPath": "spec.aicr"}}),
				xr:     xr(map[string]any{"overlayFrom": map[string]any{"fieldPath": "spec.other"}}),
			},
			want: want{err: cmpopts.AnyError},
		},
		"UnknownStaticFieldErrors": {
			reason: "The static Input is decoded strictly too.",
			args: args{
				static: static(map[string]any{"selector": map[string]any{"service": "eks"}}),
				xr:     xr(map[string]any{}),
			},
			want: want{err: cmpopts.AnyError},
		},
		"RemovedFieldErrors": {
			reason: "A Composition written for the per-field criteriaFrom is told what replaced it.",
			args: args{
				static: static(map[string]any{"criteriaFrom": map[string]any{"service": "spec.cloud"}}),
				xr:     xr(map[string]any{}),
			},
			want: want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Input(tc.args.static, fieldpath.Pave(tc.args.xr))

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nInput(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.in, got); diff != "" {
				t.Errorf("%s\nInput(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
