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

// LabelAreaRecipe is applied to all features that deploy a recipe of the
// suite's catalog (test/e2e/catalog) through a composite resource.
const LabelAreaRecipe = "recipe"

// The suite's catalog has three components. e2e-first is a chart without
// dependencies. e2e-second is a chart that depends on it and has a manifest
// file on each side of its Release. e2e-notes is manifest-only and depends on
// e2e-second. The features share one XRD and Composition and each creates a
// composite resource of its own, one at a time: the Helm releases of two
// composite resources with the same recipe would collide in the cluster.
const (
	recipeManifests = "manifests/recipe"

	componentFirst  = "e2e-first"
	componentSecond = "e2e-second"

	// The label e2e-first's pod selects a node by in TestRolloutOrder.
	gateLabel = "e2e.aicr.example.org/first"
	gateValue = "ready"
)

// TestRecipeConverges tests the whole path once: function-aicr resolves the
// recipe from the OCI recipe source, Crossplane resolves the provider
// schemas it requires, and provider-helm and provider-kubernetes accept what
// it composes and bring it up.
func TestRecipeConverges(t *testing.T) {
	const xr = "converge/xr.yaml"

	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests that a composite resource deploys the recipe its criteria select: the charts are installed from the registry, the manifest files are applied around them, and the summary on the composite resource describes what was deployed.").
			WithLabel(LabelArea, LabelAreaRecipe).
			WithLabel(LabelSize, LabelSizeLarge).
			WithLabel(config.LabelTestSuite, config.TestSuiteDefault).
			WithSetup("CreatePrerequisites", createPrerequisites()).
			Assess("CreateXR", createXR(xr)).
			Assess("XRIsReady",
				funcs.ResourcesHaveConditionWithin(3*time.Minute, recipeManifests, xr, xpv2.Available(), xpv2.ReconcileSuccess())).
			Assess("ChartsAreInstalled", funcs.AllOf(
				funcs.DeploymentBecomesAvailableWithin(1*time.Minute, componentFirst, componentFirst),
				funcs.DeploymentBecomesAvailableWithin(1*time.Minute, componentSecond, componentSecond),
				funcs.ResourceHasFieldValueWithin(30*time.Second, configMap(componentFirst, componentFirst), "data.message", "first"),
			)).
			Assess("ManifestFilesAreApplied", funcs.AllOf(
				funcs.ResourceHasFieldValueWithin(30*time.Second, configMap(componentSecond, "e2e-second-before"), "data.message", "second"),
				funcs.ResourceHasFieldValueWithin(30*time.Second, configMap(componentSecond, "e2e-second-after"), "data.chart", "e2e-app-0.1.0"),
				funcs.ResourceHasFieldValueWithin(30*time.Second, configMap(componentSecond, "e2e-notes"), "data.note", "composed without a chart"),
			)).
			Assess("SummaryDescribesTheRecipe", funcs.AllOf(
				funcs.ResourcesHaveFieldValueWithin(1*time.Minute, recipeManifests, xr, "status.aicr.recipeName", "kind"),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.recipeSource", funcs.Any),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.componentCount", int64(3)),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.deployedComponents[0].name", componentFirst),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.deployedComponents[0].version", "0.1.0"),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.deployedComponents[2].name", "e2e-notes"),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.pendingComponents", []any{}),
			)).
			WithTeardown("DeleteXR", deleteXR(xr)).
			WithTeardown("DeletePrerequisites", deletePrerequisites()).
			Feature(),
	)
}

// TestRolloutOrder holds e2e-first short of ready, which the suite's chart
// allows through its pod's node selector, and releases it by labelling the
// node.
func TestRolloutOrder(t *testing.T) {
	const xr = "order/xr.yaml"

	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests that a component is withheld until the component it depends on is ready: nothing of it is composed, the summary names what it waits for, and it follows once the dependency is ready.").
			WithLabel(LabelArea, LabelAreaRecipe).
			WithLabel(LabelSize, LabelSizeLarge).
			WithLabel(config.LabelTestSuite, config.TestSuiteDefault).
			WithSetup("CreatePrerequisites", createPrerequisites()).
			WithSetup("NoNodeMatchesTheSelector", UnlabelNodes(gateLabel)).
			Assess("CreateXR", createXR(xr)).
			Assess("DependentIsWithheld", funcs.AllOf(
				funcs.ResourcesHaveFieldValueWithin(2*time.Minute, recipeManifests, xr, "status.aicr.pendingComponents[0].name", componentSecond),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.pendingComponents[0].waitingFor", componentFirst),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.pendingComponents[1].name", "e2e-notes"),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.pendingComponents[1].waitingFor", componentSecond),
				ResourceIsAbsent(release(componentSecond)),
				ResourceIsAbsent(object("e2e-second-pre-configmap-e2e-second-before")),
			)).
			Assess("DependencyBecomesReady", funcs.AllOf(
				LabelNodes(gateLabel, gateValue),
				funcs.ResourceHasConditionWithin(3*time.Minute, release(componentFirst), xpv2.Available()),
			)).
			Assess("DependentFollows", funcs.AllOf(
				funcs.ResourcesHaveConditionWithin(3*time.Minute, recipeManifests, xr, xpv2.Available(), xpv2.ReconcileSuccess()),
				funcs.ResourcesHaveFieldValueWithin(1*time.Minute, recipeManifests, xr, "status.aicr.pendingComponents", []any{}),
				funcs.DeploymentBecomesAvailableWithin(1*time.Minute, componentSecond, componentSecond),
			)).
			WithTeardown("DeleteXR", deleteXR(xr)).
			WithTeardown("UnlabelNodes", UnlabelNodes(gateLabel)).
			WithTeardown("DeletePrerequisites", deletePrerequisites()).
			Feature(),
	)
}

// TestOverlay changes the composite resource after its recipe is deployed.
func TestOverlay(t *testing.T) {
	const (
		xr      = "overlay/xr.yaml"
		changed = "overlay/xr-changed.yaml"
	)

	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests that the overlay on the composite resource shapes what is deployed, and keeps doing so: a chart value reaches the installed release, a changed value upgrades it, and a component that is skipped later is removed from the cluster.").
			WithLabel(LabelArea, LabelAreaRecipe).
			WithLabel(LabelSize, LabelSizeLarge).
			WithLabel(config.LabelTestSuite, config.TestSuiteDefault).
			WithSetup("CreatePrerequisites", createPrerequisites()).
			Assess("CreateXR", createXR(xr)).
			Assess("XRIsReady",
				funcs.ResourcesHaveConditionWithin(3*time.Minute, recipeManifests, xr, xpv2.Available(), xpv2.ReconcileSuccess())).
			Assess("ReleaseHasTheValue",
				funcs.ResourceHasFieldValueWithin(1*time.Minute, configMap(componentFirst, componentFirst), "data.message", "from the composite resource")).
			Assess("ChangeXR", funcs.ApplyResources(FieldManager, recipeManifests, changed)).
			Assess("ReleaseIsUpgraded",
				funcs.ResourceHasFieldValueWithin(2*time.Minute, configMap(componentFirst, componentFirst), "data.message", "changed on the composite resource")).
			Assess("SkippedComponentIsRemoved", funcs.AllOf(
				funcs.ResourcesHaveFieldValueWithin(2*time.Minute, recipeManifests, changed, "status.aicr.componentCount", int64(2)),
				funcs.ResourceDeletedWithin(2*time.Minute, object("e2e-notes-post-configmap-e2e-notes")),
				funcs.ResourceDeletedWithin(1*time.Minute, configMap(componentSecond, "e2e-notes")),
			)).
			WithTeardown("DeleteXR", deleteXR(changed)).
			WithTeardown("DeletePrerequisites", deletePrerequisites()).
			Feature(),
	)
}

// TestKeptNamespace moves the composite resource to a recipe that places
// e2e-first in another namespace, which is what an AICR upgrade that moves a
// component does to every composite resource at once.
func TestKeptNamespace(t *testing.T) {
	const (
		xr            = "namespace/xr.yaml"
		recipeMoves   = "namespace/xr-recipe-moves.yaml"
		overrideMoves = "namespace/xr-override-moves.yaml"
		moved         = "e2e-first-moved"
	)

	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests that a deployed component keeps its namespace when the recipe starts to place it in another, and that an explicit override moves it: provider-helm then installs the chart in the new namespace and leaves the release in the old one behind.").
			WithLabel(LabelArea, LabelAreaRecipe).
			WithLabel(LabelSize, LabelSizeLarge).
			WithLabel(config.LabelTestSuite, config.TestSuiteDefault).
			WithSetup("CreatePrerequisites", createPrerequisites()).
			Assess("CreateXR", createXR(xr)).
			Assess("ComponentIsDeployed", funcs.AllOf(
				funcs.ResourcesHaveConditionWithin(3*time.Minute, recipeManifests, xr, xpv2.Available(), xpv2.ReconcileSuccess()),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.deployedComponents[0].namespace", componentFirst),
				funcs.DeploymentBecomesAvailableWithin(1*time.Minute, componentFirst, componentFirst),
			)).
			Assess("RecipeMovesTheComponent", funcs.AllOf(
				funcs.ApplyResources(FieldManager, recipeManifests, recipeMoves),
				funcs.ResourcesHaveFieldValueWithin(2*time.Minute, recipeManifests, recipeMoves, "status.aicr.recipeName", "kind-training"),
			)).
			Assess("ComponentKeepsItsNamespace", funcs.AllOf(
				funcs.ResourceHasFieldValueWithin(30*time.Second, release(componentFirst), "spec.forProvider.namespace", componentFirst),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, recipeMoves, "status.aicr.deployedComponents[0].namespace", componentFirst),
				ResourceIsAbsent(deployment(moved, componentFirst)),
			)).
			Assess("OverrideMovesTheComponent", funcs.AllOf(
				funcs.ApplyResources(FieldManager, recipeManifests, overrideMoves),
				funcs.ResourceHasFieldValueWithin(2*time.Minute, release(componentFirst), "spec.forProvider.namespace", moved),
				funcs.ResourceCreatedWithin(3*time.Minute, deployment(moved, componentFirst)),
				funcs.DeploymentBecomesAvailableWithin(1*time.Minute, moved, componentFirst),
				funcs.ResourcesHaveFieldValueWithin(2*time.Minute, recipeManifests, overrideMoves, "status.aicr.deployedComponents[0].namespace", moved),
			)).
			Assess("OldReleaseIsLeftBehind",
				funcs.DeploymentBecomesAvailableWithin(30*time.Second, componentFirst, componentFirst)).
			WithTeardown("DeleteXR", deleteXR(overrideMoves)).
			WithTeardown("DeleteTheReleaseLeftBehind", DeleteNamespaces(3*time.Minute, componentFirst)).
			WithTeardown("DeletePrerequisites", deletePrerequisites()).
			Feature(),
	)
}

// TestNodeScheduling places the two charts of the suite's catalog, whose
// registry entries list the chart's node selector and tolerations for system
// workloads (e2e-first) and for workloads on accelerated nodes (e2e-second).
func TestNodeScheduling(t *testing.T) {
	const (
		xr               = "scheduling/xr.yaml"
		systemLabel      = "e2e.aicr.example.org/system-pool"
		acceleratedLabel = "e2e.aicr.example.org/accelerated-pool"
	)

	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests that nodeScheduling reaches the pods: each chart gets the node selector and the tolerations of the kind of workload AICR's registry lists its values for, and runs once a node matches.").
			WithLabel(LabelArea, LabelAreaRecipe).
			WithLabel(LabelSize, LabelSizeLarge).
			WithLabel(config.LabelTestSuite, config.TestSuiteDefault).
			WithSetup("CreatePrerequisites", createPrerequisites()).
			WithSetup("NoNodeMatchesTheSelectors", funcs.AllOf(UnlabelNodes(systemLabel), UnlabelNodes(acceleratedLabel))).
			Assess("CreateXR", createXR(xr)).
			Assess("SystemWorkloadIsPlaced", funcs.AllOf(
				funcs.ResourceCreatedWithin(2*time.Minute, deployment(componentFirst, componentFirst)),
				funcs.ResourceHasFieldValueWithin(30*time.Second, deployment(componentFirst, componentFirst), "spec.template.spec.nodeSelector", map[string]any{systemLabel: ""}),
				funcs.ResourceHasFieldValueWithin(30*time.Second, deployment(componentFirst, componentFirst), "spec.template.spec.tolerations[0].key", "e2e.aicr.example.org/dedicated"),
				funcs.ResourceHasFieldValueWithin(30*time.Second, deployment(componentFirst, componentFirst), "spec.template.spec.tolerations[0].value", "system"),
			)).
			Assess("NodesJoinThePools", funcs.AllOf(
				LabelNodes(systemLabel, ""),
				LabelNodes(acceleratedLabel, ""),
			)).
			Assess("AcceleratedWorkloadIsPlaced", funcs.AllOf(
				funcs.ResourcesHaveConditionWithin(3*time.Minute, recipeManifests, xr, xpv2.Available(), xpv2.ReconcileSuccess()),
				funcs.ResourceHasFieldValueWithin(30*time.Second, deployment(componentSecond, componentSecond), "spec.template.spec.nodeSelector", map[string]any{acceleratedLabel: ""}),
				funcs.ResourceHasFieldValueWithin(30*time.Second, deployment(componentSecond, componentSecond), "spec.template.spec.tolerations[0].key", "e2e.aicr.example.org/accelerator"),
				funcs.DeploymentBecomesAvailableWithin(1*time.Minute, componentFirst, componentFirst),
				funcs.DeploymentBecomesAvailableWithin(1*time.Minute, componentSecond, componentSecond),
			)).
			WithTeardown("DeleteXR", deleteXR(xr)).
			WithTeardown("UnlabelNodes", funcs.AllOf(UnlabelNodes(systemLabel), UnlabelNodes(acceleratedLabel))).
			WithTeardown("DeletePrerequisites", deletePrerequisites()).
			Feature(),
	)
}

// createPrerequisites applies the XRD and the Composition the recipe features
// share.
func createPrerequisites() features.Func {
	return funcs.AllOf(
		funcs.ApplyResources(FieldManager, recipeManifests, "setup/*.yaml"),
		funcs.ResourcesCreatedWithin(30*time.Second, recipeManifests, "setup/*.yaml"),
		funcs.ResourcesHaveConditionWithin(1*time.Minute, recipeManifests, "setup/definition.yaml", apiextensionsv1.WatchingComposite()),
	)
}

// deletePrerequisites deletes what createPrerequisites applied.
func deletePrerequisites() features.Func {
	return funcs.AllOf(
		funcs.DeleteResourcesWithPropagationPolicy(recipeManifests, "setup/*.yaml", metav1.DeletePropagationForeground),
		funcs.ResourcesDeletedWithin(3*time.Minute, recipeManifests, "setup/*.yaml"),
	)
}

// createXR applies a feature's composite resource.
func createXR(file string) features.Func {
	return funcs.AllOf(
		funcs.ApplyResources(FieldManager, recipeManifests, file),
		funcs.ResourcesCreatedWithin(30*time.Second, recipeManifests, file),
	)
}

// deleteXR deletes a feature's composite resource and waits until what it
// composed is gone with it: the next feature deploys the same Helm releases.
func deleteXR(file string) features.Func {
	return funcs.AllOf(
		funcs.DeleteResourcesWithPropagationPolicy(recipeManifests, file, metav1.DeletePropagationForeground),
		funcs.ResourcesDeletedWithin(5*time.Minute, recipeManifests, file),
	)
}
