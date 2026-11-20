# Design decisions

The decisions behind the function's design, each with what it rules out.
`AGENTS.md` in the repository has more detail on most of them.

## Composed resources are untyped and validated against required schemas

Importing provider-helm's and provider-kubernetes' typed API structs
conflicts with the function SDK from v0.7 on. Composed resources are plain
maps, and each one is checked against the CRD schema Crossplane resolves
through the required-schemas protocol: the schema of the provider version
that's installed. Without the capability the function is Fatal. Each schema
is compiled once per kind and request.

## Readiness comes from observed state only

The gate reads the conditions of observed resources, never the readiness in
the desired state. `function-auto-ready` sets that later in the pipeline, and
reading it would deadlock the gate. A resource that's already observed is
never withheld.

## A deployed Release is never relocated

A Helm release can't be moved between namespaces. When an AICR release
changes a component's default namespace, the function keeps the namespace the
observed `Release` has. This is the one place where observed state changes a
composed resource instead of only delaying it. The order of the
pipeline can't break it, because the value is read from the `spec` of the observed
`Release`, which is what the function itself last emitted.

## One overlay object instead of per-field paths

Through v0.2 each sourced field had a twin with a field path in it. That
doubled the API with every new field and only fit strings, while AICR's
inputs include lists and maps. `overlayFrom.fieldPath` reads one object with
the fields of the `Input`, so any field can be sourced without a mirrored
type. The XRD documents and checks what a composite resource may set, and
pruning makes it the allowlist.

## A criteria dimension is never defaulted

AICR's catalog decides which combinations exist. An unstated dimension
selects the agnostic tier, and AICR rejects a stated value that isn't part
of a recipe. The function checks the values against AICR's and passes them
through. It
mirrors no rule about which values go together.

## Manifest files are rendered with AICR's renderer

Helm-style templates make up most of the catalog's manifest files. They're
rendered with `aicr.run/pkg/manifest`, exactly as AICR's bundler renders
them. A file that fails to render or parse is Fatal.

## Names are deterministic

A composed resource's name comes from its component and manifest document
alone, because the gate matches observed resources to desired ones by name.
The consequence is one composite resource per target cluster and namespace.

## The recipe source belongs to the function and is pulled once

AICR's pull budget is minutes, and the deadline of a function call is
seconds. A source per request would need an asynchronous cache and a "still
fetching" response. The `--recipe-oci-*` flags configure one catalog per
function deployment, pulled before the function serves. A reconcile never
touches the network, and a bad source fails visibly at startup. Only an immutable
`sha256:` digest is accepted.

## The DRA eviction wiring is a copy

AICR's bundler keeps its eviction wiring unexported, and the function doesn't
use the bundler. `draEviction` applies a hand copy of that logic. One test
compares its names and value paths with the constants in the bundler's
source, and another wires every recipe that has the three components.

## An unsafe upgrade is reported, not held

When AICR's records don't rate a change of chart version safe, the function
warns and applies it. Holding a version back needs a way to acknowledge the
move, which doesn't exist yet.
