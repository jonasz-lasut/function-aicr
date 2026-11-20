package resolve

import (
	"aicr.run/pkg/recipe"
	"github.com/crossplane/function-sdk-go/errors"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
)

// Criteria returns the AICR recipe criteria the resolved Input states.
//
// No single dimension is required — AICR's catalog has recipes without an
// accelerator (ocp, bcm) and accelerator-only tiers — but at least one must
// be set, so an Input whose overlay misses the composite resource is an error
// rather than a silent deployment of the base recipe. Every unset dimension
// is passed to AICR unstated, so resolution selects the agnostic tier for it;
// AICR itself rejects a stated value no recipe honors, and asks for an OS
// when a service+accelerator pair has only OS-specific recipes.
func Criteria(in *v1beta1.Input) (*recipe.Criteria, error) {
	c := in.Criteria
	if c == nil {
		c = &v1beta1.Criteria{}
	}
	if c.Accelerator == "" && c.Service == "" && c.Intent == "" && c.OS == "" && c.Platform == "" {
		return nil, errors.New("no criteria resolved: set at least one of criteria.{accelerator,service,intent,os,platform} in the Input, or in the overlay overlayFrom.fieldPath reads from the composite resource")
	}
	return &recipe.Criteria{
		Accelerator: recipe.CriteriaAcceleratorType(c.Accelerator),
		Service:     recipe.CriteriaServiceType(c.Service),
		Intent:      recipe.CriteriaIntentType(c.Intent),
		OS:          recipe.CriteriaOSType(c.OS),
		Platform:    recipe.CriteriaPlatformType(c.Platform),
	}, nil
}
