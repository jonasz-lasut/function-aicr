# Function-AICR Agent Guide

This document provides orientation for AI agents and developers working with the function-aicr codebase.

## What is Function-AICR?

Function-AICR is a **Crossplane Composition Function** that deploys an NVIDIA AI Cluster Runtime (AICR) recipe. Given recipe criteria (accelerator, service, intent, OS, platform), it resolves the matching recipe from AICR's embedded catalog and expands it into namespaced provider-helm `Release`s and provider-kubernetes `Object`s targeting a workload cluster, rolled out in AICR's dependency order.

### Purpose

The main goal is to make an AICR recipe a **continuously reconciled Crossplane API** rather than a one-shot bundle, while staying **XR-agnostic** — the function never knows the composite resource's kind. This provides:

- **An overlay from the XR**: `overlayFrom.fieldPath` merges one object on the XR over the static `Input`, so one Composition serves every cluster, each XR naming its own recipe and target cluster
- **Schema-validated output**: every composed resource is checked against the provider CRD schema Crossplane resolves for it (required-schemas protocol) before it is emitted
- **Ordered rollout**: components are withheld until what AICR orders before them is observed `Ready`
- **Manifest files as `Object`s**: AICR's Helm-templated manifest files are rendered with AICR's own renderer and wrapped in provider-kubernetes `Object`s
- **A resolved-recipe summary** written to a `status` field path the user chooses
- **An optional OCI recipe source**: the `--recipe-oci-*` flags overlay the embedded catalog with a digest-pinned recipe artifact, pulled once at startup

### Relationship to AICR

AICR (`github.com/NVIDIA/aicr`, Go module `aicr.run`) is a CLI, API server and Go library that resolves recipes and renders them into Helm, Argo CD, Flux or Helmfile bundles. Function-AICR embeds AICR's recipe engine (`pkg/recipe`) and catalog and adapts the deployment to Crossplane:

| AICR CLI / bundler | Function-AICR |
|--------------------|---------------|
| Criteria on the command line | Criteria in the function `Input`, or in the overlay read from the observed XR |
| Renders a bundle for an external deployer to apply once | Returns desired `Release`s and `Object`s to Crossplane, which reconciles them continuously |
| Deployment order left to the deployer's tooling (Argo waves, Helmfile needs, ...) | Enforced by a gate that reads the providers' observed `Ready` conditions |
| Reads recipe data from its embedded FS or an OCI source | Reads the embedded FS of the pinned `aicr.run` module version; the `--recipe-oci-*` flags overlay it with a digest-pinned OCI recipe artifact pulled once at startup |

The pinned module version **is** the recipe catalog version. Bumping it changes what the function deploys (see [Bumping the AICR module](#bumping-the-aicr-module)).

## Architecture Overview

### Request Processing Flow

```
Crossplane RunFunctionRequest
    ↓
┌──────────────────────────────────────────────────────────────────────┐
│ fn.go: RunFunction()                                                 │
│                                                                      │
│  1. resolve.Input: static Input + the overlay at overlayFrom on      │
│     the observed XR, decoded strictly; validateTarget (fieldPath     │
│     must be status[.…]); parseOverrides (values: a JSON object)      │
│  2. resolve.ProviderConfigRef and resolve.Criteria from the          │
│     resolved Input; Criteria.Validate                                │
│  3. Fatal unless Crossplane advertises CAPABILITY_REQUIRED_SCHEMAS   │
│  4. resolve.Profile, resolve.BuildOptions (configuration);           │
│     recipe.NewBuilder(...).BuildFromCriteriaWithProfile → result     │
│  5. managedComponents: enabled, not skipped, Helm-typed refs         │
│     (Warnings for skip/override names that match nothing)            │
│  6. resolveValues: effective values per component                    │
│     stack.applyNodeScheduling (nodeScheduling): selectors and        │
│     tolerations at the registry's paths                              │
│     stack.wireDRAEviction (draEviction): the three-chart contract    │
│     stack.keepNamespaces: a deployed Release keeps its namespace     │
│  7. stack.renderObjects: per component, per wave (pre/post):         │
│       manifest file → RenderManifest → SplitManifests →              │
│       ManifestObjectName → NewObject → rendered                      │
│     stack.requireNodeSelectors + checks.Run (AICR's bundle checks)   │
│     → stack.refuse: Fatal for a component that is not deployed yet,  │
│     Warning for one that is                                          │
│  8. stack.needs → RequireSchema("release") / ("object") as needed    │
│     composedKind.validator: not yet resolved → return with no        │
│     resources; resolved to nil → Fatal "is provider-… installed?"    │
│  9. stack.composeReleases: NewRelease → Validate → desired           │
│     stack.composeObjects: rendered → Validate → desired              │
│ 10. stack.gate (unless skipDeploymentOrder): withhold from desired   │
│     what is not yet allowed by observed readiness                    │
│     stack.warnUnsafeUpgrades: version changes AICR rates not safe    │
│ 11. SetDesiredComposedResources; if target: setStatus(summary)       │
│ 12. ConditionTrue FunctionSuccess                                    │
└──────────────────────────────────────────────────────────────────────┘
    ↓
Crossplane RunFunctionResponse (desired resources + requirements + XR status)
```

### Key Components

```
fn.go                             RunFunction, the stack type (render, compose, gate, summary),
                                  managedComponents, parseOverrides, setStatus, recipeName
main.go                           kong CLI, DataProvider (embedded, or the OCI overlay from
                                  the --recipe-oci-* flags), aicrVersion/moduleVersion
    ↓
input/v1beta1/input.go            Input, OverlayFrom, Criteria, ProviderConfigRef,
                                  ComponentRef, ComponentOverride, Target
    ↓
internal/resolve/                 static Input + observed XR → the resolved Input
    ├── input.go                  Input(): static Input + overlay, strict decoding
    ├── criteria.go               Criteria(): ≥1 dimension
    ├── selection.go              Profile(): one name=value; BuildOptions(): configuration
    ├── scheduling.go             NodeScheduling(): AICR validates labels and tolerations
    └── providerconfig.go         ProviderConfigRef(): kind enum, name required
    ↓
internal/compose/                 Builds the composed resources (map[string]any)
    ├── release.go                NewRelease, Override, effective(), SplitChart, MergeValues
    ├── manifest.go               RenderManifest via aicr/pkg/manifest.Render
    ├── scheduling.go             ApplyNodeScheduling, RequireNodeSelectors: a copy of the bundler's pass
    ├── draeviction.go            WireDRAEviction: a copy of the bundler's eviction wiring
    └── object.go                 SplitManifests, ManifestObjectName, NewObject
    ↓
internal/checks/                  Run AICR's registry-declared bundle checks (checks.Config, checks.Run)
    ↓
internal/schema/                  Validator: compile a required schema once, Validate many
    ↓
aicr.run/pkg/recipe               Builder, DataProvider, RecipeResult, ComponentRef,
                                  ResolveLeaves (tests), GetManifestContentWithContext
```

## Codebase Structure

```
function-aicr/
├── fn.go                    # Function implementation (START HERE)
├── fn_test.go               # TestRunFunction (goldens), TestCatalogInvariants, TestGate, ...
├── main.go                  # CLI entry point, gRPC server, aicr module version from build info
│
├── input/
│   ├── generate.go          # controller-gen: Input CRD → package/input/
│   └── v1beta1/             # Input API definitions (+ zz_generated.deepcopy.go)
│
├── internal/
│   ├── resolve/             # Input overlay from the XR, criteria and provider config
│   ├── compose/             # Release/Object builders, manifest rendering, value merging
│   ├── checks/              # AICR's bundle checks, run with AICR's own functions
│   └── schema/              # OpenAPI v3 validation of composed resources
│
├── test-fixtures/
│   ├── generate.go          # go generate → `go test ../ -run ^TestRunFunction$ -update`
│   ├── input/               # Hand-written function Inputs, one per TestRunFunction case
│   ├── observed/            # Hand-written observed XRs and composed resources
│   ├── desired/             # Hand-written desired XRs handed to the function
│   └── want/                # GENERATED goldens: the function's output for the above
│
├── package/
│   ├── crossplane.yaml      # Function package metadata
│   └── input/               # GENERATED Input CRD
├── test/e2e/                # End-to-end suite: its own Go module, on crossplane/crossplane's e2e framework
├── Makefile                 # The end-to-end cluster (make e2e.up / e2e.test / e2e.down), nothing else
├── build/                   # Submodule: crossplane/build, the Makefile's make library
├── docs/                    # The documentation site's pages (mdBook; SUMMARY.md is the table of contents)
├── book.toml                # mdBook configuration; docs/theme/aicr.css is the palette
├── .vale.ini                # Vale prose lint over README.md, docs/ and the example and e2e READMEs
├── example/clusterstack/    # XRD, Composition, XRs, schema stand-ins, render walkthrough
├── extensions/icons/        # Package icon; supplychain.yml appends it, README.md and an SBOM to the mirror image
├── .claude/skills/          # cut-release, remediate-cves (release procedures)
├── .github/workflows/       # ci (check-diff, lint, unit-test), docs, e2e, publish-pkg, tag, grype-scan, supplychain
├── .github/renovate.json5   # Renovate: grouped bump PRs; aicr bumps wait for dependency-dashboard approval
├── .golangci.yml            # Lint configuration (golangci-lint v2)
└── Dockerfile               # Distroless runtime image; Go version comes from go.mod
```

`fn.go` and `main.go` deliberately live in `package main` (the Crossplane function template layout). Do not move the function into an internal package.

## Key Concepts

### Input

The function receives an `Input` (`aicr.fn.crossplane.io/v1beta1`) — a KRM-like object whose CRD is generated only to describe its schema:

```go
type Input struct {
    OverlayFrom           *OverlayFrom                 // fieldPath of an Input-shaped object on the XR, merged over the rest
    Criteria              *Criteria                    // accelerator/service/intent/os/platform
    Profile               map[string]string            // one recipe variant, {gpuStack: dra}
    Configuration         *Configuration               // gke.tcpxoInterfaces, runtimeInventory.mode
    ProviderConfigRef     *ProviderConfigRef           // kind (enum, default ClusterProviderConfig) + name
    NodeScheduling        *NodeScheduling              // system / accelerated: nodeSelector, tolerations
    DRAEviction           bool                         // wire AICR's DRA eviction contract
    SkipDeploymentOrder   bool                         // emit everything at once, no gate
    ComponentClasses      []string                     // core and/or ops; unset = every class
    SkipComponents        []ComponentRef               // {name} entries removed from the recipe
    ComponentOverrides    map[string]ComponentOverride // version, namespace, values (RawExtension)
    Target                *Target                      // fieldPath (^status(\..+)?$) for the summary
}
```

The user-facing field reference is `docs/operators/input-reference.md`; keep it in sync with `input/v1beta1/input.go`.

### The recipe source

The recipe catalog is function-level configuration, not part of the `Input`: by default the FS embedded in the pinned `aicr.run` module, optionally overlaid with a digest-pinned OCI recipe artifact via the `--recipe-oci-repository` / `--recipe-oci-digest` flags (`RECIPE_OCI_REPOSITORY` / `RECIPE_OCI_DIGEST`). `newDataProvider` (main.go) pulls the artifact **once at startup** through `aicr/pkg/recipe/ocisource` — never per request, so no reconcile waits on a registry — and a pull failure exits the process rather than falling back to embedded data. `ociPullOptions` requires both flags together and an immutable `sha256:` manifest digest (AICR refuses to materialize tag-selected artifacts anyway). The overlay's `repository@digest` identity is threaded through `Function.recipeSource` into the summary. Registry credentials come from the Docker config (`DOCKER_CONFIG`), connections are HTTPS-only, and extraction needs a writable temp dir (`TMPDIR`); see `docs/operators/recipe-source.md` for the deployment wiring.

The recipe source is also the only way to opt in to one of AICR's opt-in mixins (the NVSentinel remediation steps, `nvsentinel-observability`, the `NVIDIADriver` wiring): a mixin applies only when an overlay lists it under `spec.mixins`, and no build option selects one. `docs/operators/mixins.md` shows the two-file artifact. It was verified end to end on 2026-11-20 against a local TLS registry: a tarball pushed with `oras push <file>:application/vnd.oci.image.layer.v1.tar+gzip` is accepted, while the layer `oras` packs from a directory is refused with "OCI recipe layer contains an unsafe archive path component" because the archive has a `.` entry.

### Input resolution

Every field of the `Input` is static, the value all composite resources get, unless the overlay sets it. `resolve.Input` reads the object at `overlayFrom.fieldPath` on the observed XR and merges it over the static Input:

1. maps merge key by key, the overlay wins, static keys it does not set are kept
2. a list replaces the static list whole
3. a null or `""` counts as unset and keeps the static value (Kubernetes cannot tell `""` from omitted); under `componentOverrides.<component>.values` an empty string is a chart value like any other, and in a `nodeScheduling` node selector it is a label's value
4. a path absent from the XR → the static Input alone
5. a path holding anything but an object, an overlay that sets `overlayFrom` (or `apiVersion`, `kind`, `metadata`), a field the Input does not have → error (Fatal), never a silent fallback

Both the static Input and the merged result are decoded strictly (`DisallowUnknownFields`), so a typo in the Composition or on the XR is Fatal. The XRD is what limits the overlay: Crossplane prunes fields the XRD does not declare, so a composite resource controls exactly the Input fields the XRD author put under the overlay path. `resolve.Criteria` and `resolve.ProviderConfigRef` then read the resolved Input.

`profile` and `configuration` shape the recipe after the criteria selected it: `resolve.Profile` turns the one-entry map into AICR's `name=value` selection for `BuildFromCriteriaWithProfile`, and `resolve.BuildOptions` turns each set `configuration` choice into a `recipe.BuildOption`. Each is passed only when set, because AICR rejects one on a recipe it does not apply to, and AICR validates the values: the function mirrors no list of profile names, modes or interface rules.

At least one criteria dimension must be set; every unset dimension is passed to AICR **unstated**. Never default a dimension client-side (see Design Decisions). `Criteria.Validate()` (AICR's) checks stated values against AICR's enums; AICR's resolver then rejects a stated value no recipe honors.

### Managed components

`managedComponents` (fn.go) turns `RecipeResult.ComponentRefs` into the map the rest of the request works on:

- `ComponentRefs` **includes components the recipe disables** (`IsEnabled() == false`); only `DeploymentOrder` omits them. Filter on `IsEnabled()`, as AICR's own deployers do.
- `skipComponents` removes more; a name matching nothing raises a Warning result.
- `componentClasses` keeps only the components whose class in AICR's registry (`registry.Get(name).GetClass()`, `core` or `ops`) it names; `validateClasses` makes any other name Fatal, since a misspelled class would deploy nothing and report success.
- Only `recipe.ComponentTypeHelm` refs are managed; other types are reported as Normal results and skipped.
- A dependency that is not managed (skipped, disabled, absent) counts as satisfied for gating.

### Composed resources and their names

Composed resources are `map[string]any` (see Design Decisions), built in `internal/compose`:

- **Release** (`helm.m.crossplane.io/v1beta1`) — one per chart-backed component, named after the component. `spec.forProvider.wait: true` always; chart repository/name from `SplitChart(source, EffectiveChart())`, which never trims the source: provider-helm joins the two for an OCI chart as AICR's deployers do, also when the source ends in the chart's name (`kai-scheduler` is at `oci://ghcr.io/kai-scheduler/kai-scheduler/kai-scheduler`; trimming it broke the pull until the KWOK tier found it); namespace/version/values from `effective()` with the override applied.
- **Object** (`kubernetes.m.crossplane.io/v1alpha1`) — one per document of every `preManifestFiles` (wave `pre`) and `manifestFiles` (wave `post`) entry, named `ManifestObjectName(component, wave, doc, index)` = `<component>-<pre|post>-<kind>-<slugged name>` (or `<component>-<wave>-<index>` when a document has no kind/name). Manifest files are Go/Helm templates rendered with `aicr/pkg/manifest.Render` from the same effective namespace/chart/version/values as the Release, so an override cannot make them disagree.

- **Namespace Object** - `stack.namespaceObject` adds `compose.NewNamespaceObject` to the `pre` wave of a component that renders pre-manifest documents, and of a manifest-only component that renders any, named as a Namespace document of the component's own would be (`<component>-pre-namespace-<namespace>`). AICR's bundler installs manifest files as a chart of their own with `--create-namespace`; without the equivalent a pre-manifest Object fails with `namespaces "..." not found` and its Release, which waits for it, is never emitted (the end-to-end suite found this). `managementPolicies: [Observe, Create]` makes the provider create a missing namespace and never update or delete one. A component whose manifest files declare the namespace (the Talos `talos-namespace.yaml` files) keeps its own Object.

Names are pure functions of their inputs because the gate correlates observed against desired resources by name. This also means one composite per target namespace: two would claim the same names.

### Node scheduling

`nodeScheduling.system` and `nodeScheduling.accelerated` each hold a `nodeSelector` and `tolerations` (`corev1.Toleration`). `resolve.NodeScheduling` validates them with AICR's own `config.ValidateNodeSelector` / `config.ValidateTolerations`, and `stack.applyNodeScheduling` writes them into every managed component's values at the paths its registry entry lists (`ComponentConfig.GetSystemNodeSelectorPaths` and its three siblings). It runs right after `resolveValues`, before any override is merged, because the bundler applies its structured overrides (`--set-json`) after scheduling: a `componentOverrides` value wins.

`compose.ApplyNodeScheduling` is a copy of the orchestration AICR's bundler keeps unexported (`applyNodeSchedulingOverrides`, `classifySchedulingPaths` and `applyTolerationPaths` in `aicr.run/pkg/bundler/bundler.go`), built on the helpers `aicr.run/pkg/component` exports. The policy comes from the recipe's inline overrides only (`ComponentRef.Overrides`), never from the values file: an empty value at a path opts it out, a non-empty one makes tolerations append, everything else is replaced, and a toleration path listed for both tiers gets `component.SharedTolerations`. Two deliberate differences from the CLI: nothing is defaulted (the CLI tolerates every taint when its flags are unset, which here would reschedule every running stack on a function upgrade), and the bundler's other inputs to the same pass (workload selector, workload gate, node count, storage class) are not implemented.

`stack.requireNodeSelectors` then runs `compose.RequireNodeSelectors` on the values with the override merged: a registry entry with `requireNodeSelector` and no selector at a non-opted-out path is a blocking finding, as `validateRequiredNodeSelectors` fails a bundle. `stack.refuse` decides what a blocking finding does (see Bundle checks below). In aicr v1.3.0 that is `slinky-slurm`, `slurm-accounting-mariadb` and `nvsentinel-mongodb`, and 6 of the 81 leaves (the Slurm recipes for AKS, EKS and GKE) fail without a selector. `TestNodeSchedulingAcrossTheCatalog` applies both tiers to every leaf, requires the check to pass and the manifests to render, and fails if a registry entry starts to use `requireNodeSelectorIfStorageClassSet`, the conditional variant that is not ported.

### Bundle checks

AICR's registry declares `validations` per component (a function name, a severity, criteria conditions and a message), which the bundler runs before it writes a bundle. `internal/checks` runs them with AICR's own functions (`validations.RunValidations` from `aicr.run/pkg/bundler/validations`), so nothing is copied: `checks.Config` describes the request as the bundle configuration AICR's command line would build (`nodeScheduling` as the four scheduling options, `skipComponents` as `--set <component>:enabled=false`, `componentOverrides` values as typed overrides, `draEviction` as the eviction label), and `checks.Run` returns one `Finding` per result for the managed components, with the four scheduling flags in AICR's messages replaced by the Input fields. Nothing is defaulted in that configuration: the checks themselves know that an empty accelerated toleration list renders a wildcard.

`stack.refuse` applies one rule to these findings and to the required node selectors: a blocking finding is Fatal for a component nothing of which is observed yet (`stack.isObserved`: its Release, or an Object of a manifest-only component), and a Warning ending in `is already deployed, so it is not refused` otherwise; a non-blocking finding is a Warning. A function or catalog upgrade therefore never freezes a running stack. It runs after `stack.renderObjects`, because a manifest-only component's Objects are what says it is deployed.

A check's `conditions` also match a recipe that leaves the dimension unstated, so a recipe without a service is checked as if it could be any service. In aicr v1.3.0, with no `nodeScheduling`, five of the 81 leaves have a blocking finding: the four AKS recipes (`CheckWildcardAcceleratedToleration` on `nodewright-customizations`) and `gb300-oke-ubuntu-training` (`CheckAcceleratedSelectorMissing`). `TestBundleChecksAcrossTheCatalog` requires every leaf to be clean when both placements are stated with keyed tolerations, and at least one leaf to be refused without them, which is what fails if AICR changes how its checks read the configuration. AICR's checks log through `log/slog`; `main.go` raises that logger to error level unless `--debug` is set. `aicr.run/pkg/bundler/validations` is not a stable API.

### DRA eviction

`draEviction: true` makes `stack.wireDRAEviction` write AICR's DRA eviction contract into the values of `gpu-operator` (`driver.manager.env` gains `NODE_LABEL_FOR_GPU_POD_EVICTION`), `nvidia-dra-driver-gpu` (`kubeletPlugin.nodeSelector` gains the label) and `dra-node-labeler` (`enabled`, `labelKey`, `labelValue`). It runs before the manifests are rendered and after the overrides are merged (`stack.takeValues` merges and clears them), as the bundler injects after user overrides. Missing any of the three components is a Warning and nothing is wired.

`compose.WireDRAEviction` is a copy of logic AICR's bundler keeps unexported (`injectDRAEvictionLabel`, `mergeDRAEvictionNodeSelector` and `upsertGPUOperatorDRAEvictionEnv` in `aicr.run/pkg/bundler/bundler.go`); only the default label is imported, from `aicr.run/pkg/defaults`. Two tests guard the copy: `TestDRAEvictionConstants` parses the bundler source of the pinned module and compares its constants with ours, and `TestDRAEvictionAcrossTheCatalog` wires every leaf that has the three components and requires the labeler to render.

### Sticky namespaces

`stack.keepNamespaces` runs before the manifests are rendered. A chart-backed component whose observed Release has a `spec.forProvider.namespace` other than the recipe's keeps the observed one (it is written into the component's `compose.Override`, so the Release and the manifests agree) and a Warning names both namespaces. An explicit `componentOverrides.<component>.namespace` wins and is how a component is moved. A manifest-only component has no Release: it follows a dependency whose namespace was kept when the recipe places both in the same namespace, so a CR stays next to its operator. aicr v1.3.0 moved `nodewright-operator` from `skyhook` to `nodewright`; `KeepsTheNamespaceOfADeployedRelease` pins that case.

### The gate

`stack.gate` (fn.go) removes from `desired` whatever is not yet allowed by **observed** readiness, per managed component:

- pre-manifest Objects wait for the component's managed dependencies to be `Ready`
- the Release waits for those dependencies and for its pre-manifest Objects
- post-manifest Objects wait for the Release
- a manifest-only component (`IsManifestOnlyHelm()`) has no Release: all its Objects wait for its dependencies, and its readiness as a dependency is that of **every Object it emitted** (none emitted → nothing to wait for)

Two invariants: readiness is read from `observed`, never from `desired[...].Ready` (that is set by function-auto-ready later in the pipeline — reading it would deadlock); and `withhold` never removes something already observed (the gate orders creation, it must not tear down a live resource because a sibling flapped). Every withholding emits a `Normal` result naming the blocker.

### Upgrade warnings

`stack.warnUnsafeUpgrades` runs after the gate. `stack.versionChanges` collects the chart-backed components whose observed Release has a `spec.forProvider.chart.version` other than the one `compose.Placement` now gives, and `unsafeUpgrades` runs them through `upgrade.Match` against the records `recipe.LoadUpgradeRecords` reads from the catalog (ADR-021). Every result whose `FailsRun()` is true, which is everything but `safe`, becomes a Warning quoting AICR's `Explanation` and the `Summary` of each crossed record. Nothing is held back: holding a version needs a design for acknowledging the move. `aicr.run/pkg/upgrade` is in AICR's "Internal" tier, so expect signature changes on a bump; `TestUpgradeRecords` fails when the embedded records no longer load or validate.

### Required schemas

`composedKind` (fn.go) wraps the protocol for the two kinds. Only kinds the recipe composes are required: `stack.needs` runs after `stack.renderObjects`, so a recipe whose manifest files render no document does not force provider-kubernetes to be installed. In the aicr v1.3.0 catalog 78 of the 81 leaf recipes include the manifest-only `dra-node-labeler`, whose manifest renders nothing unless its `enabled` value is true. Decision table: capability absent → Fatal; requirement unresolved → return with no composed resources (Crossplane calls again); resolved to nil → Fatal naming the provider; resolved → `schema.NewValidator` once per kind, `Validate` every resource, violation Fatal.

### The resolved-recipe summary

`stack.summary` returns `recipeName` (`accelerator-service[-os]-intent[-platform]`, unstated dimensions omitted), `recipeVersion` (the aicr module version), `recipeSource` (the OCI overlay's `repository@digest`, present only when the function serves one), `profile` (`Metadata.SelectedProfile` as `name=value`, which AICR also records for a default variant), `appliedOverlays` (`Metadata.AppliedOverlays`), `componentCount`, `deployedComponents` and `pendingComponents`. The last three cover the managed components that compose a resource (`stack.composes`: chart-backed, or manifest-only with at least one rendered Object), in `DeploymentOrder`: a deployed one has everything it composes present in `desired` and is listed with the namespace and chart version from `compose.Placement`, overrides and a kept namespace included; a pending one carries `waitingFor`, the blocker `stack.gate` recorded in `stack.waiting`. A manifest-only component whose manifests render nothing, such as `dra-node-labeler` by default, appears in neither list and is not counted. `setStatus` sets each key individually under `target.fieldPath` on the desired XR read from the request, so sibling keys survive and the function never needs the XR's kind. Crossplane prunes fields the XRD does not declare — the example XRD declares `status.aicr`.

## Development Guide

### Building

```bash
# Generate code (deepcopy + Input CRD, and the golden fixtures)
go generate ./...

# Build
CGO_ENABLED=0 go build ./...

# Build the runtime image (Go version taken from go.mod in CI)
docker build . --tag=runtime

# Build a Crossplane package
crossplane xpkg build -f package --embed-runtime-image=runtime
```

### Testing

```bash
# Run all tests
go test ./...

# Only the end-to-end RunFunction cases (golden-backed)
go test . -run '^TestRunFunction$'

# Every leaf recipe of the embedded catalog
go test . -run '^TestCatalogInvariants$'

# Race detection
go test -race ./...
```

`TestRunFunction` (fn_test.go) is the end-to-end suite: hand-written request fixtures under `test-fixtures/{input,observed,desired}/`, static response parts (requirements, results, conditions) asserted in code, and the desired state compared against goldens under `test-fixtures/want/`. `TestCatalogInvariants` resolves every leaf recipe with `recipe.ResolveLeaves` and asserts shape, not content: every manifest renders and parses, no two documents share an Object name, every managed component is in `DeploymentOrder`, every dependency names a real component, and the gate drains the whole recipe when everything it emits turns Ready. `TestGate` drives `stack.gate` through hand-built managed/observed maps.

### End-to-End Suite

`test/e2e` runs the function as a control plane does: built from the tree, installed as a Function package in a kind cluster (`function-aicr-e2e-<profile>`, with a kubeconfig of its own at `_output/e2e/kubeconfig-<profile>` that the Makefile exports as `KUBECONFIG`, so no target and no feature can reach another cluster through the developer's current context) with Crossplane, provider-helm and provider-kubernetes (a `ClusterProviderConfig` named `e2e` on each, pointing at the kind cluster itself), function-auto-ready and an OCI registry at `registry.e2e.svc:5443`. `test/e2e/README.md` is the reference.

The suite deploys a catalog of its own, `test/e2e/catalog`: one chart that starts a pause container (`charts/e2e-app`) and a recipe tree (`recipe/`) with three components, `e2e-first` (a chart), `e2e-second` (a chart that depends on it, with a pre-manifest and a post-manifest file) and `e2e-notes` (manifest-only, depends on `e2e-second`). `make e2e.catalog` pushes both to the registry and applies the function's runtime config with the recipe artifact's digest, so the function serves the catalog through `--recipe-oci-*`, the same path an operator's catalog takes. AICR layers it over the embedded catalog, so `recipe/overlays` replaces the stock `base`, `kind`, `kind-inference` and `monitoring-hpa` overlays, which the suite's criteria (`service: kind`; with `intent: training` a recipe that moves `e2e-first` to another namespace; with `intent: inference` a recipe that adds `e2e-checked`, whose registry entry declares AICR's `CheckAcceleratedSelectorMissing`) would otherwise also select. The check is on a component of its own and has no conditions: AICR applies a condition on a criteria dimension to every recipe that leaves the dimension unstated (`recipe.MatchesCriteriaField`), so a check conditioned on `intent` would also refuse the plain `service: kind` recipe. `TestEndToEndCatalog` (`e2e_catalog_test.go`, a unit test) resolves the three recipes through a layered provider and fails when an AICR bump adds a stock overlay that joins them: replace that overlay in the catalog too.

```bash
git submodule update --init   # crossplane/build, once
make e2e.up                   # kind, Crossplane, providers, registry, the function from this tree
make e2e.test                 # go test in test/e2e against that cluster
make e2e.function             # rebuild and reinstall the function after a code change
make e2e.catalog              # publish the catalog again after a change to it
make e2e.down
```

Two things hold it together. The Makefile uses the make library of the `build` submodule (`controlplane.mk`, `local.xpkg.mk`) and adds what the library lacks for a Function package: `xpkg.build.function-aicr` and `local.xpkg.deploy.function.%`, which loads the image into the node and installs the package from Crossplane's cache with `packagePullPolicy: Never`. The suite is a Go module of its own (`test/e2e/go.mod`), so that importing `github.com/crossplane/crossplane/v2/test/e2e/config` and `.../funcs` does not pull Crossplane's dependency tree into the function's `go.mod`; `go test ./...` at the root never builds it.

Write a feature the way crossplane/crossplane does: a `Test...` function handing `environment.Test` a `features.NewWithDescription(...)`, labelled with `LabelArea`, `LabelSize` and `config.LabelTestSuite` (a feature without the last is failed by `TestMain`), its manifests under `test/e2e/manifests/<area>/<feature>/` and applied with `funcs.ApplyResources`. The Makefile is for the end-to-end cluster only: building, testing, generating and linting stay plain Go commands.

The recipe features (`recipe_test.go`) share one XRD and Composition (`manifests/recipe/setup`) and each creates one composite resource in `default`, one at a time, deleted in the foreground so the next feature does not find the same Helm releases. A feature controls readiness without touching the Release: `TestRolloutOrder` gives `e2e-first`'s pod a node selector no node matches and labels the node to let it through. `recipe_gate_test.go` covers the rest of the gate the same way: `TestRolloutSteps` holds `e2e-second` at each of its own steps (a `ResourceQuota` of zero ConfigMaps in its namespace keeps the pre-manifest Object from being created; a node selector keeps its Release unready), `TestSkipDeploymentOrder` and `TestSkippedDependency` cover the two opt-outs, and `TestLiveResourcesAreKept` covers the rule that nothing observed is withheld. provider-helm (v1.5.0, Helm v4 with the kstatus wait) waits for readiness only while it installs or upgrades: `Observe` reports a Release `Available` whenever the Helm release is `deployed` and matches the spec, so a pod that stops running later never makes a Release unready. The feature therefore pauses the Release (`crossplane.io/paused`) and writes `Ready=False` to its status itself. Crossplane runs with `--circuit-breaker-burst=1000` (`CROSSPLANE_ARGS`): with the default burst of 100 a rollout opens the composite resource's watch circuit breaker within seconds and every further gate step waits up to 30 seconds.

The suite has a second profile, `kwok` (`make e2e.up E2E_PROFILE=kwok`, `make e2e.test E2E_PROFILE=kwok`), on a cluster of its own: the function serves the embedded catalog and `TestStockRecipeOnSimulatedNodes` (`kwok_test.go`, test suite label `kwok`) deploys the EKS `h100` training recipe with its real charts onto nodes that KWOK simulates. `make e2e.kwok` applies the KWOK release at the Makefile's `KWOK_VERSION` (Renovate moves it) and the six nodes in `test/e2e/manifests/kwok/nodes.yaml`, which were rendered once from AICR v1.3.0's node template and EKS profiles (`kwok/templates/nodes/node.yaml.tmpl`, `kwok/profiles/eks`) and are plain manifests from then on: nothing is read from the module at run time. It also labels the real node `eks.amazonaws.com/compute-type=hybrid`, as AICR's lane does, which keeps the EBS CSI node plugin off it. The Composition under `manifests/kwok/setup` is a hand copy of what AICR's lane passes to `aicr bundle` (`kwok/scripts/validate-scheduling.sh`: the scheduling flags as `nodeScheduling`, the `conditional_sets` as `componentOverrides`), plus two skipped components: `nodewright-customizations` as in the lane, and `nktp`, whose operator webhook guards DELETE and so blocks the uninstall on simulated nodes. provider-helm waits for readiness where AICR's lane only checks scheduling, so a pod that tolerates every taint and lands on the real node has to be able to run there. The first run of this tier found that `SplitChart` broke the `kai-scheduler` chart reference.

`.github/workflows/e2e.yml` runs the catalog profile on every pull request and on `main`, and the kwok profile on a pull request labelled `e2e:kwok` or on a manual run with the `kwok` input. It is not a required check of the `protect-main` ruleset, so Renovate keeps the Makefile's pins, the suite's Go module and the submodule in groups that never automerge: GitHub's auto-merge would not wait for it.

### Test Patterns

Tests are table-driven with `map[string]struct` keyed by case name and single inline `args`/`want` structs:

```go
cases := map[string]struct {
    reason string
    args   args
    want   want
}{
    "TestCaseName": {
        reason: "Description of what this tests",
        args:   args{...},
        want:   want{...},
    },
}
for name, tc := range cases {
    t.Run(name, func(t *testing.T) { ... })
}
```

Compare with `go-cmp` — `cmp.Diff(want, got)`, using `cmp.AllowUnexported` / `cmp.Transformer` where needed — rather than field-by-field `if`/`t.Errorf`. Do **not** add third-party test packages (`testify`, `assert`, `require`, `gomock`); stdlib `testing` plus `go-cmp` only.

### Test Assertion Conventions

`RunFunction` tests compare the **full expected response** with `cmp.Diff` and `protocmp.Transform()`, and errors with `cmpopts.EquateErrors()`. Construct the whole `*fnv1.RunFunctionResponse` — including `Results` for fatal cases (`fatal(msg)`) and `Requirements` (`releaseRequirement()`, `releaseAndObjectRequirements()`) — and diff it; the desired state is asserted through the case's `golden{resources, composite}` names. A case that produces desired resources must name a golden directory, and two cases must not regenerate the same golden with different content (`writeGolden` fails both).

`fnv1.Schema` carries the OpenAPI document in its `OpenapiV3` field directly (`permissiveSchema()`); tests use a permissive schema so they prove the plumbing without pinning provider-helm's real CRD, which `internal/schema` tests separately.

### Golden Fixtures and Generated Files

`go generate ./...` regenerates everything derived from source, and CI's `check-diff` job fails if it leaves the tree dirty:

- `input/generate.go` → `zz_generated.deepcopy.go` and `package/input/aicr.fn.crossplane.io_inputs.yaml` (controller-gen)
- `test-fixtures/generate.go` → `rm -rf want` then `go test ../ -run ^TestRunFunction$ -count=1 -update`; the `-update` flag on the test binary rewrites `test-fixtures/want/` from the function's own output

`go generate` visits only files with the `generate` build tag; do not build a standalone generator program for the goldens (it would force the code under test out of `package main`), and never build with `-tags generate` yourself. Test binaries carry no module list in build info, so `fn_test.go` pins `testRecipeVersion = "v0.0.0-test"` where `main.go` would read the real aicr version.

### Documentation

The documentation is an mdBook site built from `docs/` (`book.toml`; `docs/SUMMARY.md` lists the pages, and a page that is not listed is not rendered). `.github/workflows/docs.yml` builds it, checks its links and lints its prose on every pull request that touches it, and deploys `main` to GitHub Pages.

```bash
mdbook serve                 # preview at http://localhost:3000
mdbook build                 # into book/ (gitignored)
vale sync                    # fetch the style packages .vale.ini pins, once
vale README.md example/clusterstack/README.md test/e2e/README.md docs
lychee --offline --include-fragments --exclude-path book/404.html 'book/**/*.html' README.md
```

Any Vale alert fails CI, warnings included. Fix the prose rather than the configuration; `.vale.ini` turns a rule off only with the reason beside it, and `.vale/styles/config/vocabularies/Aicr/accept.txt` holds the project's terms. A change in behaviour is documented in the page that covers it, not in `README.md`, which is only the landing page (it is also what the package's mirror image publishes). The pages are written for a reader who has not seen the code: no first person, contractions, sentence-case headings, a short dash or a colon and never an em dash.

### Linting

```bash
golangci-lint run
```

Configuration is `.golangci.yml` (golangci-lint v2, the version CI pins is in `.github/workflows/ci.yml`). `goimports` groups local imports under `github.com/jonasz-lasut/function-aicr`.

### Coding Conventions

- Go ≥ 1.26 (`go.mod` is the source of truth; CI and the Dockerfile read it). For a pointer to a literal use `new(value)`; never write pointer helper funcs.
- Prefer self-documenting code; comments explain *why*, as the existing ones do.
- English only; inclusive terminology (allowlist/blocklist, primary/replica, main branch).
- Conventional commits: `<type>(<scope>): <subject>`, imperative, ≤ 50 chars, one logical change per commit.

## Common Development Tasks

### Adding an Input Field

1. Add the field to `input/v1beta1/input.go` with kubebuilder markers (`+optional`, enums, patterns)
2. `go generate ./...` — regenerates deepcopy and the CRD under `package/input/`
3. Wire it in `fn.go`. The overlay can set the new field with no further code; declare it in the example XRD if a composite resource should control it
4. Add a `test-fixtures/input/*.yaml` fixture and a `TestRunFunction` case; name a new golden if it changes desired state, then `go generate ./...` and review `test-fixtures/want/`
5. Document it in `docs/operators/input-reference.md`, and in the page that covers its behaviour

### Bumping the AICR Module

The `aicr.run` version is the recipe catalog version (AICR renamed its Go module from `github.com/NVIDIA/aicr` in v1.3.0). A bump is a **minor release** from `main` (see `/cut-release`), never a patch.

1. `go get aicr.run@vX.Y.Z && go mod tidy`
2. `go test ./...` — `TestRunFunction` fails wherever recipe content changed; `TestCatalogInvariants` fails, naming the leaf and component, wherever a recipe's *shape* broke an assumption (a manifest that no longer renders, a name collision, a dependency edge into a component the gate cannot see)
3. `go generate ./...` and **review the diff under `test-fixtures/want/`** — it is exactly what the function will now deploy. Commit it with the bump.
4. Re-render the examples (`example/clusterstack/README.md`) for both XRs
5. Run `TestBundleChecksAcrossTheCatalog` and `internal/checks`' tests with attention: they call `aicr.run/pkg/bundler/validations` and `aicr.run/pkg/bundler/config` directly, neither of which is a stable API, and a new check may read a bundle option `checks.Config` does not set yet
6. Re-read the bundler's node scheduling pass: `applyNodeSchedulingOverrides`, `classifySchedulingPaths`, `applyTolerationPaths` and `validateRequiredNodeSelectors` in `aicr.run/pkg/bundler/bundler.go`, of which `compose.ApplyNodeScheduling` and `compose.RequireNodeSelectors` are hand copies. `TestNodeSchedulingAcrossTheCatalog` catches a registry entry that needs the unported storage class condition; a changed rule only shows in the source
7. Re-read the bundler's DRA eviction wiring: `injectDRAEvictionLabel` and its two helpers in `aicr.run/pkg/bundler/bundler.go`, plus `draNodeLabelerDropReason` for when the labeler applies. `compose.WireDRAEviction` is a hand copy of that logic. `TestDRAEvictionConstants` catches a renamed constant or a moved value path; a change in behaviour (a new component in the contract, a new condition, a different merge rule) only shows in the source, so diff those functions against the previous AICR version and port what changed
8. Label the bump's pull request `e2e:kwok`: the kwok profile deploys a stock recipe from the new catalog. Compare `kwok/scripts/validate-scheduling.sh` with the Composition under `test/e2e/manifests/kwok/setup` when it fails: AICR adds a chart override there when a component stops coming up on simulated nodes. The nodes in `test/e2e/manifests/kwok/nodes.yaml` do not follow the module; re-render them if AICR's EKS node profiles change in a way the recipe depends on
9. Fix behaviour changes by hand: the static parts of each response are asserted in code, not regenerated. A component the catalog adds joins every `TestRunFunction` fixture that does not list it under `skipComponents`; add it there to keep the pinned tier small

Since aicr v0.19.0 the resolver enforces a coverage post-condition: every stated dimension must be honored by an applied overlay. Do not reintroduce client-side defaults (e.g. `os: ubuntu`) or cross-field rules — leave unresolved dimensions unstated and let AICR answer.

### Changing What a Release or Object Looks Like

`internal/compose/release.go` (`NewRelease`, `effective`), `internal/compose/object.go` (`NewObject`, `ManifestObjectName`) and `internal/compose/manifest.go` (`RenderManifest`). Any field you add is validated against the real provider CRD at runtime, so check it exists in provider-helm's / provider-kubernetes' schema; the example `schemas/` stand-ins cover only the fields currently set and need extending too. Renaming a composed resource changes the gate's correlation key and orphans existing resources — treat as breaking.

### Changing the Gate

`stack.gate`, `blockingDependency`, `componentReady`, `withhold` and `deployed` in `fn.go`; `TestGate` for unit cases and the gate-convergence check inside `TestCatalogInvariants`. Any new readiness path must handle manifest-only dependencies (they never appear under their bare name in `observed`) or it deadlocks silently with an endless stream of "delaying …" results.

### Debugging Schema Resolution

- `composedKind.require` / `composedKind.validator` in `fn.go` — the request/response side
- `internal/schema/schema.go` — `structpb.Struct` → `apiextensions.JSONSchemaProps` → `validation.NewSchemaValidator`
- `example/clusterstack/README.md` — the `--required-schemas` directory format for `crossplane render`, and how it was verified against Crossplane's source

### Rendering Locally

```bash
go run . --insecure --debug
```

```bash
cd example/clusterstack
crossplane render xr.yaml composition.yaml functions.yaml \
  --required-schemas=schemas/ --include-function-results
```

`functions.yaml` uses the Development runtime, so the function must be running locally. With nothing observed only dependency-free components appear — that is the gate working; pass `--observed-resources` marking a dependency `Ready` to see the next component. `xr-eks.yaml` exercises the manifest-file path (`Object`s).

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `aicr.run` | Recipe engine (`pkg/recipe`), manifest renderer (`pkg/manifest`) and the embedded catalog; its version is what the function deploys |
| `github.com/crossplane/function-sdk-go` | Crossplane function SDK (request/response helpers, required schemas) |
| `github.com/crossplane/crossplane-runtime/v2` | `fieldpath.Paved` for the overlay and status field paths on the XR |
| `github.com/crossplane/crossplane/apis/v2` | Condition types (`xpv2.TypeReady`) |
| `k8s.io/apiextensions-apiserver` | OpenAPI v3 schema validation of composed resources |
| `sigs.k8s.io/controller-tools` | Input CRD generation (`go generate`) |
| `github.com/google/go-cmp` | Test comparisons |

## Important Design Decisions

### Composed Resources Are Untyped, Validated Against Required Schemas

Importing provider-helm's / provider-kubernetes' typed API structs conflicts with `function-sdk-go` ≥ 0.7 (crossplane-runtime v2.3 dropped `apis/common`, which the provider API packages still import). Instead resources are `map[string]any` and every one is validated against the CRD schema Crossplane resolves through the required-schemas protocol — the schema of whatever provider version is actually installed. Missing capability is Fatal rather than a silent no-op; a nil schema is Fatal naming the provider. Validators are compiled once per kind per request (`internal/schema.NewValidator`) — compiling per resource is ~16× slower.

### Readiness Comes From Observed State Only

The gate reads `observed[...]` conditions, never `desired[...].Ready`, because readiness in desired state is set by function-auto-ready later in the pipeline. Nothing already observed is ever withheld. Manifest-only dependencies are resolved through the Objects they emitted, not their bare name.

### One Overlay Object Instead of Per-Field Paths

Until v0.2 each sourced field had a `...From` twin holding a field path (`criteriaFrom`, `providerConfigRefFrom`). That doubled the API with every new field and only fit strings, while AICR's inputs now include lists and maps. `overlayFrom.fieldPath` reads one Input-shaped object instead: every field is sourceable with no mirrored types, the XRD documents and validates what a composite resource may set, and pruning makes the XRD the allowlist. A ConfigMap as the overlay source was rejected for the opposite properties: nothing validates it at admission and nothing shows what it may contain. The cost is that the XR nests the values under one object in the Input's shape.

### Never Default a Criteria Dimension Client-Side

AICR's catalog decides which combinations exist. Unstated dimensions select the agnostic tier; a stated value no overlay honors is rejected by AICR (coverage post-condition since v0.19.0). The function validates enums and passes criteria through; it mirrors no cross-field rules.

### `recipeVersion` Comes From Build Info

`main.go` reads the `aicr.run` module version from `debug.ReadBuildInfo()` (honoring a `replace`) and injects it into `Function`; test binaries carry no module list, so tests pin `testRecipeVersion`. `moduleVersion(bi, path)` is the pure, testable helper.

### Manifest Files Are Rendered With AICR's Renderer

30 of the 36 manifest files in the v0.19.0 catalog are Go/Helm templates. Skipping files that contain `{{` would silently drop most recipe content, so they are rendered with `aicr/pkg/manifest.Render` exactly as AICR's bundler does; a render or parse failure is Fatal.

### A Deployed Release Is Never Relocated

A Helm release cannot be moved between namespaces, so when an AICR bump changes a component's default namespace the function keeps the namespace the observed Release has, rather than emitting the new one. This is the one place observed state shapes a composed resource instead of only gating it. It is safe against the pipeline-order problem the gate has, because the value is read from the observed Release's spec, which is what the function itself last emitted, not from a status another function sets. AICR solves the same problem for its bundles with `aicr recipe --inherit-from <previous> --inherit=namespace`.

### Deterministic Names, One Composite per Namespace

Composed resource names derive only from the component and manifest document, because the gate correlates observed and desired by name and the Helm release names would collide in the target cluster anyway.

### The OCI Recipe Source Is Function-Level and Pulled Once at Startup

AICR's pull budget is minutes (staging, digest authorization, extraction) while a `RunFunction` deadline is seconds, so a per-request source would need an async provider cache and a "still fetching" response path. Instead the `--recipe-oci-*` flags configure one catalog per function deployment, pulled by `ocisource.New` before `function.Serve`: reconciles never touch the network, and a bad source crashloops visibly instead of silently serving embedded data (a fallback would change what the function deploys). Only an immutable `sha256:` digest is accepted — AICR refuses to materialize tag-selected artifacts, and a tag would let registry state change deployments without a config change. The overlay is layered over the embedded catalog, so `TestCatalogInvariants` guards only the embedded base — an overlay can break its shape assumptions (a manifest that does not render is Fatal at compose time; a manifest-document name collision is not separately guarded) — and the summary therefore reports `recipeSource` next to `recipeVersion`.

## Key Reference Documents

- `docs/` — the documentation site (https://jonasz-lasut.github.io/function-aicr/): user-facing behaviour, the Input reference, the required-schemas contract, gating rules, upgrades and troubleshooting
- `README.md` — the landing page: the pitch, a quick start and links into the site
- `example/clusterstack/README.md` — `crossplane render` walkthrough and the `--required-schemas` format
- `input/v1beta1/input.go` — authoritative Input schema
- `test-fixtures/generate.go` — how the goldens are produced
- `.claude/skills/cut-release/SKILL.md` — cutting a minor/major release (`release-X.Y` branch, tag, package publish); one release branch is kept at a time
- `.claude/skills/remediate-cves/SKILL.md` — patch releases for Grype/code-scanning findings against the latest release; never bumps `aicr.run`

## Releasing

Releases are driven by two skills; use them rather than improvising the branch/tag/publish sequence:

- **`/cut-release`** — a new minor or major version from `main` HEAD: new `release-X.Y` branch, tag (`.github/workflows/tag.yml`), GitHub release, package publish (`publish-pkg.yml` → `ghcr.io/jonasz-lasut/function-aicr`, mirrored to `xpkg.upbound.io/jonasz-lasut/function-aicr`), signing/attestation (`supplychain.yml`). The bump size is the user's choice, never inferred. An aicr module bump always ships this way.
- **`/remediate-cves`** — a patch release on the current `release-X.Y` branch for CVEs found by `grype-scan.yml` (weekly against the latest release). It never bumps `aicr.run`.

## Troubleshooting

### "function-aicr requires a Crossplane that supports required schemas"

Crossplane did not advertise `CAPABILITY_REQUIRED_SCHEMAS`. Upgrade Crossplane, or pass `--required-schemas=DIR` to `crossplane render`.

### "no schema for …, Kind=Release: is provider-helm installed?"

The requirement resolved to nothing: the provider whose CRD carries that GVK is not installed in the cluster (or the JSON under `--required-schemas` lacks a schema tagged with that `x-kubernetes-group-version-kind`).

### Nothing is composed, or only dependency-free components appear

Either the schemas are still being resolved (the function returns no resources until Crossplane calls back with them), or the gate is withholding: read the `Normal` results ("delaying X until dependency Y is ready"). Set `skipDeploymentOrder: true` only if you truly want everything at once.

### "delaying X until dependency Y is ready" forever

Y is not reaching `Ready` in the target cluster (check the `Release`/`Object` in the provider), or — if you changed the gate — Y is manifest-only and is being looked up under its bare name.

### "cannot pull the OCI recipe source …" at startup

The function exits (and the pod crashloops) when the `--recipe-oci-*` source cannot be pulled or validated. `--recipe-oci-digest` must be the artifact's immutable `sha256:` manifest digest, not a tag. An auth error means the Docker config (`DOCKER_CONFIG`) lacks credentials for the registry; a TLS error means the registry's CA is not in the trust store (`SSL_CERT_FILE`); "invalid materialized OCI recipe catalog" means the artifact is not a recipe tree rooted at `registry.yaml`; an error naming the `apiVersion` of the external `registry.yaml` means the artifact was authored for an AICR older than v1.0.0, which retired the alpha catalog versions. There is deliberately no fallback to the embedded catalog.

### "this recipe ships torch-distributed-tcpxo and requires configuration.gke.tcpxoInterfaces"

The `h100` / `gke` / `cos` / `training` / `kubeflow` recipe needs a GKE TCPXO interface mapping. Set `configuration.gke.tcpxoInterfaces`, usually through the overlay, since the network names are per cluster. `TestCatalogInvariants` resolves the leaf with AICR's introspection mapping so its shape stays covered.

### "no recipe provides os 'ubuntu' for criteria(…)" / "invalid AICR recipe criteria"

A stated dimension is not honored by any overlay for the others. Leave it unstated, in the Input and in the overlay, rather than defaulting it.

### The summary does not show up in the XR's status

The XRD does not declare a schema for `target.fieldPath`; Crossplane prunes undeclared status fields. Add the subschema (see `example/clusterstack/definition.yaml`).

### `TestRunFunction` fails after a dependency bump

Recipe content changed. `go generate ./...`, review the diff under `test-fixtures/want/`, commit it. If `TestCatalogInvariants` fails, a recipe's shape broke an assumption — fix the function, not the test.

### CI `check-diff` fails

`go mod tidy` or `go generate ./...` produced changes that were not committed. Run both and commit the result.
