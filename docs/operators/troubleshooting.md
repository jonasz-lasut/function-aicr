# Troubleshooting

Each entry is a message the function reports, or a symptom, with its cause.
The function reports through events on the composite resource; with
`crossplane render`, pass `--include-function-results` to see them.

## Fatal results

### `function-aicr requires a Crossplane that supports required schemas`

Crossplane didn't advertise the required-schemas capability. Upgrade
Crossplane to v2.2 or later, or pass `--required-schemas=DIR` to
`crossplane render`.

### `no schema for …, Kind=Release: is provider-helm installed?`

Crossplane didn't find a schema. The provider whose CRD has that kind isn't
installed, or the files under `--required-schemas` have no schema for
it. The same message names provider-kubernetes for `Kind=Object`.

### `criteriaFrom was removed` and `providerConfigRefFrom was removed`

The Composition was written for an older `Input`. Move the values under one
object on the composite resource and set `overlayFrom.fieldPath`. See
[Values from the composite resource](../authors/overlay.md).

### `cannot resolve the function input: invalid Input`

The `Input`, or the overlay merged into it, has a field the `Input` doesn't
have. The message quotes the field. Check the spelling against the
[Input reference](input-reference.md).

### `invalid AICR recipe criteria`

A criteria value isn't one AICR knows, such as an accelerator that doesn't
exist. The message quotes the value. Correct it.

### `cannot resolve AICR recipe: … no recipe provides os 'ubuntu' for criteria(…)`

Each value is valid, but AICR's catalog has no recipe for the combination.
Leave the dimension the message quotes unstated, in the `Input` and in the
overlay.

### `no criteria resolved`

The `Input` and its overlay state no criteria dimension. With `overlayFrom`
set, check that the field path exists on the composite resource. When the
path is absent, the function uses the static `Input` alone.

### `providerConfigRef.name is required`

Neither the `Input` nor the overlay sets `providerConfigRef.name`.

### `the overlay at "…" must not set …`

The object on the composite resource sets `overlayFrom`, `apiVersion`, `kind`
or `metadata`. Remove the field: the overlay holds the `Input`'s own fields
only.

### `invalid Release for "…"` and `invalid Object "…"`

A composed resource failed the provider's CRD schema. With an override, check
its values. Without one, the installed provider is likely older than the
fields the function sets.

### `component "…" requires nodeScheduling.system.nodeSelector`

AICR's registry requires a node selector for the component, and its values
have none. Set the selector the message names. See
[Node scheduling](node-scheduling.md#components-that-require-a-node-selector).
For a component that's already deployed the same message is a Warning.

### `AICR's bundle check for "…"`

AICR would refuse to build a bundle for this request, and the message is
AICR's reason. It's Fatal for a component that isn't deployed yet and a
Warning otherwise. See [AICR's bundle checks](bundle-checks.md) for what the
checks read from the `Input`.

### `cannot resolve the node scheduling`

A node selector under `nodeScheduling` isn't made of valid labels, or a
toleration is one the API server would refuse. The message quotes it.

### `this recipe ships torch-distributed-tcpxo and requires configuration.gke.tcpxoInterfaces`

The `h100` / `gke` / `cos` / `training` / `kubeflow` recipe needs the
cluster's GPU network names. Set `configuration.gke.tcpxoInterfaces`. See
[Recipe variants and configuration](../authors/variants.md).

### `componentClasses names "…", which is not an AICR component class`

The classes are `core` and `ops`.

## Warnings

### `skipComponents names "…", which is not a component of the resolved recipe`

The name matches no component, so nothing was skipped. Check the spelling
against `deployedComponents` and `pendingComponents` in the status summary.

### `componentOverrides["…"] names no managed component of the resolved recipe`

The override is ignored. The component is misspelled, skipped, left out by
`componentClasses` or turned off by the recipe.

### `"…" stays in namespace "…", where it is deployed`

The recipe now places the component in another namespace, and the function
doesn't move a live `Release`. Nothing is wrong. To move the component,
set the override the message names, and read
[what a move leaves behind](../authors/components.md#a-deployed-component-keeps-its-namespace)
first.

### `upgrading "…" from … to … is rated … by AICR`

AICR's upgrade records don't rate the chart version change safe. The change
is applied anyway. Read the explanation in the message, and see
[Upgrades](upgrades.md).

### `draEviction is set, but "…" is not a managed component`

The eviction contract spans `gpu-operator`, `nvidia-dra-driver-gpu` and
`dra-node-labeler`. One of them is skipped, turned off by the recipe or left
out by `componentClasses`, so the function doesn't wire any of it.

## Symptoms

### No resources are composed, or only a few components appear

The [gate](rollout-order.md) is withholding the rest. Read the Normal results
(`delaying X until dependency Y is ready`) or `pendingComponents` in the
status summary.

### The rollout advances one step every 30 seconds

The composite resource has the condition `Responsive` at `False` with the
reason `WatchCircuitOpen`. Crossplane's
[circuit breaker](rollout-order.md#crossplanes-circuit-breaker) is open, and
the rollout continues at its pace.

### `delaying X until dependency Y is ready` never goes away

Y isn't reaching `Ready` in the target cluster. Look at its `Release` or
`Object` and at the provider's events for it.

### The function pod restarts with `cannot pull the OCI recipe source`

The function exits when the [recipe source](recipe-source.md) can't be pulled
or validated. It never falls back to the embedded catalog.

| The error mentions | Cause |
|---|---|
| authentication | The Docker config (`DOCKER_CONFIG`) has no credentials for the registry. |
| a certificate | The registry's CA isn't in the trust store (`SSL_CERT_FILE`). |
| `invalid materialized OCI recipe catalog` | The artifact isn't a recipe tree with `registry.yaml` at its root. |
| the `apiVersion` of the external `registry.yaml` | The artifact was written for an AICR older than v1.0.0. |
| `unsafe archive path component` | The layer was packed from a directory by `oras`. See [Opt in to a mixin](mixins.md). |

### The function pod restarts with `--recipe-oci-digest must be an immutable sha256 manifest digest`

The flags are checked before anything is pulled. `--recipe-oci-digest` must
be the artifact's `sha256:` manifest digest. A tag is refused. Set
`--recipe-oci-repository` with it.

### The summary doesn't show up in the status

The XRD doesn't declare a schema for `target.fieldPath`, and Crossplane
prunes undeclared status fields. Add the subschema from
`example/clusterstack/definition.yaml`.
