package resolve

import (
	"github.com/crossplane/function-sdk-go/errors"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
	"github.com/jonasz-lasut/function-aicr/internal/compose"
)

// The provider config kinds provider-helm and provider-kubernetes accept.
const (
	providerConfigKindCluster    = "ClusterProviderConfig"
	providerConfigKindNamespaced = "ProviderConfig"
)

// ProviderConfigRef returns the provider config every composed resource
// references, from the resolved Input. The name is required. The kind
// defaults to ClusterProviderConfig and must be a kind the providers accept:
// a value the overlay read off the composite resource bypasses the Input
// schema's enum.
func ProviderConfigRef(in *v1beta1.Input) (compose.ProviderConfigRef, error) {
	ref := in.ProviderConfigRef
	if ref == nil {
		ref = &v1beta1.ProviderConfigRef{}
	}
	if ref.Name == "" {
		return compose.ProviderConfigRef{}, errors.New("providerConfigRef.name is required: set it in the Input, or in the overlay overlayFrom.fieldPath reads from the composite resource")
	}
	kind := ref.Kind
	switch kind {
	case "":
		kind = providerConfigKindCluster
	case providerConfigKindCluster, providerConfigKindNamespaced:
	default:
		return compose.ProviderConfigRef{}, errors.Errorf("providerConfigRef.kind must be %s or %s, got %q", providerConfigKindCluster, providerConfigKindNamespaced, kind)
	}
	return compose.ProviderConfigRef{Kind: kind, Name: ref.Name}, nil
}
