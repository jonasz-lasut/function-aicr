# Write a Composition

Use function-aicr as a step in a Composition pipeline. Its `Input` names the
recipe criteria and the provider config of the target cluster. A value is
either static in the Composition or set by the composite resource, through
the object `overlayFrom` names.

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: clusterstacks.example.org
spec:
  compositeTypeRef:
    apiVersion: example.org/v1alpha1
    kind: ClusterStack
  mode: Pipeline
  pipeline:
  - step: aicr
    functionRef:
      name: function-aicr
    input:
      apiVersion: aicr.fn.crossplane.io/v1beta1
      kind: Input
      overlayFrom:
        fieldPath: spec.aicr
      providerConfigRef:
        kind: ClusterProviderConfig
        name: gpu-cluster
      target:
        fieldPath: status.aicr
  - step: auto-ready
    functionRef:
      name: function-auto-ready
```

A composite resource of that kind then picks its own recipe:

```yaml
apiVersion: example.org/v1alpha1
kind: ClusterStack
metadata:
  name: gpu-training-stack
  namespace: default
spec:
  aicr:
    criteria:
      accelerator: h100
      service: kind
      intent: training
```

The object at `spec.aicr` has the fields of the `Input` and is merged over
it. A composite resource controls the `Input` fields its XRD declares there,
and the function doesn't need to know the composite resource's kind.
[Values from the composite resource](overlay.md) has the merge rules.

## The XRD

Crossplane prunes anything an XRD doesn't declare. The schema here declares
the `Input` fields a composite resource may set under the overlay path, and
the complete XRD also declares the summary under the `target` path.

```yaml
spec:
  type: object
  required:
  - aicr
  properties:
    aicr:
      type: object
      required:
      - criteria
      properties:
        criteria:
          type: object
          properties:
            accelerator:
              type: string
            service:
              type: string
            intent:
              type: string
            os:
              type: string
            platform:
              type: string
```

The complete XRD is in
[`example/clusterstack/definition.yaml`][definition]. It also declares
`profile`, `configuration` and `nodeScheduling` under the overlay path, and
the status schema.

## One composite resource per cluster and namespace

A composed resource is named after its component, because the rollout order
is worked out by matching observed resources to desired ones by name.
Composite resources in one namespace that deploy the same component would
claim the same name. A recipe is one stack per cluster in any case: its Helm
release names would collide in the target cluster.

[definition]: https://github.com/jonasz-lasut/function-aicr/blob/main/example/clusterstack/definition.yaml
