# Development

This page is for contributors. It covers the layout of the repository and
the commands that build, generate and lint it. [Testing](testing.md) covers
the tests.

## Layout

| Path | Contents |
|---|---|
| `fn.go` | `RunFunction` and the `stack` type: render, compose, gate, summary. Start here. |
| `main.go` | The command line, the gRPC server and the recipe source. |
| `input/v1beta1/` | The `Input` API types. The CRD under `package/input/` is generated from them. |
| `internal/resolve/` | The overlay from the composite resource, the criteria and the provider config. |
| `internal/compose/` | The `Release` and `Object` builders, manifest rendering, value merging, DRA eviction wiring. |
| `internal/schema/` | OpenAPI v3 validation of composed resources. |
| `test-fixtures/` | The requests `TestRunFunction` sends and the goldens it compares against. |
| `example/clusterstack/` | The XRD, Composition and composite resources of the render walkthrough. |
| `test/e2e/` | The end-to-end suite, a Go module of its own. |
| `Makefile`, `build/` | The end-to-end cluster and the Crossplane make library it uses. |
| `docs/` | This site. |

`fn.go` and `main.go` live in `package main`, the layout of Crossplane's
function template.

## Build

```shell
# Generate the deepcopy code, the Input CRD and the golden fixtures
go generate ./...

# Build
CGO_ENABLED=0 go build ./...

# Lint; the version CI pins is in .github/workflows/ci.yml
golangci-lint run

# Build the runtime image, then a function package from it
docker build . --tag=runtime
crossplane xpkg build -f package --embed-runtime-image=runtime
```

`go.mod` is the source of truth for the Go version. CI reads it and passes it
to the image build.

CI's `check-diff` job runs `go mod tidy` and `go generate ./...` and fails
when either leaves the tree changed, so commit what they produce.

## Add an Input field

1. Add the field to `input/v1beta1/input.go` with its kubebuilder markers.
2. Run `go generate ./...`, which regenerates the deepcopy code and the CRD.
3. Use the field in `fn.go`. The overlay can set it with no further code.
4. Add a fixture under `test-fixtures/input/` and a `TestRunFunction` case.
   Give the case a new golden if the field changes the composed resources,
   regenerate, and review `test-fixtures/want/`.
5. Document it in the [Input reference](../operators/input-reference.md).

## Bump AICR

The version of `aicr.run`, AICR's Go module, is the recipe catalog version. A
bump changes what the function deploys, so it's a minor release and never a
patch.

1. `go get aicr.run@vX.Y.Z && go mod tidy`
2. `go test ./...`. `TestRunFunction` fails wherever recipe content changed.
   `TestCatalogInvariants` fails, with the recipe and component named,
   wherever a recipe's shape broke an assumption of the function.
3. `go generate ./...`, then review the diff under `test-fixtures/want/`.
   It's exactly what the function now deploys.
4. Render both [example](../authors/render-locally.md) composite resources.
5. Re-read AICR's DRA eviction wiring, `injectDRAEvictionLabel` and its
   helpers in `pkg/bundler/bundler.go`. The function has a hand copy of that
   logic; `TestDRAEvictionConstants` catches a renamed constant or a moved
   value path, but not a change in behaviour.
6. Fix behaviour changes by hand. A component the catalog adds joins every
   `TestRunFunction` fixture that doesn't skip it.

`AGENTS.md` in the repository has the same procedure with more detail.

## Conventions

- Go: a pointer to a literal is `new(value)`. Pointer helper functions aren't
  used.
- Tests are table-driven and compare with `go-cmp`. No third-party test
  packages.
- Commits follow the conventional format, `<type>(<scope>): <subject>`, one
  logical change each.
