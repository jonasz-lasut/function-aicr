package compose

import (
	"maps"
	"strings"

	"aicr.run/pkg/recipe"
)

// The composed provider-helm Release GVK. Namespaced, so it can be composed by
// a namespaced composite resource.
const (
	ReleaseAPIVersion = "helm.m.crossplane.io/v1beta1"
	ReleaseKind       = "Release"
)

// A ProviderConfigRef references the provider config for the target cluster.
type ProviderConfigRef struct {
	Kind string
	Name string
}

// An Override customizes one resolved recipe component.
type Override struct {
	Version   string
	Namespace string
	Values    map[string]any
}

// NewRelease builds a provider-helm Release for an AICR component, applying any
// override and stamping the target cluster's provider config.
func NewRelease(c *recipe.ComponentRef, values map[string]any, ov *Override, pcRef ProviderConfigRef) map[string]any {
	namespace, version, values := effective(c, values, ov)
	repository, chart := SplitChart(c.Source, c.EffectiveChart())

	chartSpec := map[string]any{"name": chart}
	if repository != "" {
		chartSpec["repository"] = repository
	}
	if version != "" {
		chartSpec["version"] = version
	}

	forProvider := map[string]any{
		"chart":     chartSpec,
		"namespace": namespace,
		// Wait so the Release reports Ready only once its resources are ready —
		// the readiness signal the dependency gate relies on.
		"wait": true,
	}
	if len(values) > 0 {
		forProvider["values"] = values
	}

	return map[string]any{
		"apiVersion": ReleaseAPIVersion,
		"kind":       ReleaseKind,
		"metadata":   map[string]any{"name": c.Name},
		"spec": map[string]any{
			"forProvider":       forProvider,
			"providerConfigRef": providerConfigRef(pcRef),
		},
	}
}

// effective returns the namespace, chart version and values a component is
// deployed with: the recipe's, with any override applied. Both the Release and
// the component's manifests derive from these, so they cannot disagree.
func effective(c *recipe.ComponentRef, values map[string]any, ov *Override) (namespace, version string, effectiveValues map[string]any) {
	namespace, version = Placement(c, ov)
	if ov != nil && ov.Values != nil {
		values = MergeValues(values, ov.Values)
	}
	return namespace, version, values
}

// Placement returns the namespace and chart version a component is deployed
// with: the recipe's, with any override applied.
func Placement(c *recipe.ComponentRef, ov *Override) (namespace, version string) {
	namespace, version = c.Namespace, c.Version
	if ov == nil {
		return namespace, version
	}
	if ov.Namespace != "" {
		namespace = ov.Namespace
	}
	if ov.Version != "" {
		version = ov.Version
	}
	return namespace, version
}

func providerConfigRef(pcRef ProviderConfigRef) map[string]any {
	return map[string]any{"kind": pcRef.Kind, "name": pcRef.Name}
}

// SplitChart derives the provider-helm repository and chart name from an AICR
// component's source and chart. An OCI chart is at the source with the chart
// name appended, which is how AICR's deployers form the reference and how
// provider-helm joins the two. That holds when the source already ends in the
// chart's name: kai-scheduler is at
// oci://ghcr.io/kai-scheduler/kai-scheduler/kai-scheduler.
func SplitChart(source, chart string) (repository, name string) {
	return strings.TrimRight(source, "/"), chart
}

// MergeValues deep-merges override onto base and never mutates either;
// nested maps that override does not touch are shared with base, not copied.
// Override wins; nested maps merge recursively, every other type replaces.
func MergeValues(base, override map[string]any) map[string]any {
	out := make(map[string]any, len(base))
	maps.Copy(out, base)
	for k, v := range override {
		if ov, ok := v.(map[string]any); ok {
			if bv, ok := out[k].(map[string]any); ok {
				out[k] = MergeValues(bv, ov)
				continue
			}
		}
		out[k] = v
	}
	return out
}
