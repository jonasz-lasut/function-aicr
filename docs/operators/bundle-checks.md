# AICR's bundle checks

AICR's component registry declares checks for some of its components, and
AICR's bundler runs them before it writes a bundle. They guard combinations
that install cleanly and then break a cluster. The function runs the same
checks, with AICR's own code, on every request.

Two of them matter for the stock recipes of AICR v1.3.0:

- On AKS, a toleration without a key deadlocks the first installation of
  NodeWright's packages. The tuning manifest renders one when
  `nodeScheduling.accelerated.tolerations` is empty. The four AKS recipes
  need tolerations that name a key.
- On OKE, the `gb300` tuning resource reboots every node it matches, so that
  recipe needs `nodeScheduling.accelerated.nodeSelector`.

The 81 stock recipes of that version all pass the checks once both
placements are stated, with keyed tolerations.

AICR limits a check like these to a service or an accelerator. Such a check
also applies to a recipe whose criteria leave that dimension unstated: a
recipe without a service could be deployed to AKS.

## Findings

AICR rates a check `error`, `warning` or `info`.

| AICR's rating | The component isn't deployed yet | The component is already deployed |
|---|---|---|
| `error` | Fatal: the function composes nothing | Warning |
| `warning` | Warning | Warning |
| `info` | Not reported | Not reported |

A component that's already deployed isn't refused, because its stack was
accepted before, by an earlier version of the function or of the catalog.
Refusing it would stop every other change to the stack until the finding is
resolved, which for a running workload can need more care than an upgrade
allows. The Warning ends in `is already deployed, so it is not refused`.

The same rule applies to a component that
[requires a node selector](node-scheduling.md#components-that-require-a-node-selector).

A finding reads like this:

```text
AICR's bundle check for "nodewright-customizations": [INVALID_REQUEST] nodewright-customizations renders a wildcard (keyless) accelerated-node toleration. AKS admission collapses a pod's tolerations to just the wildcard, which deadlocks nodewright packages that declare interrupts on first install (NVIDIA/nodewright#296). Pass keyed tolerations instead, e.g. nodeScheduling.accelerated.tolerations nvidia.com/gpu:NoSchedule.
```

## The request as the checks see it

The checks were written for AICR's command line. The function describes a
request to them in its terms:

| AICR's command line | The `Input` |
|---|---|
| `--system-node-selector`, `--system-node-toleration`, `--accelerated-node-selector`, `--accelerated-node-toleration` | `nodeScheduling` |
| `--set <component>:enabled=false` | `skipComponents` |
| `--set-json <component>:<path>=<value>` | `componentOverrides.<component>.values` |
| `--dra-eviction-node-label` | `draEviction` |

Where AICR's text has one of the four scheduling flags, the message has the
`Input` field. Any other flag appears as AICR wrote it, and the table says
what stands for it.

The function checks the components that the composite resource deploys. One
that `componentClasses` leaves to another composite resource is checked
there.
