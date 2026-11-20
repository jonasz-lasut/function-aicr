# function-aicr

function-aicr is a [Crossplane composition function][functions] that deploys
an [NVIDIA AI Cluster Runtime (AICR)][aicr] recipe. AICR captures known-good
combinations of GPU drivers, operators and platform components as
version-locked recipes. Give the function the recipe criteria (accelerator,
service, intent, OS, platform) and it resolves the matching recipe from
AICR's catalog. It expands the recipe into namespaced
[provider-helm][provider-helm] `Release`s and
[provider-kubernetes][provider-kubernetes] `Object`s that target a workload
cluster, rolled out in AICR's dependency order.

The function embeds AICR's recipe engine and catalog, so there is nothing to
install besides the function package and the two providers whose resources it
composes. The [AICR documentation][aicr-docs] describes the recipes
themselves.

Each recipe component becomes one `Release` for its Helm chart and one
`Object` per document of its manifest files. The function checks each
composed resource against the provider CRD schema Crossplane resolves for it
before emitting it, and withholds it from the desired state until what AICR
orders before it reports `Ready`.

## Where to start

| You want to | Read |
|---|---|
| deploy a recipe to a cluster | [Install](authors/install.md), then [Write a Composition](authors/composition.md) |
| let each composite resource name its own recipe | [Values from the composite resource](authors/overlay.md) |
| look up a field | [Input reference](operators/input-reference.md) |
| serve your own recipe catalog | [Recipe source](operators/recipe-source.md) |
| understand a Warning or a Fatal result | [Troubleshooting](operators/troubleshooting.md) |
| change the function | [Development](project/development.md) |

[functions]: https://docs.crossplane.io/latest/packages/functions/
[aicr]: https://github.com/NVIDIA/aicr
[aicr-docs]: https://docs.nvidia.com/aicr
[provider-helm]: https://github.com/crossplane-contrib/provider-helm
[provider-kubernetes]: https://github.com/crossplane-contrib/provider-kubernetes
