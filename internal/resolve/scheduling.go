package resolve

import (
	"aicr.run/pkg/bundler/config"
	"github.com/crossplane/function-sdk-go/errors"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
	"github.com/jonasz-lasut/function-aicr/internal/compose"
)

// NodeScheduling returns the node placement the resolved Input states. AICR
// validates the selectors and tolerations as its bundler does for the flags
// they stand for: a node selector must be made of valid labels, and a
// toleration must be one the API server accepts.
func NodeScheduling(in *v1beta1.Input) (compose.NodeScheduling, error) {
	var s compose.NodeScheduling
	if in.NodeScheduling == nil {
		return s, nil
	}
	tiers := []struct {
		name string
		from *v1beta1.NodePlacement
		to   *compose.NodePlacement
	}{
		{"system", in.NodeScheduling.System, &s.System},
		{"accelerated", in.NodeScheduling.Accelerated, &s.Accelerated},
	}
	for _, tier := range tiers {
		if tier.from == nil {
			continue
		}
		if err := config.ValidateNodeSelector(tier.from.NodeSelector); err != nil {
			return compose.NodeScheduling{}, errors.Wrapf(err, "invalid nodeScheduling.%s.nodeSelector", tier.name)
		}
		if err := config.ValidateTolerations(tier.from.Tolerations); err != nil {
			return compose.NodeScheduling{}, errors.Wrapf(err, "invalid nodeScheduling.%s.tolerations", tier.name)
		}
		tier.to.NodeSelector = tier.from.NodeSelector
		tier.to.Tolerations = tier.from.Tolerations
	}
	return s, nil
}
