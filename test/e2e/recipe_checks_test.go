package e2e

import (
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/pkg/features"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/crossplane/crossplane/v2/test/e2e/config"
	"github.com/crossplane/crossplane/v2/test/e2e/funcs"
)

// TestBundleChecks uses the check the suite's catalog declares for
// e2e-checked, a component only the inference recipe has: AICR refuses it
// without a node selector for the accelerated workloads.
func TestBundleChecks(t *testing.T) {
	const (
		xr               = "checks/xr.yaml"
		placed           = "checks/xr-placed.yaml"
		acceleratedLabel = "e2e.aicr.example.org/accelerated-pool"
		checked          = "e2e-checked"
	)

	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests that the function runs the bundle checks AICR's registry declares: a recipe AICR would refuse to bundle is not deployed, it is deployed once the composite resource states what the check asks for, and a stack that is already deployed is not refused when it stops stating it.").
			WithLabel(LabelArea, LabelAreaRecipe).
			WithLabel(LabelSize, LabelSizeLarge).
			WithLabel(config.LabelTestSuite, config.TestSuiteDefault).
			WithSetup("CreatePrerequisites", createPrerequisites()).
			WithSetup("NodesAreInTheAcceleratedPool", LabelNodes(acceleratedLabel, "")).
			Assess("CreateXR", createXR(xr)).
			Assess("FirstInstallIsRefused", funcs.AllOf(
				funcs.ResourceValidatedWithin(2*time.Minute, stack("checks"), func(o k8s.Object) bool {
					return conditionMessageContains(o, "Synced", "False", `AICR's bundle check for "e2e-checked"`)
				}),
				ResourceIsAbsent(release(componentFirst)),
				ResourceIsAbsent(release(checked)),
			)).
			Assess("PlacementIsStated", funcs.AllOf(
				funcs.ApplyResources(FieldManager, recipeManifests, placed),
				funcs.ResourcesHaveConditionWithin(3*time.Minute, recipeManifests, placed, xpv2.Available(), xpv2.ReconcileSuccess()),
				funcs.ResourceHasFieldValueWithin(30*time.Second, deployment(checked, checked), "spec.template.spec.nodeSelector", map[string]any{acceleratedLabel: ""}),
			)).
			Assess("DeployedStackIsNotRefused", funcs.AllOf(
				funcs.ApplyResources(FieldManager, recipeManifests, xr),
				// The selector leaves the Release, so the function composed
				// the request that it refused when nothing was deployed.
				funcs.ResourceHasFieldValueWithin(2*time.Minute, release(checked), "spec.forProvider.values.nodeSelector", funcs.NotFound),
				funcs.ResourceHasFieldValueWithin(2*time.Minute, deployment(checked, checked), "spec.template.spec.nodeSelector", funcs.NotFound),
				funcs.DeploymentBecomesAvailableWithin(2*time.Minute, checked, checked),
				// provider-helm reports the upgraded Release ready again at
				// its next poll, which the suite's cluster sets to 30 seconds.
				funcs.ResourcesHaveConditionWithin(3*time.Minute, recipeManifests, xr, xpv2.Available(), xpv2.ReconcileSuccess()),
			)).
			WithTeardown("DeleteXR", deleteXR(xr)).
			WithTeardown("UnlabelNodes", UnlabelNodes(acceleratedLabel)).
			WithTeardown("DeletePrerequisites", deletePrerequisites()).
			Feature(),
	)
}

// stack returns a composite resource of the suite's XRD.
func stack(name string) *unstructured.Unstructured {
	return composed(schema.GroupVersionKind{Group: "e2e.aicr.example.org", Version: "v1alpha1", Kind: "Stack"}, name)
}

// conditionMessageContains reports whether a resource has the condition with
// the status and with the text in its message.
func conditionMessageContains(o k8s.Object, conditionType, status, text string) bool {
	u, ok := o.(*unstructured.Unstructured)
	if !ok {
		return false
	}

	conditions, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, entry := range conditions {
		condition, ok := entry.(map[string]any)
		if !ok || condition["type"] != conditionType || condition["status"] != status {
			continue
		}

		message, _ := condition["message"].(string)

		return strings.Contains(message, text)
	}

	return false
}
