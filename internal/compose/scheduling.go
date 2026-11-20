package compose

import (
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"aicr.run/pkg/component"
	"aicr.run/pkg/recipe"

	"github.com/crossplane/function-sdk-go/errors"
)

// NodePlacement is where one kind of workload may run.
type NodePlacement struct {
	NodeSelector map[string]string
	Tolerations  []corev1.Toleration
}

// NodeScheduling is the placement of the two kinds of workload AICR's
// component registry tells apart.
type NodeScheduling struct {
	System      NodePlacement
	Accelerated NodePlacement
}

// ApplyNodeScheduling writes the node selectors and tolerations into a
// component's values, at the paths its registry entry names, and returns the
// selector paths it left alone because the recipe opts them out.
//
// It is a copy of what AICR's bundler does with --system-node-selector,
// --system-node-toleration and their accelerated twins
// (applyNodeSchedulingOverrides and classifySchedulingPaths in
// aicr.run/pkg/bundler/bundler.go), which keeps the orchestration unexported
// and exports the helpers used here. The recipe's own overrides of a path
// decide what happens there: an empty value opts the path out, a non-empty
// list of tolerations is appended to, and everything else is replaced. The
// component's values file is deliberately not consulted, so a chart default
// cannot turn the input into a no-op. A toleration path the registry names
// for both kinds of workload gets both lists.
//
// The bundler does this before the user's structured overrides are merged,
// so componentOverrides values win over what is written here.
func ApplyNodeScheduling(values map[string]any, comp *recipe.ComponentConfig, ref *recipe.ComponentRef, s NodeScheduling) (optedOut map[string]bool) {
	if values == nil || comp == nil {
		return nil
	}
	systemSelectors, acceleratedSelectors := comp.GetSystemNodeSelectorPaths(), comp.GetAcceleratedNodeSelectorPaths()
	systemTolerations, acceleratedTolerations := comp.GetSystemTolerationPaths(), comp.GetAcceleratedTolerationPaths()

	optedOut, appended := classifySchedulingPaths(ref,
		slices.Concat(systemSelectors, systemTolerations, acceleratedSelectors, acceleratedTolerations))
	keep := func(paths []string) []string {
		return slices.DeleteFunc(slices.Clone(paths), func(p string) bool { return optedOut[p] })
	}

	if paths := keep(systemSelectors); len(paths) > 0 {
		component.ApplyNodeSelectorOverrides(values, s.System.NodeSelector, paths...)
	}
	if paths := keep(acceleratedSelectors); len(paths) > 0 {
		component.ApplyNodeSelectorOverrides(values, s.Accelerated.NodeSelector, paths...)
	}

	shared := component.SharedTolerationPaths(systemTolerations, acceleratedTolerations)
	unshared := func(paths []string) []string {
		return slices.DeleteFunc(keep(paths), func(p string) bool { return slices.Contains(shared, p) })
	}
	applyTolerations(values, s.System.Tolerations, unshared(systemTolerations), appended)
	applyTolerations(values, s.Accelerated.Tolerations, unshared(acceleratedTolerations), appended)
	applyTolerations(values, component.SharedTolerations(s.System.Tolerations, s.Accelerated.Tolerations), keep(shared), appended)

	return optedOut
}

// classifySchedulingPaths reads the recipe's inline overrides of a component:
// a path they set to an empty value is opted out, and one they set to
// anything else is appended to.
func classifySchedulingPaths(ref *recipe.ComponentRef, paths []string) (optedOut, appended map[string]bool) {
	optedOut, appended = make(map[string]bool), make(map[string]bool)
	if ref == nil || ref.Overrides == nil {
		return optedOut, appended
	}
	for _, p := range paths {
		v, ok := component.GetValueByPath(ref.Overrides, p)
		if !ok {
			continue
		}
		if isEmptyValue(v) {
			optedOut[p] = true
			continue
		}
		appended[p] = true
	}
	return optedOut, appended
}

// applyTolerations writes tolerations to the paths: appended where the recipe
// has a list of its own, replacing everywhere else.
func applyTolerations(values map[string]any, tolerations []corev1.Toleration, paths []string, appended map[string]bool) {
	if len(tolerations) == 0 {
		return
	}
	for _, p := range paths {
		if appended[p] {
			component.AppendTolerationsOverrides(values, tolerations, p)
			continue
		}
		component.ApplyTolerationsOverrides(values, tolerations, p)
	}
}

// RequireNodeSelectors returns an error when a component's registry entry
// requires a node selector and its values, with every override applied, have
// none at one of the paths. AICR sets the requirement where the wrong node
// pool silently breaks the component, such as a StatefulSet whose volume ties
// it to a zone, and its bundler refuses to build a bundle then
// (validateRequiredNodeSelectors). A path the recipe opts out is exempt.
func RequireNodeSelectors(name string, values map[string]any, comp *recipe.ComponentConfig, optedOut map[string]bool) error {
	if comp == nil {
		return nil
	}
	tiers := []struct {
		required bool
		paths    []string
		field    string
	}{
		{comp.RequireSystemNodeSelector(), comp.GetSystemNodeSelectorPaths(), "nodeScheduling.system.nodeSelector"},
		{comp.RequireAcceleratedNodeSelector(), comp.GetAcceleratedNodeSelectorPaths(), "nodeScheduling.accelerated.nodeSelector"},
	}
	for _, tier := range tiers {
		if !tier.required {
			continue
		}
		var empty []string
		for _, p := range tier.paths {
			if optedOut[p] {
				continue
			}
			if v, ok := component.GetValueByPath(values, p); !ok || !isNonEmptyMap(v) {
				empty = append(empty, p)
			}
		}
		if len(empty) > 0 {
			return errors.Errorf("component %q requires %s: its values have no node selector at %s", name, tier.field, strings.Join(empty, ", "))
		}
	}
	return nil
}

// isEmptyValue reports whether a recipe override is the deliberate "nothing
// here": an empty list or map, or a null.
func isEmptyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	default:
		return false
	}
}

func isNonEmptyMap(v any) bool {
	m, ok := v.(map[string]any)
	return ok && len(m) > 0
}
