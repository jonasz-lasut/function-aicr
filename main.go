// Package main implements a Composition Function.
package main

import (
	"runtime/debug"
	"time"

	"github.com/alecthomas/kong"

	"github.com/crossplane/function-sdk-go"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/NVIDIA/aicr/pkg/recipe"
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

	return function.Serve(&Function{log: log, ttl: ttl, dp: newDataProvider(), recipeVersion: aicrVersion()},
		function.Listen(c.Network, c.Address),
		function.MTLSCertificates(c.TLSCertsDir),
		function.Insecure(c.Insecure),
		function.MaxRecvMessageSize(c.MaxRecvMessageSize*1024*1024))
}

// newDataProvider returns the AICR recipe data embedded in this function's
// image. Recipe data version is pinned by the github.com/NVIDIA/aicr module
// version. An OCI source can be added when AICR's OCISource lands upstream.
func newDataProvider() recipe.DataProvider {
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
