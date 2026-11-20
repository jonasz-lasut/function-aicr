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
| `TestUpgradeRecords` | That AICR's upgrade records still load and pass AICR's own checks. |
| `TestGate`, `TestSummary` and the other unit tests | One piece each, with hand-built inputs. |
| `internal/resolve`, `internal/compose`, `internal/schema` | The packages' own tables. |

They all run without a cluster. `crossplane render` on the
[example](../authors/render-locally.md) is part of the development loop, not
a suite.

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

## Conventions

Tests are tables: a `map[string]struct` keyed by case name, with `reason`,
`args` and `want`. Results are compared with `cmp.Diff`, and a `RunFunction`
case compares the whole response. `testify`, `gomock` and similar packages
aren't used.
