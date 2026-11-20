package resolve

import (
	"maps"
	"slices"
	"strings"

	"aicr.run/pkg/recipe"
	"github.com/crossplane/function-sdk-go/errors"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
)

// Profile returns the profile selection the resolved Input states, in the
// name=value form AICR accepts, or "" for the recipe's default variant. AICR
// accepts one selection per recipe, so more than one is an error here; which
// names and values exist is AICR's to decide.
func Profile(in *v1beta1.Input) (string, error) {
	selected := make([]string, 0, len(in.Profile))
	for _, name := range slices.Sorted(maps.Keys(in.Profile)) {
		if value := in.Profile[name]; value != "" {
			selected = append(selected, name+"="+value)
		}
	}
	switch len(selected) {
	case 0:
		return "", nil
	case 1:
		return selected[0], nil
	default:
		return "", errors.Errorf("profile selects %d variants (%s); AICR accepts one selection per recipe", len(selected), strings.Join(selected, ", "))
	}
}

// BuildOptions returns the generation-time choices the resolved Input states
// under configuration. Each is passed to AICR only when set, because AICR
// rejects one on a recipe it does not apply to; AICR validates the values.
func BuildOptions(in *v1beta1.Input) []recipe.BuildOption {
	cfg := in.Configuration
	if cfg == nil {
		return nil
	}
	var opts []recipe.BuildOption
	if cfg.GKE != nil && len(cfg.GKE.TCPXOInterfaces) > 0 {
		mapping := make([]recipe.NetworkInterfaceMapping, 0, len(cfg.GKE.TCPXOInterfaces))
		for _, m := range cfg.GKE.TCPXOInterfaces {
			mapping = append(mapping, recipe.NetworkInterfaceMapping{InterfaceName: m.InterfaceName, Network: m.Network})
		}
		opts = append(opts, recipe.WithGKETCPXOInterfaces(mapping))
	}
	if cfg.RuntimeInventory != nil && cfg.RuntimeInventory.Mode != "" {
		opts = append(opts, recipe.WithRuntimeInventoryMode(recipe.RuntimeInventoryMode(cfg.RuntimeInventory.Mode)))
	}
	return opts
}
