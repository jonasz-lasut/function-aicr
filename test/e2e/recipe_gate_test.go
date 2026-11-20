package e2e

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/features"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/crossplane/crossplane/v2/test/e2e/config"
	"github.com/crossplane/crossplane/v2/test/e2e/funcs"
)

// The Objects function-aicr composes for the manifest files of the suite's
// catalog.
const (
	objectSecondBefore = "e2e-second-pre-configmap-e2e-second-before"
	objectSecondAfter  = "e2e-second-post-configmap-e2e-second-after"
	objectNotes        = "e2e-notes-post-configmap-e2e-notes"
)

// TestRolloutSteps holds e2e-second at each of its own steps in turn. A quota
// in its namespace keeps the ConfigMap of its pre-manifest file from being
// created, and a node selector no node matches keeps its Release short of
// ready.
func TestRolloutSteps(t *testing.T) {
	const (
		xr          = "steps/xr.yaml"
		secondLabel = "e2e.aicr.example.org/second"
	)

	quota := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Namespace: componentSecond, Name: "e2e-no-configmaps"}}

	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests the order within one component: its Release is withheld until the Objects of its pre-manifest files are ready, and the Objects of its manifest files, like the manifest-only component that depends on it, are withheld until the Release is ready.").
			WithLabel(LabelArea, LabelAreaRecipe).
			WithLabel(LabelSize, LabelSizeLarge).
			WithLabel(config.LabelTestSuite, config.TestSuiteDefault).
			WithSetup("CreatePrerequisites", createPrerequisites()).
			WithSetup("NoNodeMatchesTheSelector", UnlabelNodes(secondLabel)).
			WithSetup("PreManifestCannotBeApplied", funcs.AllOf(
				funcs.ApplyResources(FieldManager, recipeManifests, "steps/namespace.yaml"),
				funcs.ApplyResources(FieldManager, recipeManifests, "steps/quota.yaml"),
				funcs.ResourcesCreatedWithin(30*time.Second, recipeManifests, "steps/quota.yaml"),
			)).
			Assess("CreateXR", createXR(xr)).
			Assess("ReleaseWaitsForItsPreManifest", funcs.AllOf(
				funcs.ResourceHasConditionWithin(3*time.Minute, release(componentFirst), xpv2.Available()),
				funcs.ResourcesHaveFieldValueWithin(2*time.Minute, recipeManifests, xr, "status.aicr.pendingComponents[0].waitingFor", objectSecondBefore),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.pendingComponents[0].name", componentSecond),
				funcs.ResourceCreatedWithin(30*time.Second, object(objectSecondBefore)),
				ResourceIsAbsent(release(componentSecond)),
			)).
			Assess("PreManifestIsApplied", funcs.AllOf(
				DeleteIfExists(quota),
				funcs.ResourceHasConditionWithin(3*time.Minute, object(objectSecondBefore), xpv2.Available()),
				funcs.ResourceCreatedWithin(2*time.Minute, release(componentSecond)),
			)).
			Assess("PostManifestWaitsForTheRelease", funcs.AllOf(
				funcs.ResourcesHaveFieldValueWithin(2*time.Minute, recipeManifests, xr, "status.aicr.pendingComponents[0].name", "e2e-notes"),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.pendingComponents[0].waitingFor", componentSecond),
				ResourceIsNotReady(release(componentSecond)),
				ResourceIsAbsent(object(objectSecondAfter)),
				ResourceIsAbsent(object(objectNotes)),
			)).
			Assess("ReleaseBecomesReady", funcs.AllOf(
				LabelNodes(secondLabel, gateValue),
				funcs.ResourceHasConditionWithin(3*time.Minute, release(componentSecond), xpv2.Available()),
			)).
			Assess("PostManifestFollows", funcs.AllOf(
				funcs.ResourcesHaveConditionWithin(3*time.Minute, recipeManifests, xr, xpv2.Available(), xpv2.ReconcileSuccess()),
				funcs.ResourceHasConditionWithin(1*time.Minute, object(objectSecondAfter), xpv2.Available()),
				funcs.ResourceHasConditionWithin(1*time.Minute, object(objectNotes), xpv2.Available()),
			)).
			WithTeardown("DeleteXR", deleteXR(xr)).
			WithTeardown("DeleteQuota", DeleteIfExists(quota)).
			WithTeardown("DeleteNamespace", DeleteNamespaces(3*time.Minute, componentSecond)).
			WithTeardown("UnlabelNodes", UnlabelNodes(secondLabel)).
			WithTeardown("DeletePrerequisites", deletePrerequisites()).
			Feature(),
	)
}

// TestLiveResourcesAreKept makes a dependency report unready after everything
// is deployed. provider-helm derives a Release's readiness from the state of
// the Helm release, which a pod that stops running does not change, so the
// feature takes the Release out of the provider's hands for a moment: it
// pauses its reconciliation and sets its Ready condition to False itself.
func TestLiveResourcesAreKept(t *testing.T) {
	const xr = "live/xr.yaml"

	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests that the rollout order only orders creation: when a deployed component goes unready, what depends on it is neither withheld nor removed from the cluster.").
			WithLabel(LabelArea, LabelAreaRecipe).
			WithLabel(LabelSize, LabelSizeLarge).
			WithLabel(config.LabelTestSuite, config.TestSuiteDefault).
			WithSetup("CreatePrerequisites", createPrerequisites()).
			Assess("CreateXR", createXR(xr)).
			Assess("XRIsReady",
				funcs.ResourcesHaveConditionWithin(3*time.Minute, recipeManifests, xr, xpv2.Available(), xpv2.ReconcileSuccess())).
			Assess("DependencyGoesUnready", funcs.AllOf(
				PauseReconciliation(release(componentFirst)),
				funcs.ResourceHasConditionWithin(1*time.Minute, release(componentFirst), xpv2.ReconcilePaused()),
				MarkUnready(release(componentFirst)),
				funcs.ResourcesHaveConditionWithin(2*time.Minute, recipeManifests, xr, xpv2.Creating()),
			)).
			Assess("DependentsStay", funcs.AllOf(
				funcs.SleepFor(20*time.Second),
				ResourceIsNotReady(release(componentFirst)),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.pendingComponents", []any{}),
				funcs.ResourceHasConditionWithin(30*time.Second, release(componentSecond), xpv2.Available()),
				funcs.ResourceHasConditionWithin(30*time.Second, object(objectSecondBefore), xpv2.Available()),
				funcs.ResourceHasConditionWithin(30*time.Second, object(objectSecondAfter), xpv2.Available()),
				funcs.ResourceHasConditionWithin(30*time.Second, object(objectNotes), xpv2.Available()),
				funcs.DeploymentBecomesAvailableWithin(30*time.Second, componentSecond, componentSecond),
			)).
			Assess("DependencyRecovers", funcs.AllOf(
				ResumeReconciliation(release(componentFirst)),
				funcs.ResourceHasConditionWithin(2*time.Minute, release(componentFirst), xpv2.Available()),
				funcs.ResourcesHaveConditionWithin(2*time.Minute, recipeManifests, xr, xpv2.Available(), xpv2.ReconcileSuccess()),
			)).
			WithTeardown("ResumeReconciliation", ResumeReconciliation(release(componentFirst))).
			WithTeardown("DeleteXR", deleteXR(xr)).
			WithTeardown("DeletePrerequisites", deletePrerequisites()).
			Feature(),
	)
}

// TestSkipDeploymentOrder keeps e2e-first short of ready, as TestRolloutOrder
// does, on a composite resource that turns the rollout order off.
func TestSkipDeploymentOrder(t *testing.T) {
	const xr = "unordered/xr.yaml"

	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests that skipDeploymentOrder composes every component at once: what depends on a component that is not ready yet is deployed anyway, and the summary reports nothing as pending.").
			WithLabel(LabelArea, LabelAreaRecipe).
			WithLabel(LabelSize, LabelSizeLarge).
			WithLabel(config.LabelTestSuite, config.TestSuiteDefault).
			WithSetup("CreatePrerequisites", createPrerequisites()).
			WithSetup("NoNodeMatchesTheSelector", UnlabelNodes(gateLabel)).
			Assess("CreateXR", createXR(xr)).
			Assess("EverythingIsComposedAtOnce", funcs.AllOf(
				funcs.ResourceCreatedWithin(2*time.Minute, release(componentSecond)),
				funcs.ResourceCreatedWithin(1*time.Minute, object(objectNotes)),
				funcs.ResourcesHaveFieldValueWithin(1*time.Minute, recipeManifests, xr, "status.aicr.pendingComponents", []any{}),
				funcs.ResourceHasConditionWithin(3*time.Minute, release(componentSecond), xpv2.Available()),
				ResourceIsNotReady(release(componentFirst)),
			)).
			Assess("FirstComponentBecomesReady", funcs.AllOf(
				LabelNodes(gateLabel, gateValue),
				funcs.ResourcesHaveConditionWithin(3*time.Minute, recipeManifests, xr, xpv2.Available(), xpv2.ReconcileSuccess()),
			)).
			WithTeardown("DeleteXR", deleteXR(xr)).
			WithTeardown("UnlabelNodes", UnlabelNodes(gateLabel)).
			WithTeardown("DeletePrerequisites", deletePrerequisites()).
			Feature(),
	)
}

// TestSkippedDependency leaves e2e-first out of the recipe.
func TestSkippedDependency(t *testing.T) {
	const xr = "skipped/xr.yaml"

	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests that a dependency the composite resource skips counts as satisfied: what depends on it is deployed without it.").
			WithLabel(LabelArea, LabelAreaRecipe).
			WithLabel(LabelSize, LabelSizeLarge).
			WithLabel(config.LabelTestSuite, config.TestSuiteDefault).
			WithSetup("CreatePrerequisites", createPrerequisites()).
			Assess("CreateXR", createXR(xr)).
			Assess("DependentIsDeployedWithoutIt", funcs.AllOf(
				funcs.ResourcesHaveConditionWithin(3*time.Minute, recipeManifests, xr, xpv2.Available(), xpv2.ReconcileSuccess()),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.componentCount", int64(2)),
				funcs.ResourcesHaveFieldValueWithin(30*time.Second, recipeManifests, xr, "status.aicr.deployedComponents[0].name", componentSecond),
				ResourceIsAbsent(release(componentFirst)),
				funcs.DeploymentBecomesAvailableWithin(1*time.Minute, componentSecond, componentSecond),
			)).
			WithTeardown("DeleteXR", deleteXR(xr)).
			WithTeardown("DeletePrerequisites", deletePrerequisites()).
			Feature(),
	)
}
