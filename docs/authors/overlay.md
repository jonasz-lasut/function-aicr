# Values from the composite resource

Every field of the `Input` is static: the value all composite resources of a
Composition get. `overlayFrom.fieldPath` points at an object on the composite
resource, with the fields of the `Input`, that's merged over it. One
Composition then serves many clusters, and each composite resource states its
own recipe and target.

```yaml
# In the Composition
input:
  apiVersion: aicr.fn.crossplane.io/v1beta1
  kind: Input
  overlayFrom:
    fieldPath: spec.aicr
  criteria:
    intent: training
  providerConfigRef:
    kind: ClusterProviderConfig
```

```yaml
# On the composite resource
spec:
  aicr:
    criteria:
      accelerator: h100
      service: eks
    providerConfigRef:
      name: workload-a
```

This composite resource is reconciled with `accelerator: h100`,
`service: eks`, the static `intent: training`, and the `ClusterProviderConfig`
named `workload-a`.

## The merge rules

1. Maps merge key by key, and the overlay's value replaces the static one. A
   static key the overlay doesn't set is kept.
2. A list in the overlay replaces the static list whole.
3. An empty string or a null in the overlay counts as unset and keeps the
   static value. Kubernetes API conventions make `""` and an omitted optional
   field indistinguishable. Chart values are the exception: under
   `componentOverrides.<component>.values` an empty string or a null is a
   value like any other.
4. A field path that's absent from the composite resource leaves the static
   `Input` unchanged.
5. A field path that holds anything but an object, an overlay that sets
   `overlayFrom`, `apiVersion`, `kind` or `metadata`, and a field the `Input`
   doesn't have are Fatal, never a silent fallback. An unknown field in the
   static `Input` is Fatal too.

## Fields the overlay may set

Any field of the [Input reference](../operators/input-reference.md) but
`overlayFrom`: `criteria`, `providerConfigRef`, `profile`, `configuration`,
`skipComponents`, `componentOverrides` and the rest. The overlay is the
`Input`'s fields alone, without the `apiVersion`, `kind` and `metadata` of a
whole `Input` object.

The XRD limits it. Crossplane prunes the fields an XRD doesn't declare, so
declare under the overlay path exactly the fields a composite resource should
control. A value that only one cluster knows, such as the
[GKE network names](variants.md), belongs there. A value meant for all
clusters belongs in the Composition.

## Criteria

You can leave any criteria dimension out, but you must set at least one. An
`Input` with no criteria, as when its overlay misses the composite resource,
is Fatal instead of a silent deployment of the base recipe.

The function passes an unset dimension to AICR unstated, and AICR then
resolves a recipe that doesn't depend on it. AICR's catalog alone decides which
combinations exist, and AICR rejects a stated value that isn't part of one.
