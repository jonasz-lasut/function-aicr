# Recipe source

By default the function serves the recipe catalog embedded in the `aicr.run`
module, AICR's Go module, from the function's build. The module version is the
catalog version, and nothing is fetched at runtime.

The `--recipe-oci-*` flags overlay that catalog with a digest-pinned OCI
recipe artifact, pulled once at startup so that a reconcile never waits on a
registry:

```
--recipe-oci-repository=ghcr.io/acme/aicr-recipes
--recipe-oci-digest=sha256:8ac7f6d54e8bcbf074f156a11f2c8c1ca6dcafed85e880e99a3126c0810cd66f
```

The environment variables `RECIPE_OCI_REPOSITORY` and `RECIPE_OCI_DIGEST` do
the same. Set either form through a
[DeploymentRuntimeConfig][deployment-runtime-config] that the Function
package references:

```yaml
apiVersion: pkg.crossplane.io/v1beta1
kind: DeploymentRuntimeConfig
metadata:
  name: function-aicr
spec:
  deploymentTemplate:
    spec:
      selector: {}
      template:
        spec:
          containers:
          - name: package-runtime
            args:
            - --recipe-oci-repository=ghcr.io/acme/aicr-recipes
            - --recipe-oci-digest=sha256:8ac7f6d54e8bcbf074f156a11f2c8c1ca6dcafed85e880e99a3126c0810cd66f
```

The source belongs to the function deployment, not to the `Input`: every
composite resource that function serves resolves against the same catalog.

## The rules

**Digest-pinned only.** The selector must be the artifact's immutable
`sha256:` manifest digest. A tag is refused, because mutable registry state
could then change what the function deploys. Rolling the catalog forward is a new
digest, and rolling back is the old one.

**Layered over the embedded catalog.** The artifact's files are layered over
the embedded catalog, and `registry.yaml` is merged. The artifact only needs
what differs. The [status summary](../authors/summary.md) reports the source
as `recipeSource` next to the embedded `recipeVersion`.

**Fail fast.** A pull or validation failure exits the function.
It never falls back to the embedded catalog, which would change what gets
deployed. The pod's restart backoff and its logs make the failure visible.

**Authored for the embedded AICR release.** The artifact's `registry.yaml`
must declare the `apiVersion` that release expects. AICR v1.0.0 retired the
alpha versions, so an artifact written for an older release is refused at
startup.

**Registry access.** Credentials are read from the Docker config
(`$DOCKER_CONFIG/config.json`): mount a `dockerconfigjson` Secret and set
`DOCKER_CONFIG`. Connections are HTTPS with the image's CA bundle. Set
`SSL_CERT_FILE` for a private CA. Extraction needs a writable temporary
directory (`TMPDIR`), so mount an `emptyDir` when the container filesystem is
read-only.

[deployment-runtime-config]: https://docs.crossplane.io/latest/packages/providers/#runtime-configuration
