// Package resolve turns the function input and the observed composite
// resource into the Input a composite resource is reconciled with, and into
// the validated AICR recipe criteria and provider config it names.
package resolve

import (
	"bytes"
	"encoding/json"
	"maps"

	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/crossplane/function-sdk-go/errors"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
	"github.com/jonasz-lasut/function-aicr/internal/compose"
)

// Fields the Input no longer has. A Composition that still sets one is told
// what replaced it instead of an unknown-field error.
var removedFields = []string{"criteriaFrom", "providerConfigRefFrom"}

// Keys an overlay must not set: where the overlay is read from, and the
// Input's own identity.
var reservedOverlayKeys = []string{"overlayFrom", "apiVersion", "kind", "metadata"}

// Input resolves the Input a composite resource is reconciled with: the
// static Input from the Composition with, when overlayFrom names one, the
// overlay object read from the observed composite resource merged over it.
//
// A field path that is absent from the composite resource leaves the static
// Input as it is. A path that holds anything but an object, an overlay that
// sets a reserved key, and a field the Input does not have are errors, never
// a silent fallback: the static Input and the merged result are both decoded
// strictly.
func Input(static map[string]any, oxr *fieldpath.Paved) (*v1beta1.Input, error) {
	for _, field := range removedFields {
		if _, ok := static[field]; ok {
			return nil, errors.Errorf("%s was removed: read values from the composite resource with overlayFrom.fieldPath", field)
		}
	}
	in, err := decode(static)
	if err != nil {
		return nil, errors.Wrap(err, "invalid Input")
	}
	if in.OverlayFrom == nil {
		return in, nil
	}

	path := in.OverlayFrom.FieldPath
	if path == "" {
		return nil, errors.New("overlayFrom.fieldPath is required")
	}
	v, err := oxr.GetValue(path)
	switch {
	case fieldpath.IsNotFound(err):
		return in, nil
	case err != nil:
		return nil, errors.Wrapf(err, "cannot resolve overlayFrom.fieldPath %q on the observed composite resource", path)
	case v == nil:
		return in, nil
	}
	overlay, ok := v.(map[string]any)
	if !ok {
		return nil, errors.Errorf("overlayFrom.fieldPath %q must name an object on the observed composite resource, got %s", path, jsonKind(v))
	}
	for _, key := range reservedOverlayKeys {
		if _, ok := overlay[key]; ok {
			return nil, errors.Errorf("the overlay at %q must not set %s", path, key)
		}
	}

	merged, err := decode(merge(static, overlay, nil))
	return merged, errors.Wrapf(err, "invalid Input after applying the overlay at %q", path)
}

// decode decodes m into an Input, rejecting any field the Input does not have.
func decode(m map[string]any) (*v1beta1.Input, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	in := &v1beta1.Input{}
	if err := dec.Decode(in); err != nil {
		return nil, err
	}
	return in, nil
}

// merge deep-merges overlay over base and mutates neither. Maps merge key by
// key with the overlay winning; every other type, lists included, replaces.
// A null or an empty string in the overlay counts as unset and keeps the base
// value, because the Kubernetes API cannot tell an empty optional string from
// an omitted one. Chart values are the exception: under
// componentOverrides.<component>.values an empty string or a null is a value
// like any other.
func merge(base, overlay map[string]any, path []string) map[string]any {
	out := maps.Clone(base)
	if out == nil {
		out = make(map[string]any, len(overlay))
	}
	for k, v := range overlay {
		switch ov := v.(type) {
		case nil:
			continue
		case string:
			if ov == "" {
				continue
			}
			out[k] = ov
		case map[string]any:
			bv, _ := out[k].(map[string]any)
			if len(path) == 2 && path[0] == "componentOverrides" && k == "values" {
				out[k] = compose.MergeValues(bv, ov)
				continue
			}
			out[k] = merge(bv, ov, append(path, k))
		default:
			out[k] = v
		}
	}
	return out
}

// jsonKind names the JSON kind of a value read from the composite resource.
func jsonKind(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "number"
	}
}
