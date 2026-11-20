# Input reference

The function's input is an object of `kind: Input`,
`apiVersion: aicr.fn.crossplane.io/v1beta1`. Its authoritative source is
[`input/v1beta1/input.go`][input-go], and the generated CRD is
[`package/input/aicr.fn.crossplane.io_inputs.yaml`][input-crd].

Every field is static unless the overlay sets it. See
[Values from the composite resource](../authors/overlay.md).

## Where values come from

| Field | Type | Default | Notes |
|---|---|---|---|
| `overlayFrom` | object | unset | Where an overlay of this `Input` is read from. |
| `overlayFrom.fieldPath` | string, required when `overlayFrom` is set | | A field path on the observed composite resource, such as `spec.aicr`, to an object with the fields of this `Input`. |

## The recipe

| Field | Type | Default | Notes |
|---|---|---|---|
| `criteria` | object | | AICR recipe criteria. At least one dimension must be set, here or by the overlay. |
| `criteria.accelerator` | string | unstated | One of AICR's accelerators, such as `h100`, `b200` or `a100`. |
| `criteria.service` | string | unstated | One of AICR's services, such as `eks`, `gke`, `aks` or `kind`. |
| `criteria.intent` | string | unstated | `training` or `inference`. |
| `criteria.os` | string | unstated | One of AICR's operating systems, such as `ubuntu`, `cos` or `talos`. Unstated, AICR selects the OS-agnostic recipe tier. AICR rejects an OS no recipe for the service honors, and asks for one when a service and accelerator pair has only OS-specific recipes. |
| `criteria.platform` | string | unstated | One of AICR's platforms, such as `kubeflow`, `slurm`, `runai`, `dynamo` or `nim`. |
| `profile` | map of string to string | unset | One recipe variant, keyed by profile name, such as `{gpuStack: dra}`. See [Recipe variants and configuration](../authors/variants.md). |
| `configuration.gke.tcpxoInterfaces` | list of `{interfaceName, network}` | unset | The eight `eth1` to `eth8` GKE Network mappings a recipe with the `torch-distributed-tcpxo` runtime requires. |
| `configuration.runtimeInventory.mode` | string | unset | `enabled` adds `k8s-aibom` and `disabled` removes it. |

## The target cluster

| Field | Type | Default | Notes |
|---|---|---|---|
| `providerConfigRef.kind` | string | `ClusterProviderConfig` | `ClusterProviderConfig` or `ProviderConfig`. A kind the overlay sets must be one of the two as well. |
| `providerConfigRef.name` | string, required | | Stamped onto every composed `Release` and `Object`. From the `Input` or its overlay, so one Composition can target the cluster each composite resource names. |

## Components

| Field | Type | Default | Notes |
|---|---|---|---|
| `componentClasses` | list of string | every class | Limits the recipe to the components of the listed AICR classes, `core` or `ops`. Any other class is Fatal. |
| `skipComponents` | list of `{name}` | none | Components left out of the recipe. A name that matches no component raises a Warning. |
| `componentOverrides` | map of component name to override | none | An entry for a component that isn't deployed is ignored with a Warning. |
| `componentOverrides.<name>.version` | string | the recipe's | The chart version. |
| `componentOverrides.<name>.namespace` | string | the recipe's, or the one the component is already deployed in | The target namespace. Also how a deployed component is moved to the namespace a newer recipe names. |
| `componentOverrides.<name>.values` | object | unset | Deep-merged over the values the recipe resolved. On a conflict the override's value is used. Anything but an object is Fatal on the first pass. |
| `draEviction` | bool | `false` | Wires AICR's eviction contract across three components. See [DRA eviction](dra-eviction.md). |

See [Components and overrides](../authors/components.md).

## Rollout and status

| Field | Type | Default | Notes |
|---|---|---|---|
| `skipDeploymentOrder` | bool | `false` | Emits every component at once instead of withholding each until its dependencies report `Ready`. See [Rollout order](rollout-order.md). |
| `target.fieldPath` | string, required when `target` is set | | Where the [status summary](../authors/summary.md) is written. Must be `status` or begin with `status.`. |

[input-go]: https://github.com/jonasz-lasut/function-aicr/blob/main/input/v1beta1/input.go
[input-crd]: https://github.com/jonasz-lasut/function-aicr/blob/main/package/input/aicr.fn.crossplane.io_inputs.yaml
