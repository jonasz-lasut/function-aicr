# function-aicr

A [Crossplane Composition Function][functions] that deploys an
[NVIDIA AI Cluster Runtime (AICR)][aicr] recipe. Name the recipe criteria —
accelerator, service, intent, OS, platform — and the function resolves the
matching recipe from AICR's catalog and expands it into namespaced
[provider-helm][provider-helm] `Release`s and
[provider-kubernetes][provider-kubernetes] `Object`s targeting a workload
cluster, rolled out in AICR's dependency order.

## Overview

`function-aicr` is a Crossplane composition function for GPU cluster stacks.
AICR captures known-good combinations of GPU drivers, operators and platform
components as version-locked recipes; this function embeds AICR's recipe
engine and catalog, so there is nothing to install besides the function
package and the two providers whose resources it composes. See the
[AICR documentation][aicr-docs] for the recipes themselves.

Each recipe component becomes one `Release` for its Helm chart and one
`Object` per document of the manifest files it carries. Every composed
resource is validated against the provider CRD schema Crossplane resolves for
it before it is emitted, and withheld from the desired state until what AICR
orders before it reports `Ready`.

## Usage

Use `function-aicr` as a step in a Crossplane Composition pipeline. The
function takes an `Input` naming the recipe criteria — literally, or as field
paths into the composite resource — and the provider config of the target
cluster:

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
      criteriaFrom:
        accelerator: spec.accelerator
        service: spec.service
        intent: spec.intent
        os: spec.os
        platform: spec.platform
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
  accelerator: h100
  service: kind
  intent: training
```

The XRD author exposes whatever field names they like; `criteriaFrom` (and
`providerConfigRefFrom`) wire them to the function without it ever knowing
the XR's kind. Put [function-auto-ready][auto-ready] after `function-aicr`:
the function gates on the observed readiness of the resources it composes and
leaves the XR's own readiness to the pipeline.

Releases are published as `ghcr.io/jonasz-lasut/function-aicr`, mirrored at
`xpkg.upbound.io/jonasz-lasut/function-aicr`.

## Why Crossplane?

AICR renders a recipe into Helm, Argo CD, Flux or Helmfile bundles. Running
it as a Crossplane composition instead adds what the platform brings:

- **Continuous reconciliation.** The recipe is desired state, not a one-shot
  bundle. provider-helm and provider-kubernetes keep every `Release` and
  `Object` converged, so a component that drifts or disappears from the
  workload cluster is put back, and a change to the recipe rolls out through
  the same machinery.
- **One API, many clusters.** Define an XRD of your own shape and let each
  composite resource name its accelerator, service, intent and target
  cluster. One Composition serves every workload cluster.
- **Ordered rollout on observed readiness.** Components are withheld until
  what AICR orders before them reports `Ready` — read from the providers'
  observed conditions, not from an install script that returned.
- **Pipeline composability.** Add functions before or after: derive criteria
  or overrides from the XR, enforce policy, aggregate readiness — each an
  independent step that leaves the recipe untouched.
- **Safe rollouts.** CompositionRevisions let you pin existing stacks to a
  known-good revision and roll a new recipe version forward incrementally.
- **Developer experience.** Test locally with `crossplane render`; every
  composed resource is checked against the real provider CRD schema before
  it is ever applied.

## How it works

### Criteria resolution

For each of `accelerator`, `service`, `intent`, `os` and `platform` the
function resolves a final value from the input:

1. If `criteriaFrom.<field>` is unset, the literal `criteria.<field>` is used
   (which may itself be empty).
2. If `criteriaFrom.<field>` is set and the field path resolves to a
   non-empty string on the observed composite resource, that value wins.
3. If the field path is absent or an empty string, the function falls back
   to the literal — Kubernetes API conventions make `""` and an omitted
   optional field indistinguishable.
4. If the field path is malformed, or resolves to a value that is not a
   string, the function is Fatal — never a silent fallback.

`providerConfigRefFrom.{kind,name}` follow the same rules against
`providerConfigRef.{kind,name}`, so the target cluster can come from the XR
as well.

No single dimension is required, but at least one must resolve — an input
whose `criteriaFrom` paths all miss the composite is Fatal rather than a
silent deployment of the base recipe. Every unresolved dimension is passed to
AICR unstated, so resolution selects the agnostic tier for it; AICR's catalog
alone decides which combinations exist and rejects a stated value no recipe
honors.

### Components, charts and manifest files

The function deploys the recipe's Helm components that AICR enables: a
component an overlay disables stays in AICR's inventory but is never
composed, exactly as AICR's own deployers treat it. `skipComponents` removes
further components; a skipped or disabled component that another depends on
counts as satisfied for gating — the user opted to manage it elsewhere.

A chart-backed component becomes a `helm.m.crossplane.io/v1beta1` `Release`
named after the component (`cert-manager`, `gpu-operator`), so the Helm
release in the target cluster carries that name too; `componentOverrides`
adjust its chart version, namespace and values. Components also carry
manifest files — `preManifestFiles` that must exist before the chart and
`manifestFiles` that follow it; a manifest-only component has no chart at
all. Those files are Helm-style templates, which the function renders with
AICR's own renderer from the component's effective namespace, chart, version
and values (so an override shapes the manifests exactly as it shapes the
`Release`) and wraps, one document each, in a
`kubernetes.m.crossplane.io/v1alpha1` `Object` named
`<component>-<pre|post>-<kind>-<name>`. A file that fails to render or parse
is Fatal — a recipe is never deployed with a piece silently missing.

Composed resource names are deterministic because the dependency gate
correlates observed against desired resources by them. The consequence is one
composite resource per target cluster and namespace — which matches the
one-stack-per-cluster shape of a recipe, whose Helm release names would
collide in the target cluster anyway.

### Schema validation

Composed resources are built as plain maps, not typed provider structs, so
the function validates every one against the actual CRD schema Crossplane
resolves for it, using Crossplane's required-schemas protocol
(`CAPABILITY_REQUIRED_SCHEMAS`). There are four states:

| State | What Crossplane and the function do |
|---|---|
| Crossplane does not advertise `CAPABILITY_REQUIRED_SCHEMAS` | **Fatal.** The function needs a Crossplane (or `crossplane render` invocation) that supports required schemas. |
| The function has asked for a schema but Crossplane has not resolved it yet | The function emits **no composed resources** this pass and asks again. Crossplane re-invokes the pipeline once the schema is resolved. |
| Crossplane resolves the requirement to nothing (no CRD for the GVK) | **Fatal**, e.g. `no schema for helm.m.crossplane.io/v1beta1, Kind=Release: is provider-helm installed?` |
| Crossplane resolves the requirement to a schema | Every composed resource of that kind is **validated** against it. A violation is Fatal. |

Only the schemas the resolved recipe needs are required: a recipe with no
manifest files does not oblige you to install provider-kubernetes, and one
with no charts does not oblige you to install provider-helm. With
`crossplane render`, supply schemas via `--required-schemas=DIR`, a directory
of JSON files in the OpenAPI v3 document format
`kubectl get --raw /openapi/v3/<group-version>` returns — see the
[example README](example/clusterstack/README.md).

### Dependency ordering

Every `Release` sets `spec.forProvider.wait: true`; that is the readiness
signal the gate reads, since provider-helm reports a `Release` `Ready` only
once its Helm release's resources are actually up. Unless
`skipDeploymentOrder: true`, the function withholds composed resources from
the desired state in the order AICR deploys them:

- a component's pre-manifest `Object`s wait for its managed dependencies
  (components in this recipe, not skipped) to report `Ready`;
- its `Release` waits for those dependencies and for its pre-manifest
  `Object`s;
- its post-manifest `Object`s wait for its `Release`;
- a manifest-only component has no `Release`, so all of its `Object`s wait
  for its dependencies, and its readiness — as somebody else's dependency —
  is that of every `Object` it emitted.

A resource that has already been observed is never gated: the gate orders
creation and must never tear down a live resource because a sibling went
briefly unready.

### The resolved-recipe summary

When `target.fieldPath` is set, the function writes a summary of the resolved
recipe to that path on the desired composite resource:

```yaml
status:
  aicr:
    recipeName: h100-kind-training
    recipeVersion: v0.19.0
    componentCount: 3
    deployedComponents:
    - name: cert-manager
    - name: gpu-operator
```

`recipeVersion` is the `github.com/NVIDIA/aicr` module version the function
was built with — what pins the embedded recipe data. `deployedComponents`
lists, in AICR's deployment order, only the components the gate is not
withholding. The keys are set individually, so keys already present at the
path survive. Crossplane prunes any status field the XRD does not declare,
so the XRD must carry a matching subschema — see
[`example/clusterstack/definition.yaml`](example/clusterstack/definition.yaml).

## Input reference

The function's input is a KRM-like object of `kind: Input`,
`apiVersion: aicr.fn.crossplane.io/v1beta1`. Its authoritative source is
[`input/v1beta1/input.go`](input/v1beta1/input.go); the generated CRD lives at
[`package/input/aicr.fn.crossplane.io_inputs.yaml`](package/input/aicr.fn.crossplane.io_inputs.yaml).

| Field | Type | Default | Notes |
|---|---|---|---|
| `criteria` | object (`Criteria`), optional | — | Literal AICR recipe criteria. Any field may be overridden per composite resource via `criteriaFrom`. |
| `criteria.accelerator` | string, optional | `""` | e.g. `h100`, `b200`, `a100`. Validated against AICR's accelerator enum. |
| `criteria.service` | string, optional | `""` | e.g. `eks`, `gke`, `aks`, `kind`. Validated against AICR's service enum. |
| `criteria.intent` | string, optional | `""` | `training` or `inference`. |
| `criteria.os` | string, optional | `""` (unstated) | e.g. `ubuntu`, `cos`, `talos`. Left unstated when neither `criteria.os` nor a resolving `criteriaFrom.os` supplies a value, so AICR selects the OS-agnostic recipe tier. AICR rejects an OS no recipe for the service honors (e.g. `ubuntu` on `kind`), and asks for one when a service+accelerator pair has only OS-specific recipes. |
| `criteria.platform` | string, optional | `""` | e.g. `kubeflow`, `slurm`, `runai`, `dynamo`, `nim`. |
| `criteriaFrom` | object (`CriteriaFrom`), optional | — | Field paths into the observed composite resource whose values take precedence over the matching `criteria` field. |
| `criteriaFrom.accelerator` | `*string`, optional | unset | A field path, e.g. `spec.accelerator`. |
| `criteriaFrom.service` | `*string`, optional | unset | |
| `criteriaFrom.intent` | `*string`, optional | unset | |
| `criteriaFrom.os` | `*string`, optional | unset | |
| `criteriaFrom.platform` | `*string`, optional | unset | |
| `providerConfigRef` | object (`ProviderConfigRef`), optional | — | The provider config for the target cluster, stamped onto every composed `Release` and `Object`. Either it or `providerConfigRefFrom` must supply a name. |
| `providerConfigRef.kind` | string, optional | `ClusterProviderConfig` | Enum: `ClusterProviderConfig`, `ProviderConfig`. |
| `providerConfigRef.name` | string, optional | — | Required unless `providerConfigRefFrom.name` resolves. |
| `providerConfigRefFrom` | object (`ProviderConfigRefFrom`), optional | — | Field paths into the observed composite resource whose values take precedence over the matching `providerConfigRef` field — the same rules as `criteriaFrom` — so one Composition can target the cluster each composite resource names. |
| `providerConfigRefFrom.kind` | `*string`, optional | unset | A field path, e.g. `spec.providerConfigKind`. A resolved kind must be `ClusterProviderConfig` or `ProviderConfig`. |
| `providerConfigRefFrom.name` | `*string`, optional | unset | A field path, e.g. `spec.clusterRef.name`. |
| `skipDeploymentOrder` | bool, optional | `false` | When `true`, every component is emitted at once rather than withheld until its dependencies report `Ready`. |
| `skipComponents` | `[]ComponentRef`, optional | `[]` | Components excluded from the resolved recipe. Each entry is `{name: string}` (`name` required). An entry naming no component of the resolved recipe raises a Warning result — a typo would otherwise silently deploy the component. |
| `componentOverrides` | `map[string]ComponentOverride`, optional | `{}` | Keyed by component name. An entry for anything not managed — misspelled, skipped, or disabled by the recipe — is ignored with a Warning result. |
| `componentOverrides[name].version` | string, optional | `""` (recipe's version) | Overrides the component's chart version. |
| `componentOverrides[name].namespace` | string, optional | `""` (recipe's namespace) | Overrides the component's target namespace. |
| `componentOverrides[name].values` | object, optional | unset | Deep-merged over the values the recipe resolved; override wins on conflict. Must be an object — any other JSON kind is Fatal on the first reconcile, before any provider schema is requested. |
| `target` | object (`Target`), optional | unset | Where the resolved-recipe summary is written on the desired composite resource. When unset, no status is written. |
| `target.fieldPath` | string, **required if `target` set** | — | Must be `"status"` or begin with `"status."`. Names the object that receives the summary keys; sibling keys already there are preserved. |

## Examples

See the [`example/clusterstack`](example/clusterstack/) directory for a
complete working example — an XRD, a Composition, and composite resources
that exercise it — with a `crossplane render` walkthrough:

| Example | Description |
|---------|-------------|
| [xr.yaml](example/clusterstack/xr.yaml) | An `h100` / `kind` / `training` stack: `cert-manager` renders first, `gpu-operator` once it is observed `Ready` |
| [xr-eks.yaml](example/clusterstack/xr-eks.yaml) | An EKS tier whose manifest-only `nodewright-customizations` component renders its Skyhook tuning CR into a provider-kubernetes `Object` |
| [schemas/](example/clusterstack/schemas/) | Hand-written stand-ins for the `Release` and `Object` CRD schemas that `crossplane render --required-schemas` needs, and how to replace them with the real ones |

See the [example README](example/clusterstack/README.md) for setup
instructions and the walkthrough.

## Development

```shell
# Run code generation - see input/generate.go and test-fixtures/generate.go
$ go generate ./...

# Run tests - see fn_test.go
$ go test ./...

# Lint - see .golangci.yml
$ golangci-lint run

# Build the function's runtime image - see Dockerfile
$ docker build . --tag=runtime

# Build a function package - see package/crossplane.yaml
$ crossplane xpkg build -f package --embed-runtime-image=runtime
```

`go generate ./...` regenerates the `Input` CRD under `package/input/` and the
golden fixtures under `test-fixtures/want/` — the function's own output for
the hand-written request fixtures, pinning the exact charts, versions and
values the embedded recipes resolve to. After bumping `github.com/NVIDIA/aicr`,
run it and review the diff under `test-fixtures/want/`: it is exactly what
the function will now deploy. `TestCatalogInvariants` resolves every leaf
recipe of the embedded catalog and checks the properties the function relies
on across all of them, so a bump that breaks one fails with the leaf and
component named. See [AGENTS.md](AGENTS.md) for a full tour of the codebase.

## Contributing

We welcome contributions of all kinds — opening issues, improving
documentation, fixing bugs, or adding new features. If you don't know where
to start or have any questions, please open an issue.

## License

Apache 2.0. See [LICENSE](LICENSE) for details.

[functions]: https://docs.crossplane.io/latest/packages/functions/
[aicr]: https://github.com/NVIDIA/aicr
[aicr-docs]: https://docs.nvidia.com/aicr
[provider-helm]: https://github.com/crossplane-contrib/provider-helm
[provider-kubernetes]: https://github.com/crossplane-contrib/provider-kubernetes
[auto-ready]: https://github.com/crossplane-contrib/function-auto-ready
