# Testing

This page is for contributors adding or changing a test. It says what each
suite covers and how the golden fixtures work.

```shell
go test ./...            # everything
go test -race ./...      # with the race detector
```

| Suite | What it covers |
|---|---|
| `TestRunFunction` | The function end to end at the request level: hand-written requests in, the whole response compared. |
| `TestCatalogInvariants` | Every leaf recipe of the embedded catalog, for the properties the function relies on. |
| `TestDRAEvictionAcrossTheCatalog`, `TestDRAEvictionConstants` | The hand copy of AICR's DRA eviction wiring, against the catalog and against AICR's source. |
| `TestNodeSchedulingAcrossTheCatalog` | Node scheduling on every leaf recipe: with both node selectors set, each component is accepted and every manifest still renders. |
| `TestEndToEndCatalog` | That the end-to-end suite's catalog resolves to its own components alone. |
| `TestUpgradeRecords` | That AICR's upgrade records still load and pass AICR's own checks. |
| `TestGate`, `TestSummary` and the other unit tests | One piece each, with hand-built inputs. |
| `internal/resolve`, `internal/compose`, `internal/schema` | The packages' own tables. |

They all run without a cluster. `crossplane render` on the
[example](../authors/render-locally.md) is part of the development loop, not
a suite. The [end-to-end suite](#the-end-to-end-suite) is the one tier that
needs a cluster, and it's a separate Go module.

## The request suite and its goldens

Each case is a request built from fixtures under `test-fixtures/`:

| Directory | Contents |
|---|---|
| `input/` | Hand-written function `Input`s. Cases that send the same `Input` share one. |
| `observed/` | Hand-written observed composite and composed resources. |
| `desired/` | Hand-written desired composite resources handed to the function. |
| `want/` | Generated goldens: the function's own output for those requests. |

The static parts of a response (requirements, results, conditions) are
asserted in the test's code. The desired state is compared against the
goldens under `want/`.

`go generate ./...` rewrites the goldens. It deletes `want/` and runs
`TestRunFunction` with `-update`, so a golden is always the function's
current output. Review the diff: it's what the function deploys. A case that
produces desired resources must have a golden directory, and two cases must
not write different content to the same one.

The fixtures skip most of a recipe's components to pin a small set. When an
AICR bump adds a component, add it to the fixtures' `skipComponents`.

The tests use a schema that accepts anything. They prove the validation is
wired, without pinning the test to a provider's CRD; `internal/schema` tests
the validation itself.

## TestCatalogInvariants

It resolves the leaf recipes of the catalog with AICR's
`recipe.ResolveLeaves` and asserts shape, not content:

- every manifest file renders and parses;
- no two documents map to the same `Object` name;
- every deployed component is in AICR's deployment order;
- every dependency is a component of the recipe;
- the gate emits the whole recipe once everything it emitted is `Ready`.

When it fails after an AICR bump, a recipe's shape broke an assumption. Fix
the function, not the test.

## The end-to-end suite

`test/e2e` runs the function the way a control plane does. `make e2e.up`
builds it from the tree and installs it as a Function package in a kind
cluster, next to Crossplane, provider-helm, provider-kubernetes,
`function-auto-ready` and an OCI registry. The providers compose into the
kind cluster itself. The registry has a small catalog of the suite's own,
which the function serves as its OCI recipe source. A feature deploys one of
its recipes in seconds.

```shell
git submodule update --init   # the Crossplane build submodule, once
make e2e.up
make e2e.test
make e2e.function             # after a code change
make e2e.down
```

The suite follows the one in `crossplane/crossplane`. Features are built with
`sigs.k8s.io/e2e-framework` and Crossplane's e2e helpers, and each keeps its
manifests in a directory under `test/e2e/manifests/`. It's a Go module of its
own, so `go test ./...` at the root doesn't build it.

CI runs it on every pull request.

A second profile deploys a stock recipe with its real charts onto nodes that
KWOK simulates, the way AICR's own KWOK lane does. It pulls every chart from
its upstream registry, so CI runs it on request: on a pull request with the
`e2e:kwok` label, or from the Actions tab.

```shell
make e2e.up E2E_PROFILE=kwok
make e2e.test E2E_PROFILE=kwok
```

[`test/e2e/README.md`][e2e-readme] describes the clusters, the catalog and
how to add a feature.

## Conventions

Tests are tables: a `map[string]struct` keyed by case name, with `reason`,
`args` and `want`. Results are compared with `cmp.Diff`, and a `RunFunction`
case compares the whole response. `testify`, `gomock` and similar packages
aren't used.

[e2e-readme]: https://github.com/jonasz-lasut/function-aicr/blob/main/test/e2e/README.md
