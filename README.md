# function-aicr

A [Crossplane composition function][functions] that deploys an
[NVIDIA AI Cluster Runtime (AICR)][aicr] recipe. Give it the recipe criteria
(accelerator, service, intent, OS, platform) and it resolves the matching
recipe from AICR's catalog. It expands the recipe into namespaced
[provider-helm][provider-helm] `Release`s and
[provider-kubernetes][provider-kubernetes] `Object`s that target a workload
cluster, rolled out in AICR's dependency order.

The documentation is at <https://jonasz-lasut.github.io/function-aicr/>.

## Overview

AICR captures known-good combinations of GPU drivers, operators and platform
components as version-locked recipes. The function embeds AICR's recipe
engine and catalog, so there is nothing to install besides the function
package and the providers whose resources it composes.

Each recipe component becomes one `Release` for its Helm chart and one
`Object` per document of its manifest files. The function checks each
composed resource against the provider CRD schema Crossplane resolves for it,
and withholds it from the desired state until what AICR orders before it
reports `Ready`.

## Quick start

Install the function on a control plane that runs Crossplane v2.2 or later,
with provider-helm and provider-kubernetes:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: function-aicr
spec:
  package: ghcr.io/jonasz-lasut/function-aicr:<version>
```

Use it as a step in a Composition pipeline. The `Input` states the recipe
criteria and the provider config of the target cluster. A value is either
static in the Composition or set by the composite resource, through the
object at `overlayFrom.fieldPath`:

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: clusterstacks.example.org
spec:
  compositeTypeRef:
    apiVersion: example.org/v1alpha1
    kind: ClusterStack
  mode: Pipeline
  pipeline:
  - step: aicr
    functionRef:
      name: function-aicr
    input:
      apiVersion: aicr.fn.crossplane.io/v1beta1
      kind: Input
      overlayFrom:
        fieldPath: spec.aicr
      providerConfigRef:
        kind: ClusterProviderConfig
        name: gpu-cluster
      target:
        fieldPath: status.aicr
  - step: auto-ready
    functionRef:
      name: function-auto-ready
```

A composite resource of that kind then picks its own recipe:

```yaml
apiVersion: example.org/v1alpha1
kind: ClusterStack
metadata:
  name: gpu-training-stack
  namespace: default
spec:
  aicr:
    criteria:
      accelerator: h100
      service: kind
      intent: training
```

The [`example/clusterstack`](example/clusterstack/) directory has the XRD,
the Composition and composite resources to run through `crossplane render`.

## Documentation

| Page | Covers |
|---|---|
| [Install][docs-install] | What the function needs and how to install the package. |
| [Write a Composition][docs-composition] | The pipeline step, the XRD and the composite resource. |
| [Values from the composite resource][docs-overlay] | `overlayFrom` and its merge rules. |
| [Recipe variants and configuration][docs-variants] | `profile` and `configuration`. |
| [Components and overrides][docs-components] | Which components are deployed, and how to skip or change one. |
| [Input reference][docs-input] | Every field of the `Input`. |
| [Recipe source][docs-source] | Serving your own recipe catalog from an OCI registry. |
| [Rollout order][docs-rollout] | How components are ordered on observed readiness. |
| [Upgrades][docs-upgrades] | What a new version changes for a running cluster. |
| [Troubleshooting][docs-troubleshooting] | The function's messages and their causes. |

## Development

```shell
go generate ./...   # the Input CRD and the golden fixtures
go test ./...
golangci-lint run
```

[Development][docs-development] and [Testing][docs-testing] have the rest,
and [AGENTS.md](AGENTS.md) is a full tour of the codebase.

Contributions are welcome: open an issue to report a bug, ask a question or
propose a change.

## License

Apache 2.0. See [LICENSE](LICENSE).

[functions]: https://docs.crossplane.io/latest/packages/functions/
[aicr]: https://github.com/NVIDIA/aicr
[provider-helm]: https://github.com/crossplane-contrib/provider-helm
[provider-kubernetes]: https://github.com/crossplane-contrib/provider-kubernetes
[docs-install]: https://jonasz-lasut.github.io/function-aicr/authors/install.html
[docs-composition]: https://jonasz-lasut.github.io/function-aicr/authors/composition.html
[docs-overlay]: https://jonasz-lasut.github.io/function-aicr/authors/overlay.html
[docs-variants]: https://jonasz-lasut.github.io/function-aicr/authors/variants.html
[docs-components]: https://jonasz-lasut.github.io/function-aicr/authors/components.html
[docs-input]: https://jonasz-lasut.github.io/function-aicr/operators/input-reference.html
[docs-source]: https://jonasz-lasut.github.io/function-aicr/operators/recipe-source.html
[docs-rollout]: https://jonasz-lasut.github.io/function-aicr/operators/rollout-order.html
[docs-upgrades]: https://jonasz-lasut.github.io/function-aicr/operators/upgrades.html
[docs-troubleshooting]: https://jonasz-lasut.github.io/function-aicr/operators/troubleshooting.html
[docs-development]: https://jonasz-lasut.github.io/function-aicr/project/development.html
[docs-testing]: https://jonasz-lasut.github.io/function-aicr/project/testing.html
