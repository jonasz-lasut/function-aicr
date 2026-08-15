// Package schema validates composed resources against the OpenAPI v3 schemas
// Crossplane resolves for their CRDs.
package schema

import (
	"encoding/json"

	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"

	"github.com/crossplane/function-sdk-go/errors"
)

// A Validator checks composed resources against one OpenAPI v3 schema. The
// schema is compiled once, by NewValidator; validating a resource against it
// is then cheap, so a request that composes many resources of one kind should
// share a single Validator.
type Validator struct {
	v validation.SchemaValidator
}

// NewValidator compiles the supplied OpenAPI v3 schema.
func NewValidator(s *structpb.Struct) (*Validator, error) {
	if s == nil || len(s.GetFields()) == 0 {
		return nil, errors.New("cannot validate against a nil schema")
	}

	props, err := jsonSchemaProps(s)
	if err != nil {
		return nil, err
	}

	v, _, err := validation.NewSchemaValidator(props)
	if err != nil {
		return nil, errors.Wrap(err, "cannot build schema validator")
	}
	return &Validator{v: v}, nil
}

// Validate checks obj against the schema.
func (v *Validator) Validate(obj map[string]any) error {
	if v == nil {
		return errors.New("cannot validate against a nil schema")
	}
	if errs := validation.ValidateCustomResource(nil, obj, v.v); len(errs) > 0 {
		return errors.Wrap(errs.ToAggregate(), "composed resource does not match its schema")
	}
	return nil
}

// jsonSchemaProps converts Crossplane's protobuf-encoded openAPIV3Schema into
// the internal type apiextensions' validator expects.
func jsonSchemaProps(s *structpb.Struct) (*apiextensions.JSONSchemaProps, error) {
	raw, err := json.Marshal(s.AsMap())
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal schema")
	}

	versioned := &apiextensionsv1.JSONSchemaProps{}
	if err := json.Unmarshal(raw, versioned); err != nil {
		return nil, errors.Wrap(err, "cannot parse schema as JSONSchemaProps")
	}

	// json.Unmarshal silently zero-values fields it doesn't recognise, so an
	// empty object or an unrelated envelope decodes without error into an
	// empty schema that would validate any resource. A usable
	// openAPIV3Schema always declares at least one of these.
	if versioned.Type == "" && len(versioned.Properties) == 0 && versioned.Ref == nil {
		return nil, errors.New("schema is empty or not an OpenAPI v3 object schema")
	}

	internal := &apiextensions.JSONSchemaProps{}
	if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(versioned, internal, nil); err != nil {
		return nil, errors.Wrap(err, "cannot convert schema")
	}
	return internal, nil
}
