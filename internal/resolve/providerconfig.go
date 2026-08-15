package resolve

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/crossplane/function-sdk-go/errors"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
	"github.com/jonasz-lasut/function-aicr/internal/compose"
)

// The provider config kinds provider-helm and provider-kubernetes accept.
const (
	providerConfigKindCluster    = "ClusterProviderConfig"
	providerConfigKindNamespaced = "ProviderConfig"
)

// ProviderConfigRef resolves the provider config every composed resource
// references, from the function input and the observed composite resource,
// with the same rules as Criteria: a providerConfigRefFrom field path that
// resolves to a non-empty string wins over its providerConfigRef literal; a
// path that is absent or empty falls back to the literal; a malformed path or
// a non-string value is an error. The name is required. The kind defaults to
// ClusterProviderConfig and must be a kind the providers accept — a value
// pulled off the composite resource bypasses the Input schema's enum.
func ProviderConfigRef(in *v1beta1.Input, oxr *fieldpath.Paved) (compose.ProviderConfigRef, error) {
	literal := in.ProviderConfigRef
	if literal == nil {
		literal = &v1beta1.ProviderConfigRef{}
	}
	from := in.ProviderConfigRefFrom
	if from == nil {
		from = &v1beta1.ProviderConfigRefFrom{}
	}

	name, err := resolveString(oxr, from.Name, literal.Name)
	if err != nil {
		return compose.ProviderConfigRef{}, err
	}
	if name == "" {
		return compose.ProviderConfigRef{}, errors.New("providerConfigRef.name is required: set it, or a providerConfigRefFrom.name field path that resolves on the composite resource")
	}

	kind, err := resolveString(oxr, from.Kind, literal.Kind)
	if err != nil {
		return compose.ProviderConfigRef{}, err
	}
	switch kind {
	case "":
		kind = providerConfigKindCluster
	case providerConfigKindCluster, providerConfigKindNamespaced:
	default:
		return compose.ProviderConfigRef{}, errors.Errorf("providerConfigRef.kind must be %s or %s, got %q", providerConfigKindCluster, providerConfigKindNamespaced, kind)
	}

	return compose.ProviderConfigRef{Kind: kind, Name: name}, nil
}
