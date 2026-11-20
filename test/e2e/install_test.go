package e2e

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/features"

	pkgv1 "github.com/crossplane/crossplane/apis/v2/pkg/v1"

	"github.com/crossplane/crossplane/v2/test/e2e/config"
	"github.com/crossplane/crossplane/v2/test/e2e/funcs"
)

// LabelAreaInstall is applied to all features pertaining to the packages the
// suite's cluster is provisioned with.
const LabelAreaInstall = "install"

// TestFunctionInstalled is the suite's smoke test: it proves the cluster
// `make e2e.up` provisioned is the one the other features assume.
func TestFunctionInstalled(t *testing.T) {
	environment.Test(t,
		features.NewWithDescription(t.Name(), "Tests that function-aicr, built from this tree and installed from the local package cache, becomes healthy next to the providers whose resources it composes and the function that follows it in a pipeline.").
			WithLabel(LabelArea, LabelAreaInstall).
			WithLabel(LabelSize, LabelSizeSmall).
			WithLabel(config.LabelTestSuite, config.TestSuiteDefault).
			Assess("FunctionAICRIsHealthy",
				funcs.ResourceHasConditionWithin(2*time.Minute, &pkgv1.Function{ObjectMeta: metav1.ObjectMeta{Name: "function-aicr"}}, pkgv1.Healthy(), pkgv1.Active()),
			).
			Assess("ProvidersAreHealthy", funcs.AllOf(
				funcs.ResourceHasConditionWithin(2*time.Minute, &pkgv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "provider-helm"}}, pkgv1.Healthy(), pkgv1.Active()),
				funcs.ResourceHasConditionWithin(2*time.Minute, &pkgv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "provider-kubernetes"}}, pkgv1.Healthy(), pkgv1.Active()),
			)).
			Assess("FunctionAutoReadyIsHealthy",
				funcs.ResourceHasConditionWithin(2*time.Minute, &pkgv1.Function{ObjectMeta: metav1.ObjectMeta{Name: "function-auto-ready"}}, pkgv1.Healthy(), pkgv1.Active()),
			).
			Feature(),
	)
}
