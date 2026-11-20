# Install

This page is for whoever installs the function on a Crossplane control plane.
It lists what the function needs around it and how to install the package.

## What it needs

| Needed | Why |
|---|---|
| Crossplane v2.2 or later | The function validates what it composes against the provider CRD schemas, which it asks Crossplane for. Crossplane advertises that capability from v2.2.0, and without it the function is Fatal. |
| [provider-helm][provider-helm] with the `helm.m.crossplane.io/v1beta1` `Release` | Every chart-backed component becomes a namespaced `Release`. |
| [provider-kubernetes][provider-kubernetes] with the `kubernetes.m.crossplane.io/v1alpha1` `Object` | Every document of a component's manifest files becomes a namespaced `Object`. Only needed when the recipe renders one. See [Schema validation](../operators/schema-validation.md). |
| A provider config for the target cluster | A `ClusterProviderConfig` or a `ProviderConfig` of each provider, under the same name. The function stamps that name on every resource it composes. |

## The package

Releases are published as `ghcr.io/jonasz-lasut/function-aicr` and mirrored
at `xpkg.upbound.io/jonasz-lasut/function-aicr`. The
[releases page][releases] lists the versions.

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: function-aicr
spec:
  package: ghcr.io/jonasz-lasut/function-aicr:<version>
```

The recipe catalog comes with the function's version. Each release embeds
the catalog of one AICR release, and upgrading the function can change what
an existing composite resource deploys. Read [Upgrades](../operators/upgrades.md)
before moving a cluster to a new version.

Install [`function-auto-ready`][auto-ready] too and run it after
function-aicr in the pipeline. function-aicr orders the rollout from the observed readiness
of the resources it composes, and leaves the composite resource's own
readiness to the pipeline.

The next page is [Write a Composition](composition.md).

[provider-helm]: https://github.com/crossplane-contrib/provider-helm
[provider-kubernetes]: https://github.com/crossplane-contrib/provider-kubernetes
[auto-ready]: https://github.com/crossplane-contrib/function-auto-ready
[releases]: https://github.com/jonasz-lasut/function-aicr/releases
