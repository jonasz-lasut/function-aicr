# Opt in to a mixin

AICR's catalog has opt-in mixins that no stock recipe includes, such as the
graded NVSentinel remediation steps or `nvsentinel-observability`. A recipe
takes a mixin only when a catalog overlay lists it, and AICR has no criteria
value or flag for that. The [recipe source](recipe-source.md) is how a
function deployment opts in.

## The artifact

It needs two small files: a `registry.yaml` with nothing but its header, and
an overlay that extends the stock recipe.

```yaml
# registry.yaml
apiVersion: aicr.run/v1beta1
kind: ComponentRegistry
components: []
```

```yaml
# overlays/h100-kind-training-observability.yaml
kind: RecipeMetadata
apiVersion: aicr.run/v1beta1
metadata:
  name: h100-kind-training-observability
spec:
  base: h100-kind-training
  criteria:
    service: kind
    accelerator: h100
    intent: training
  mixins:
  - nvsentinel-observability
```

The overlay sets the stock recipe as its `base` and repeats its criteria, so
every composite resource that resolves that recipe gets the mixin.

## Publish it

Publish the two files as one gzipped tar layer with AICR's artifact type:

```shell
tar -czf recipes.tar.gz registry.yaml overlays
oras push --artifact-type application/vnd.nvidia.aicr.artifact \
  ghcr.io/acme/aicr-recipes:observability \
  recipes.tar.gz:application/vnd.oci.image.layer.v1.tar+gzip
```

`oras` prints the manifest digest to pass as `--recipe-oci-digest`. Build the
tarball yourself, as shown. AICR refuses the layer `oras` packs from a
directory, because its archive has a `.` entry.

## Check it

With the function running on that artifact, the
[status summary](../authors/summary.md) lists the overlay under
`appliedOverlays`, and the `nvsentinel` `Release` has the mixin's values.
