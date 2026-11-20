// Package v1beta1 contains the input type for this Function.
// +kubebuilder:object:generate=true
// +groupName=aicr.fn.crossplane.io
// +versionName=v1beta1
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// Input configures how function-aicr resolves an NVIDIA AICR recipe and
// expands it into composed provider-helm Releases and provider-kubernetes
// Objects.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	// OverlayFrom names an object on the observed composite resource that is
	// merged over this Input, so one Composition can serve composite resources
	// that each name their own recipe and target cluster. Every other field
	// of this Input is static: the value every composite resource gets unless
	// its overlay sets the field.
	// +optional
	OverlayFrom *OverlayFrom `json:"overlayFrom,omitempty"`

	// Criteria are the AICR recipe criteria. At least one dimension must be
	// set, here or by the overlay.
	// +optional
	Criteria *Criteria `json:"criteria,omitempty"`

	// ProviderConfigRef references the provider config holding credentials for
	// the target cluster. It is stamped onto every composed Release and Object.
	// A name is required, here or from the overlay.
	// +optional
	ProviderConfigRef *ProviderConfigRef `json:"providerConfigRef,omitempty"`

	// SkipDeploymentOrder deploys every component at once rather than
	// withholding a component until its dependencies report Ready.
	// +optional
	SkipDeploymentOrder bool `json:"skipDeploymentOrder,omitempty"`

	// SkipComponents are excluded from the resolved recipe.
	// +optional
	SkipComponents []ComponentRef `json:"skipComponents,omitempty"`

	// ComponentOverrides customize resolved components, keyed by component name.
	// +optional
	ComponentOverrides map[string]ComponentOverride `json:"componentOverrides,omitempty"`

	// Target names where on the desired composite resource the resolved-recipe
	// summary is written. When unset no status is written.
	// +optional
	Target *Target `json:"target,omitempty"`
}

// Criteria are literal AICR recipe criteria.
type Criteria struct {
	// +optional
	Accelerator string `json:"accelerator,omitempty"`
	// +optional
	Service string `json:"service,omitempty"`
	// +optional
	Intent string `json:"intent,omitempty"`
	// +optional
	OS string `json:"os,omitempty"`
	// +optional
	Platform string `json:"platform,omitempty"`
}

// An OverlayFrom names where an overlay of the Input is read from.
type OverlayFrom struct {
	// FieldPath names an object on the observed composite resource, shaped
	// like this Input. Its maps are merged over the Input key by key and its
	// lists replace the Input's; an empty string counts as unset. It may set
	// any field but overlayFrom. A path that is absent from the composite
	// resource leaves the Input as it is.
	FieldPath string `json:"fieldPath"`
}

// A ProviderConfigRef references a provider config for the target cluster.
type ProviderConfigRef struct {
	// +kubebuilder:validation:Enum=ClusterProviderConfig;ProviderConfig
	// +kubebuilder:default=ClusterProviderConfig
	// +optional
	Kind string `json:"kind,omitempty"`

	// +optional
	Name string `json:"name,omitempty"`
}

// A ComponentRef names an AICR recipe component.
type ComponentRef struct {
	Name string `json:"name"`
}

// A ComponentOverride customizes one resolved recipe component.
type ComponentOverride struct {
	// +optional
	Version string `json:"version,omitempty"`

	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Values are deep-merged over the values the recipe resolved.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Values *runtime.RawExtension `json:"values,omitempty"`
}

// A Target names where the resolved-recipe summary is written.
type Target struct {
	// FieldPath names the object on the composite resource that receives the
	// summary keys (recipeName, recipeVersion, componentCount,
	// deployedComponents). It must begin with "status". Keys already present
	// at the path are preserved; the summary keys are set alongside them.
	// +kubebuilder:validation:Pattern=`^status(\..+)?$`
	FieldPath string `json:"fieldPath"`
}
