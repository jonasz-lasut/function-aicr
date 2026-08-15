// Package resolve turns function input and the observed composite resource
// into validated AICR recipe criteria.
package resolve

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/crossplane/function-sdk-go/errors"

	"github.com/NVIDIA/aicr/pkg/recipe"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
)

// Criteria resolves AICR recipe criteria from the function input and the
// observed composite resource. A criteriaFrom field path that resolves to a
// non-empty string wins over its literal; a path that is absent or empty falls
// back to the literal; a malformed path or a non-string value at a path is an
// error.
//
// No single dimension is required — AICR's catalog has recipes without an
// accelerator (ocp, bcm) and accelerator-only tiers — but at least one must
// resolve, so an input whose criteriaFrom paths all miss the composite is an
// error rather than a silent deployment of the base recipe. Every unresolved
// dimension is passed to AICR unstated, so resolution selects the agnostic
// tier for it; AICR itself rejects a stated value no recipe honors, and asks
// for an OS when a service+accelerator pair has only OS-specific recipes.
func Criteria(in *v1beta1.Input, oxr *fieldpath.Paved) (*recipe.Criteria, error) {
	literal := in.Criteria
	if literal == nil {
		literal = &v1beta1.Criteria{}
	}
	from := in.CriteriaFrom
	if from == nil {
		from = &v1beta1.CriteriaFrom{}
	}

	accelerator, err := resolveString(oxr, from.Accelerator, literal.Accelerator)
	if err != nil {
		return nil, err
	}
	service, err := resolveString(oxr, from.Service, literal.Service)
	if err != nil {
		return nil, err
	}
	intent, err := resolveString(oxr, from.Intent, literal.Intent)
	if err != nil {
		return nil, err
	}
	os, err := resolveString(oxr, from.OS, literal.OS)
	if err != nil {
		return nil, err
	}
	platform, err := resolveString(oxr, from.Platform, literal.Platform)
	if err != nil {
		return nil, err
	}

	if accelerator == "" && service == "" && intent == "" && os == "" && platform == "" {
		return nil, errors.New("no criteria resolved: set at least one of criteria.{accelerator,service,intent,os,platform}, or a criteriaFrom field path that resolves on the composite resource")
	}

	return &recipe.Criteria{
		Accelerator: recipe.CriteriaAcceleratorType(accelerator),
		Service:     recipe.CriteriaServiceType(service),
		Intent:      recipe.CriteriaIntentType(intent),
		OS:          recipe.CriteriaOSType(os),
		Platform:    recipe.CriteriaPlatformType(platform),
	}, nil
}

func resolveString(oxr *fieldpath.Paved, path *string, literal string) (string, error) {
	if path == nil {
		return literal, nil
	}
	v, err := oxr.GetString(*path)
	switch {
	case fieldpath.IsNotFound(err):
		return literal, nil
	case err != nil:
		return "", errors.Wrapf(err, "cannot resolve field path %q on the observed composite resource", *path)
	case v == "":
		return literal, nil
	}
	return v, nil
}
