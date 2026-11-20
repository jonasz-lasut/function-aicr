# Node scheduling

A cluster for AI workloads often has its accelerated nodes in a pool of
their own, set apart from the other nodes by labels and taints.
`nodeScheduling` places a recipe's components on those pools:

```yaml
nodeScheduling:
  system:
    nodeSelector:
      nodeGroup: system-pool
    tolerations:
    - key: dedicated
      operator: Equal
      value: system
      effect: NoSchedule
  accelerated:
    nodeSelector:
      nodeGroup: gpu-pool
    tolerations:
    - key: nvidia.com/gpu
      operator: Equal
      value: present
      effect: NoSchedule
```

AICR's component registry lists, for each component, the chart values that
take a node selector or tolerations. It lists them in two groups: `system`
for operators, controllers and webhooks, and `accelerated` for what runs on
the accelerated nodes, such as device plugins, drivers and exporters. The
function writes each placement to the values of its group.

These are the values that AICR's `bundle` command writes its
`--system-node-selector`, `--system-node-toleration`,
`--accelerated-node-selector` and `--accelerated-node-toleration` flags to,
by the same rules.

## Merge rules

A placement replaces what the component's values have at a path. A recipe
can decide otherwise with an override of its own at that path:

| The recipe's override at the path | Node selector | Tolerations |
|---|---|---|
| None | Replaced | Replaced |
| A list of tolerations | | Appended to |
| Empty, as in `tolerations: []` | Left alone | Left alone |

The kind recipes set the GPU Operator's daemon set tolerations to an empty
list, and so keep the chart's default there whatever `nodeScheduling` says.

A path the registry lists in both groups belongs to a workload that runs on
every node, such as the worker of Node Feature Discovery. It gets both lists
of tolerations.

`componentOverrides.<component>.values` are merged afterward. Use them to
place one component differently: a list of tolerations there replaces what
was written, and a node selector is merged into it.

## The function has no default placement

AICR's command line tolerates every taint when its toleration flags are
unset. The function writes only what the `Input` states, so an upgrade of
the function doesn't reschedule a running stack. To get what the command
line does, state the toleration:

```yaml
nodeScheduling:
  system:
    tolerations:
    - operator: Exists
  accelerated:
    tolerations:
    - operator: Exists
```

## Components that require a node selector

A registry entry can require a node selector. AICR requires one where the
wrong pool breaks the component without an error, as with a StatefulSet
whose volume is in one zone. AICR's bundler refuses to build a bundle
without the selector, and the function reports Fatal:

```text
component "slinky-slurm" requires nodeScheduling.system.nodeSelector: its values have no node selector at …
```

In AICR v1.3.0 the entries of `slinky-slurm`, `slurm-accounting-mariadb` and
`nvsentinel-mongodb` require one. Of the embedded recipes, that's the six
Slurm recipes for AKS, EKS and GKE.

The check reads the values the component is deployed with, so a selector
from `componentOverrides` satisfies it too. A recipe that sets the path to
an empty value has decided to deploy without one, and the check lets it.
