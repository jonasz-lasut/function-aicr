// Package main implements a Composition Function.
package main

import (
	"context"
	"io"
	"runtime/debug"
	"strings"
	"time"

	"github.com/alecthomas/kong"

	"github.com/NVIDIA/aicr/pkg/oci"
	"github.com/NVIDIA/aicr/pkg/recipe"
	"github.com/NVIDIA/aicr/pkg/recipe/ocisource"

	"github.com/crossplane/function-sdk-go"
	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	"github.com/crossplane/function-sdk-go/response"
)

// CLI of this Function.
type CLI struct {
	Debug bool `short:"d" help:"Emit debug logs in addition to info logs."`

	Network            string         `help:"Network on which to listen for gRPC connections." default:"tcp"`
	Address            string         `help:"Address at which to listen for gRPC connections." default:":9443"`
	TLSCertsDir        string         `help:"Directory containing server certs (tls.key, tls.crt) and the CA used to verify client certificates (ca.crt)" env:"TLS_SERVER_CERTS_DIR"`
	Insecure           bool           `help:"Run without mTLS credentials. If you supply this flag --tls-server-certs-dir will be ignored."`
	MaxRecvMessageSize int            `help:"Maximum size of received messages in MB." default:"4"`
	TTL                *time.Duration `help:"Time to live for function response."`

	RecipeOCIRepository string `name:"recipe-oci-repository" help:"OCI repository of a recipe artifact layered over the embedded catalog, e.g. ghcr.io/acme/aicr-recipes. Requires --recipe-oci-digest." env:"RECIPE_OCI_REPOSITORY"`
	RecipeOCIDigest     string `name:"recipe-oci-digest" help:"Immutable sha256 manifest digest selecting the recipe artifact, e.g. sha256:8ac7…. Requires --recipe-oci-repository." env:"RECIPE_OCI_DIGEST"`
}

// Run this Function.
func (c *CLI) Run() error {
	log, err := function.NewLogger(c.Debug)
	if err != nil {
		return err
	}
	ttl := response.DefaultTTL
	if c.TTL != nil {
		ttl = *c.TTL
	}

	dp, source, err := newDataProvider(context.Background(), log, c.RecipeOCIRepository, c.RecipeOCIDigest)
	if err != nil {
		return err
	}
	if closer, ok := dp.(io.Closer); ok {
		defer closer.Close() //nolint:errcheck // The process is exiting; the workspace dies with it.
	}

	return function.Serve(&Function{log: log, ttl: ttl, dp: dp, recipeVersion: aicrVersion(), recipeSource: source},
		function.Listen(c.Network, c.Address),
		function.MTLSCertificates(c.TLSCertsDir),
		function.Insecure(c.Insecure),
		function.MaxRecvMessageSize(c.MaxRecvMessageSize*1024*1024))
}

// newDataProvider returns the recipe data this function serves: the catalog
// embedded in the github.com/NVIDIA/aicr module or — when an OCI source is
// configured — that catalog with the digest-pinned recipe artifact layered
// over it, pulled once at startup so no reconcile ever waits on a registry.
// A pull failure is returned rather than fallen back from: silently serving
// embedded data would change what the function deploys. The returned source
// is the overlay's repository@digest identity, empty for embedded-only data.
func newDataProvider(ctx context.Context, log logging.Logger, repository, digest string) (recipe.DataProvider, string, error) {
	embedded := embeddedDataProvider()
	if repository == "" && digest == "" {
		return embedded, "", nil
	}
	opts, source, err := ociPullOptions(repository, digest)
	if err != nil {
		return nil, "", err
	}
	log.Info("Pulling the OCI recipe source", "source", source)
	dp, err := ocisource.New(ctx, embedded, ocisource.Config{PullOptions: opts})
	if err != nil {
		return nil, "", errors.Wrapf(err, "cannot pull the OCI recipe source %s", source)
	}
	log.Info("Serving recipes from the OCI source layered over the embedded catalog", "source", source)
	return dp, source, nil
}

// ociPullOptions validates the OCI recipe source flags. Both flags must be
// set together, and the selector must be an immutable sha256 manifest digest:
// a tag would let mutable registry state change what the function deploys
// (and AICR refuses to materialize tag-selected artifacts anyway). The
// returned source is the canonical repository@digest identity.
func ociPullOptions(repository, digest string) (oci.RecipePullOptions, string, error) {
	if repository == "" || digest == "" {
		return oci.RecipePullOptions{}, "", errors.New("--recipe-oci-repository and --recipe-oci-digest must be set together")
	}
	opts := oci.RecipePullOptions{Repository: repository, Selector: digest}
	isDigest, err := oci.ValidateRecipePullOptions(opts)
	if err != nil {
		return oci.RecipePullOptions{}, "", errors.Wrap(err, "invalid OCI recipe source")
	}
	if !isDigest {
		return oci.RecipePullOptions{}, "", errors.Errorf("--recipe-oci-digest must be an immutable sha256 manifest digest, got %q", digest)
	}
	return opts, strings.TrimPrefix(repository, "oci://") + "@" + digest, nil
}

// embeddedDataProvider returns the AICR recipe data embedded in this
// function's image, pinned by the github.com/NVIDIA/aicr module version.
func embeddedDataProvider() *recipe.EmbeddedDataProvider {
	return recipe.NewEmbeddedDataProvider(recipe.GetEmbeddedFS(), ".")
}

// aicrModule is the module whose version pins the embedded recipe data.
const aicrModule = "github.com/NVIDIA/aicr"

// aicrVersion returns the github.com/NVIDIA/aicr module version this binary
// was built with, read from the build info Go embeds in it. It is what the
// resolved-recipe summary reports as recipeVersion.
func aicrVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return moduleVersion(bi, aicrModule)
}

// moduleVersion returns the version of the named module in bi, honoring a
// replace directive, or "unknown" when the module is not among the
// dependencies — as in test binaries, which carry no module list.
func moduleVersion(bi *debug.BuildInfo, path string) string {
	for _, dep := range bi.Deps {
		if dep.Path != path {
			continue
		}
		if dep.Replace != nil && dep.Replace.Version != "" {
			return dep.Replace.Version
		}
		return dep.Version
	}
	return "unknown"
}

func main() {
	ctx := kong.Parse(&CLI{}, kong.Description("A Crossplane composition function that resolves an NVIDIA AICR recipe into provider-helm Releases and provider-kubernetes Objects."))
	ctx.FatalIfErrorf(ctx.Run())
}
