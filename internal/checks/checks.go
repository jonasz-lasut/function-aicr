// Package checks runs the bundle checks AICR's component registry declares.
//
// AICR's bundler runs them before it writes a bundle: a registry entry lists
// validation functions for its component, each with a severity and the
// criteria it applies to, and an error refuses the bundle. They guard
// combinations that deploy cleanly and then break a cluster, such as a
// toleration without a key on AKS, which deadlocks NodeWright's packages, or
// a tuning resource on OKE that reboots every node it is not scoped away
// from.
//
// The check functions are AICR's own (aicr.run/pkg/bundler/validations), so
// nothing is copied here. What this package does is describe a request to
// them the way the command line would: as a bundle configuration.
package checks

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"aicr.run/pkg/bundler/config"
	"aicr.run/pkg/bundler/validations"
	"aicr.run/pkg/defaults"
	"aicr.run/pkg/recipe"

	"github.com/jonasz-lasut/function-aicr/internal/compose"
)

// Request is what the checks need to know of a request besides the resolved
// recipe.
type Request struct {
	// Scheduling is the node placement the Input states.
	Scheduling compose.NodeScheduling
	// Skipped names the components the Input leaves out with skipComponents.
	// AICR's checks then treat them as its --set <component>:enabled=false.
	Skipped []string
	// Values holds the componentOverrides values, per component, which the
	// checks read as AICR's --set-json overrides.
	Values map[string]map[string]any
	// DRAEviction says whether the DRA eviction contract is wired.
	DRAEviction bool
}

// A Finding is one result of a check.
type Finding struct {
	// Component is the component whose registry entry declares the check.
	Component string
	// Message is AICR's, with the flags of its command line replaced by the
	// Input fields that stand for them.
	Message string
	// Blocking is true when AICR would refuse to build a bundle.
	Blocking bool
}

// Config returns the bundle configuration AICR's command line would build
// for the request. Only what the Input can state is set. In particular the
// tolerations are the Input's and not the tolerate-all default of the command
// line: the function writes no default, and the checks know what a component
// renders when none is given.
func Config(r Request) *config.Config {
	disabled := make(map[string]map[string]string, len(r.Skipped))
	for _, name := range r.Skipped {
		disabled[name] = map[string]string{config.ComponentEnabledKey: "false"}
	}

	var typed []config.TypedComponentPath
	for _, name := range slices.Sorted(maps.Keys(r.Values)) {
		for _, path := range slices.Sorted(maps.Keys(r.Values[name])) {
			typed = append(typed, config.TypedComponentPath{Component: name, Path: path, Value: r.Values[name][path]})
		}
	}

	opts := []config.Option{
		config.WithSystemNodeSelector(r.Scheduling.System.NodeSelector),
		config.WithSystemNodeTolerations(r.Scheduling.System.Tolerations),
		config.WithAcceleratedNodeSelector(r.Scheduling.Accelerated.NodeSelector),
		config.WithAcceleratedNodeTolerations(r.Scheduling.Accelerated.Tolerations),
		config.WithValueOverrides(disabled),
		config.WithValueOverridesTypedPaths(typed),
	}
	if r.DRAEviction {
		opts = append(opts, config.WithDRAEvictionNodeLabel(config.NodeLabel{
			Key:   defaults.DRAEvictionNodeLabelKey,
			Value: defaults.DRAEvictionNodeLabelValue,
		}))
	}
	return config.NewConfig(opts...)
}

// Run runs the checks the registry declares for the named components and
// returns their findings in the order of the names. A check AICR rates info
// is logged by AICR and reported nowhere, as in its bundler.
func Run(ctx context.Context, res *recipe.RecipeResult, cfg *config.Config, registry *recipe.ComponentRegistry, components []string) []Finding {
	var findings []Finding
	for _, name := range components {
		comp := registry.Get(name)
		if comp == nil || len(comp.GetValidations()) == 0 {
			continue
		}
		warnings, errs := validations.RunValidations(ctx, name, comp.GetValidations(), res, cfg)
		for _, w := range warnings {
			findings = append(findings, Finding{Component: name, Message: message(name, w)})
		}
		for _, err := range errs {
			findings = append(findings, Finding{Component: name, Message: message(name, err.Error()), Blocking: true})
		}
	}
	return findings
}

// message says whose check reported and puts AICR's text in the Input's
// terms.
func message(component, text string) string {
	return fmt.Sprintf("AICR's bundle check for %q: %s", component, inputFields.Replace(text))
}

// inputFields names the Input field for each flag of AICR's command line
// that a check's message can tell the reader to pass.
var inputFields = strings.NewReplacer(
	"--accelerated-node-selector", "nodeScheduling.accelerated.nodeSelector",
	"--accelerated-node-toleration", "nodeScheduling.accelerated.tolerations",
	"--system-node-selector", "nodeScheduling.system.nodeSelector",
	"--system-node-toleration", "nodeScheduling.system.tolerations",
)
