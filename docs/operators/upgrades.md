# Upgrades

Each release of the function embeds the catalog of one AICR release, so an
upgrade of the function is also an upgrade of every stack it serves. Chart
versions move, components appear, and the default namespace of a component
can change. This page covers what the function does about that, and what to check
before moving on from v0.2.

The catalog comes with the Function package, not with the Composition. A new
function version reaches every composite resource the function serves, on
its next reconcile.

## Upgrade warnings

A pass can change the chart version of a `Release` that's already deployed:
a new AICR release moved the recipe's pin, or an override changed. The
function looks every such move up in AICR's upgrade records. They rate a
recorded move `safe`, `manual` or `blocked`. A move the records don't cover
is `unknown`, or `blocked` when it crosses a recorded boundary from a version
that isn't the start of a record. Anything but `safe` raises a Warning on the
composite resource with AICR's explanation and the summary of each record the
move crosses.

The upgrade is still applied. The Warning is raised on the passes where the
observed and the composed version differ, which is while the change is being
applied, so it isn't a check to run before an upgrade. The records are in AICR's catalog under
`recipes/components/<component>/upgrades.yaml`.

## Namespaces

A component that's already deployed keeps its namespace when the new recipe
places it in another one. See
[Components and overrides](../authors/components.md). The Warning it raises
quotes both namespaces and the override that moves the component.

## Moving on from v0.2

v0.2 embeds AICR v0.20.0. The version after it embeds AICR v1.3.0 and changes
the `Input`.

### The Input

`criteriaFrom` and `providerConfigRefFrom` are removed. Move the values under
one object on the composite resource and point `overlayFrom.fieldPath` at it.
See [Values from the composite resource](../authors/overlay.md). A
Composition that still sets a removed field is Fatal, and the message says
what replaced it.

A field the `Input` doesn't have is now Fatal. It used to be ignored.

In the [status summary](../authors/summary.md), `componentCount` is now the
number of components the recipe deploys. It used to equal the length of
`deployedComponents`.

### The catalog

AICR's own upgrade records don't rate these moves safe. They're the registry
pins of AICR v0.20.0 against the EKS `h100` training recipe of v1.3.0. The
function applies them and raises a Warning for each.

| Component | Move | Verdict |
|---|---|---|
| `gpu-operator` | v26.3.3 to v26.7.2 | blocked: no record starts from v26.3.3 |
| `nvidia-dra-driver-gpu` | 0.4.1 to 0.5.1 | blocked: no record starts from 0.4.1 |
| `nvsentinel` | v1.20.0 to v1.31.0 | blocked: no record starts from v1.20.0 |
| `kai-scheduler` | v0.14.1 to v0.16.9 | unknown |
| `kube-prometheus-stack` | 84.4.0 to 84.5.0 | manual |
| `nfd` | 0.19.0 to 0.20.1 | manual |
| `nodewright-operator` | v0.17.1 to v0.21.0 | manual |

Other changes to plan for:

- `nodewright-operator` moves its default namespace from `skyhook` to
  `nodewright`, and the kind of its tuning resource changes from `Skyhook` to
  `NodeWright`. The function keeps the operator in `skyhook` on an existing
  cluster. The `Object` for the tuning resource gets a new name, so the old
  one is deleted and the new one created.
- The stock recipes use NKTP for telemetry where they used the full
  kube-prometheus-stack.
- `cert-manager` enables its ServiceMonitor and depends on
  `prometheus-operator-crds`. If you skip that component, the CRDs must come
  from somewhere else.
- A [recipe source](recipe-source.md) artifact written for an AICR older than
  v1.0.0 is refused at startup.
