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
│     stack.keepNamespaces: a deployed Release keeps its namespace     │
│  7. stack.renderObjects: per component, per wave (pre/post):         │
│       manifest file → RenderManifest → SplitManifests →              │
│       ManifestObjectName → NewObject → rendered                      │
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
    └── providerconfig.go         ProviderConfigRef(): kind enum, name required
    ↓
internal/compose/                 Builds the composed resources (map[string]any)
    ├── release.go                NewRelease, Override, effective(), SplitChart, MergeValues
    ├── manifest.go               RenderManifest via aicr/pkg/manifest.Render
    └── object.go                 SplitManifests, ManifestObjectName, NewObject
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
├── example/clusterstack/    # XRD, Composition, XRs, schema stand-ins, render walkthrough
├── extensions/icons/        # Package icon; supplychain.yml appends it, README.md and an SBOM to the mirror image
├── .claude/skills/          # cut-release, remediate-cves (release procedures)
├── .github/workflows/       # ci (check-diff, lint, unit-test), publish-pkg, tag, grype-scan, supplychain
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
    SkipDeploymentOrder   bool                         // emit everything at once, no gate
    ComponentClasses      []string                     // core and/or ops; unset = every class
    SkipComponents        []ComponentRef               // {name} entries removed from the recipe
    ComponentOverrides    map[string]ComponentOverride // version, namespace, values (RawExtension)
    Target                *Target                      // fieldPath (^status(\..+)?$) for the summary
}
```

The user-facing field reference lives in `README.md` ("Input reference"); keep it in sync with `input/v1beta1/input.go`.

### The recipe source

The recipe catalog is function-level configuration, not part of the `Input`: by default the FS embedded in the pinned `aicr.run` module, optionally overlaid with a digest-pinned OCI recipe artifact via the `--recipe-oci-repository` / `--recipe-oci-digest` flags (`RECIPE_OCI_REPOSITORY` / `RECIPE_OCI_DIGEST`). `newDataProvider` (main.go) pulls the artifact **once at startup** through `aicr/pkg/recipe/ocisource` — never per request, so no reconcile waits on a registry — and a pull failure exits the process rather than falling back to embedded data. `ociPullOptions` requires both flags together and an immutable `sha256:` manifest digest (AICR refuses to materialize tag-selected artifacts anyway). The overlay's `repository@digest` identity is threaded through `Function.recipeSource` into the summary. Registry credentials come from the Docker config (`DOCKER_CONFIG`), connections are HTTPS-only, and extraction needs a writable temp dir (`TMPDIR`); see the README's "Recipe source" section for the deployment wiring.

### Input resolution

Every field of the `Input` is static, the value all composite resources get, unless the overlay sets it. `resolve.Input` reads the object at `overlayFrom.fieldPath` on the observed XR and merges it over the static Input:

1. maps merge key by key, the overlay wins, static keys it does not set are kept
2. a list replaces the static list whole
3. a null or `""` counts as unset and keeps the static value (Kubernetes cannot tell `""` from omitted); under `componentOverrides.<component>.values` an empty string is a chart value like any other
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

- **Release** (`helm.m.crossplane.io/v1beta1`) — one per chart-backed component, named after the component. `spec.forProvider.wait: true` always; chart repository/name from `SplitChart(source, EffectiveChart())`; namespace/version/values from `effective()` with the override applied.
- **Object** (`kubernetes.m.crossplane.io/v1alpha1`) — one per document of every `preManifestFiles` (wave `pre`) and `manifestFiles` (wave `post`) entry, named `ManifestObjectName(component, wave, doc, index)` = `<component>-<pre|post>-<kind>-<slugged name>` (or `<component>-<wave>-<index>` when a document has no kind/name). Manifest files are Go/Helm templates rendered with `aicr/pkg/manifest.Render` from the same effective namespace/chart/version/values as the Release, so an override cannot make them disagree.

Names are pure functions of their inputs because the gate correlates observed against desired resources by name. This also means one composite per target namespace: two would claim the same names.

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
5. Document it in the README's "Input reference" table

### Bumping the AICR Module

The `aicr.run` version is the recipe catalog version (AICR renamed its Go module from `github.com/NVIDIA/aicr` in v1.3.0). A bump is a **minor release** from `main` (see `/cut-release`), never a patch.

1. `go get aicr.run@vX.Y.Z && go mod tidy`
2. `go test ./...` — `TestRunFunction` fails wherever recipe content changed; `TestCatalogInvariants` fails, naming the leaf and component, wherever a recipe's *shape* broke an assumption (a manifest that no longer renders, a name collision, a dependency edge into a component the gate cannot see)
3. `go generate ./...` and **review the diff under `test-fixtures/want/`** — it is exactly what the function will now deploy. Commit it with the bump.
4. Re-render the examples (`example/clusterstack/README.md`) for both XRs
5. Fix behaviour changes by hand: the static parts of each response are asserted in code, not regenerated. A component the catalog adds joins every `TestRunFunction` fixture that does not list it under `skipComponents`; add it there to keep the pinned tier small

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

- `README.md` — user-facing behaviour, the Input reference, the required-schemas contract and gating rules
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
