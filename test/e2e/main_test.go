package e2e

import (
	"context"
	"os"
	"testing"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/e2e-framework/klient/conf"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/crossplane/crossplane/v2/test/e2e/config"
	"github.com/crossplane/crossplane/v2/test/e2e/funcs"
)

// The namespace Crossplane, the providers and the functions run in.
const namespace = "crossplane-system"

var environment = config.NewEnvironmentFromFlags()

func TestMain(m *testing.M) {
	// TODO(negz): Global loggers are dumb and klog is dumb. Remove this when
	// e2e-framework is running controller-runtime v0.15.x per
	// https://github.com/kubernetes-sigs/e2e-framework/issues/270
	log.SetLogger(klog.NewKlogr())

	cfg, err := envconf.NewFromFlags()
	if err != nil {
		panic(err)
	}

	// The suite has one test suite, Crossplane's default, without its Helm
	// install: `make e2e.up` installed Crossplane already.
	environment.AddDefaultTestSuite(
		config.WithoutBaseDefaultTestSuite(),
		config.WithLabelsToSelect(features.Labels{
			config.LabelTestSuite: []string{config.TestSuiteDefault},
		}),
	)
	environment.AddTestSuite(TestSuiteKWOK,
		config.WithoutBaseDefaultTestSuite(),
		config.WithLabelsToSelect(features.Labels{
			config.LabelTestSuite: []string{TestSuiteKWOK},
		}),
	)

	var (
		setup  []env.Func
		finish []env.Func
	)

	// The cluster is `make e2e.up`'s, which the suite neither creates nor
	// destroys. It is found through KUBECONFIG, which the Makefile points at
	// a file that holds that cluster alone.
	cfg.WithKubeconfigFile(conf.ResolveKubeConfigFile())

	// Enrich the selected labels with the ones from the suite.
	// Not replacing the user provided ones if any.
	cfg.WithLabels(environment.EnrichLabels(cfg.Labels()))

	environment.SetEnvironment(env.NewWithConfig(cfg))

	// Add default setup and finish funcs
	setup = append(setup, funcs.AddCrossplaneTypesToScheme(), funcs.AddCRDsToScheme())

	if environment.ShouldCollectKindLogsOnFailure() {
		finish = append(finish, envfuncs.ExportClusterLogs(environment.GetKindClusterName(), environment.GetKindClusterLogsLocation()))
	}

	// Check that all features have the LabelTestSuite label.
	environment.BeforeEachFeature(func(ctx context.Context, _ *envconf.Config, t *testing.T, feature features.Feature) (context.Context, error) {
		t.Helper()

		if _, exists := feature.Labels()[config.LabelTestSuite]; !exists {
			t.Fatalf("Feature %q does not have the required %q label set", feature.Name(), config.LabelTestSuite)
		}

		return ctx, nil
	})

	environment.Setup(setup...)
	environment.Finish(finish...)
	os.Exit(environment.Run(m))
}
