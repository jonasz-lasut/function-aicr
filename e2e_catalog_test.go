package main

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"aicr.run/pkg/recipe"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
	"github.com/jonasz-lasut/function-aicr/internal/compose"
)

// The end-to-end suite serves the catalog under test/e2e/catalog/recipe over
// the embedded one, as an operator's OCI recipe source is. It stays hermetic
// only while the criteria it uses select nothing but its own overlays, which
// an AICR bump can change by adding a stock overlay that matches them. This
// test fails then, long before a cluster would deploy a stock chart.
func TestEndToEndCatalog(t *testing.T) {
	dp, err := recipe.NewLayeredDataProvider(embeddedDataProvider(), recipe.LayeredProviderConfig{ExternalDir: "test/e2e/catalog/recipe"})
	if err != nil {
		t.Fatalf("recipe.NewLayeredDataProvider(...): %v", err)
	}
	registry, err := recipe.GetComponentRegistryFor(dp)
	if err != nil {
		t.Fatalf("recipe.GetComponentRegistryFor(...): %v", err)
	}

	type args struct {
		criteria *recipe.Criteria
	}
	type want struct {
		order      []string
		namespaces map[string]string
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Kind": {
			reason: "The recipe every feature starts from is the catalog's base alone, in dependency order.",
			args:   args{criteria: &recipe.Criteria{Service: recipe.CriteriaServiceKind}},
			want: want{
				order:      []string{"e2e-first", "e2e-second", "e2e-notes"},
				namespaces: map[string]string{"e2e-first": "e2e-first", "e2e-second": "e2e-second", "e2e-notes": "e2e-second"},
			},
		},
		"KindTraining": {
			reason: "The recipe a feature moves to places e2e-first in another namespace and changes nothing else.",
			args:   args{criteria: &recipe.Criteria{Service: recipe.CriteriaServiceKind, Intent: recipe.CriteriaIntentTraining}},
			want: want{
				order:      []string{"e2e-first", "e2e-second", "e2e-notes"},
				namespaces: map[string]string{"e2e-first": "e2e-first-moved", "e2e-second": "e2e-second", "e2e-notes": "e2e-second"},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := recipe.NewBuilder(recipe.WithVersion(testRecipeVersion), recipe.WithDataProvider(dp)).
				BuildFromCriteria(context.Background(), tc.args.criteria)
			if err != nil {
				t.Fatalf("%s\nBuildFromCriteria(%s): %v", tc.reason, tc.args.criteria, err)
			}

			managed := managedComponents(nil, res, &v1beta1.Input{}, registry)
			namespaces := make(map[string]string, len(managed))
			for name, c := range managed {
				namespaces[name], _ = compose.Placement(c, nil)
			}

			if diff := cmp.Diff(tc.want.order, res.DeploymentOrder); diff != "" {
				t.Errorf("%s\nDeploymentOrder: -want, +got:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.namespaces, namespaces); diff != "" {
				t.Errorf("%s\nmanaged components and their namespaces: -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
