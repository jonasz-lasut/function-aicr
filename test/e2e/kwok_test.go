package e2e

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/features"

	apiextensionsv1 "github.com/crossplane/crossplane/apis/v2/apiextensions/v1"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/crossplane/crossplane/v2/test/e2e/config"
	"github.com/crossplane/crossplane/v2/test/e2e/funcs"
)

// TestSuiteKWOK selects the features that need the cluster of the kwok
// profile (`make e2e.up E2E_PROFILE=kwok`): the embedded catalog, and nodes
// that KWOK simulates.
const TestSuiteKWOK = "kwok"

// LabelAreaStockRecipe is applied to all features that deploy a recipe of
// AICR's own catalog.
const LabelAreaStockRecipe = "stock-recipe"

// TestStockRecipeOnSimulatedNodes deploys one recipe of the embedded catalog
// with its real charts. KWOK reports a pod on a simulated node as running
// without starting a container, so the feature proves what the suite's own
// catalog cannot: the charts are pulled from their registries, their
// manifests are accepted by the API server, every pod finds a node through
// nodeScheduling, and provider-helm sees each release become ready in AICR's
// order. No operator reconciles anything.
func TestStockRecipeOnSimulatedNodes(t *testing.T) {
	const (
		manifests = "manifests/kwok"
		xr        = "xr.yaml"
	)

	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests that a stock recipe, the EKS H100 training recipe, rolls out through function-aicr onto simulated system and accelerated nodes: every component is deployed and every release becomes ready.").
			WithLabel(LabelArea, LabelAreaStockRecipe).
			WithLabel(LabelSize, LabelSizeLarge).
			WithLabel(config.LabelTestSuite, TestSuiteKWOK).
			WithSetup("CreatePrerequisites", funcs.AllOf(
				funcs.ApplyResources(FieldManager, manifests, "setup/*.yaml"),
				funcs.ResourcesCreatedWithin(30*time.Second, manifests, "setup/*.yaml"),
				funcs.ResourcesHaveConditionWithin(1*time.Minute, manifests, "setup/definition.yaml", apiextensionsv1.WatchingComposite()),
			)).
			Assess("CreateXR", funcs.AllOf(
				funcs.ApplyResources(FieldManager, manifests, xr),
				funcs.ResourcesCreatedWithin(30*time.Second, manifests, xr),
			)).
			Assess("RecipeIsResolved", funcs.AllOf(
				funcs.ResourcesHaveFieldValueWithin(2*time.Minute, manifests, xr, "status.aicr.recipeName", "h100-eks-training"),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, manifests, xr, "status.aicr.recipeSource", funcs.NotFound),
			)).
			Assess("EveryComponentIsDeployed",
				funcs.ResourcesHaveFieldValueWithin(10*time.Minute, manifests, xr, "status.aicr.pendingComponents", []any{})).
			Assess("XRIsReady",
				funcs.ResourcesHaveConditionWithin(10*time.Minute, manifests, xr, xpv2.Available(), xpv2.ReconcileSuccess())).
			Assess("ReleasesAreReady", funcs.AllOf(
				funcs.ResourceHasConditionWithin(1*time.Minute, release("cert-manager"), xpv2.Available()),
				funcs.ResourceHasConditionWithin(1*time.Minute, release("gpu-operator"), xpv2.Available()),
				funcs.ResourceHasConditionWithin(1*time.Minute, release("nvidia-dra-driver-gpu"), xpv2.Available()),
				funcs.ResourceHasConditionWithin(1*time.Minute, release("kai-scheduler"), xpv2.Available()),
				funcs.ResourceHasConditionWithin(1*time.Minute, release("kube-prometheus-stack"), xpv2.Available()),
			)).
			WithTeardown("DeleteXR", funcs.AllOf(
				funcs.DeleteResourcesWithPropagationPolicy(manifests, xr, metav1.DeletePropagationForeground),
				funcs.ResourcesDeletedWithin(10*time.Minute, manifests, xr),
			)).
			WithTeardown("DeletePrerequisites", funcs.AllOf(
				funcs.DeleteResourcesWithPropagationPolicy(manifests, "setup/*.yaml", metav1.DeletePropagationForeground),
				funcs.ResourcesDeletedWithin(3*time.Minute, manifests, "setup/*.yaml"),
			)).
			Feature(),
	)
}
