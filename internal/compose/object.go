// Package compose builds the composed resources an AICR recipe expands into.
package compose

import (
	"fmt"
	"regexp"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"
)

// docSepRE matches a YAML document separator line.
var docSepRE = regexp.MustCompile(`(?m)^---[ \t]*(\n|$)`)

// nonAlphanumRE matches the runs of characters a manifest's metadata.name is
// slugged over when it names an Object.
var nonAlphanumRE = regexp.MustCompile(`[^a-z0-9]+`)

// SplitManifests splits a multi-document YAML byte slice into individual
// documents. Blank documents are skipped. Returns an error if any document
// fails to parse.
func SplitManifests(data []byte) ([]map[string]any, error) {
	parts := docSepRE.Split(string(data), -1)
	var docs []map[string]any
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		var doc map[string]any
		if err := sigsyaml.Unmarshal([]byte(part), &doc); err != nil {
			return nil, err
		}
		if len(doc) > 0 {
			docs = append(docs, doc)
		}
	}
	return docs, nil
}

// ManifestObjectName derives the composed-resource name for a manifest
// document. The dependency gate correlates observed against desired resources
// by this name, so it must be a pure function of its inputs.
func ManifestObjectName(component, group string, doc map[string]any, index int) string {
	kind, _ := doc["kind"].(string)
	name := ""
	if meta, ok := doc["metadata"].(map[string]any); ok {
		name, _ = meta["name"].(string)
	}
	if kind != "" && name != "" {
		slug := nonAlphanumRE.ReplaceAllString(strings.ToLower(name), "-")
		slug = strings.Trim(slug, "-")
		return strings.Join([]string{component, group, strings.ToLower(kind), slug}, "-")
	}
	return fmt.Sprintf("%s-%s-%d", component, group, index)
}

// The composed provider-kubernetes Object GVK. Namespaced, so it can be
// composed by a namespaced composite resource.
const (
	ObjectAPIVersion = "kubernetes.m.crossplane.io/v1alpha1"
	ObjectKind       = "Object"
)

// NewObject builds a provider-kubernetes Object carrying manifest.
func NewObject(manifest map[string]any, name string, pcRef ProviderConfigRef) map[string]any {
	return map[string]any{
		"apiVersion": ObjectAPIVersion,
		"kind":       ObjectKind,
		"metadata":   map[string]any{"name": name},
		"spec": map[string]any{
			"forProvider":       map[string]any{"manifest": manifest},
			"providerConfigRef": providerConfigRef(pcRef),
		},
	}
}

// NamespaceObjectName returns the name of the Object that makes a component's
// namespace exist ahead of its manifests in a wave. It is the name a
// Namespace document in the component's own manifest files gets, so a recipe
// that declares the namespace itself is recognized by it.
func NamespaceObjectName(component, wave, namespace string) string {
	return ManifestObjectName(component, wave, namespaceManifest(namespace), 0)
}

// NewNamespaceObject returns an Object that makes a namespace exist and
// otherwise leaves it alone, as Helm's --create-namespace does: a namespace
// that is already there is only observed, and deleting the Object does not
// delete the namespace, which may hold more than this component.
func NewNamespaceObject(namespace, name string, pcRef ProviderConfigRef) map[string]any {
	o := NewObject(namespaceManifest(namespace), name, pcRef)
	o["spec"].(map[string]any)["managementPolicies"] = []any{"Observe", "Create"}
	return o
}

func namespaceManifest(namespace string) map[string]any {
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   map[string]any{"name": namespace},
	}
}
