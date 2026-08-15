package schema

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/types/known/structpb"
)

// releaseSchema is a cut-down stand-in for provider-helm's Release CRD schema:
// spec.forProvider.chart.name is required, and wait must be a boolean.
func releaseSchema(t *testing.T) *structpb.Struct {
	t.Helper()
	return mustStruct(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"spec": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"forProvider": map[string]any{
						"type":     "object",
						"required": []any{"chart"},
						"properties": map[string]any{
							"chart": map[string]any{
								"type":     "object",
								"required": []any{"name"},
								"properties": map[string]any{
									"name": map[string]any{"type": "string"},
								},
							},
							"wait": map[string]any{"type": "boolean"},
						},
					},
				},
			},
		},
	})
}

func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatalf("structpb.NewStruct(...): %v", err)
	}
	return s
}

func TestNewValidator(t *testing.T) {
	type args struct {
		schema *structpb.Struct
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ObjectSchema": {
			reason: "A schema declaring a type and properties compiles.",
			args:   args{schema: releaseSchema(t)},
		},
		"NilSchema": {
			reason: "There is nothing to compile; the caller must not proceed as if everything validated.",
			args:   args{schema: nil},
			want:   want{err: cmpopts.AnyError},
		},
		"EmptyStruct": {
			reason: "An empty structpb.Struct has no fields to validate against, so it must error rather than permit everything.",
			args:   args{schema: &structpb.Struct{}},
			want:   want{err: cmpopts.AnyError},
		},
		"WrongShapeEnvelope": {
			reason: "A JSON object that isn't an OpenAPI v3 schema (e.g. an outer envelope) must not silently decode into a permissive zero-value schema.",
			args: args{schema: mustStruct(t, map[string]any{
				"components": map[string]any{
					"schemas": map[string]any{},
				},
			})},
			want: want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewValidator(tc.args.schema)
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nNewValidator(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	compiled, err := NewValidator(releaseSchema(t))
	if err != nil {
		t.Fatalf("NewValidator(...): %v", err)
	}

	type args struct {
		v   *Validator
		obj map[string]any
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Valid": {
			reason: "An object satisfying the schema validates.",
			args: args{v: compiled, obj: map[string]any{
				"spec": map[string]any{
					"forProvider": map[string]any{
						"chart": map[string]any{"name": "cert-manager"},
						"wait":  true,
					},
				},
			}},
		},
		"MissingRequiredField": {
			reason: "A missing required field is a violation.",
			args: args{v: compiled, obj: map[string]any{
				"spec": map[string]any{
					"forProvider": map[string]any{
						"chart": map[string]any{},
					},
				},
			}},
			want: want{err: cmpopts.AnyError},
		},
		"WrongType": {
			reason: "A field of the wrong type is a violation.",
			args: args{v: compiled, obj: map[string]any{
				"spec": map[string]any{
					"forProvider": map[string]any{
						"chart": map[string]any{"name": "cert-manager"},
						"wait":  "yes",
					},
				},
			}},
			want: want{err: cmpopts.AnyError},
		},
		"NilValidator": {
			reason: "A validator that was never compiled must not pass everything.",
			args:   args{v: nil, obj: map[string]any{}},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.args.v.Validate(tc.args.obj)
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nValidate(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}
