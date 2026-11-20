# The status summary

When `target.fieldPath` is set, the function writes a summary of the resolved
recipe and of the rollout to that path on the composite resource. This is the
[example](render-locally.md) composite resource once
`prometheus-operator-crds` is observed `Ready`:

```yaml
status:
  aicr:
    recipeName: h100-kind-training
    recipeVersion: v1.3.0
    appliedOverlays:
    - base
    - monitoring-hpa
    - h100-any
    - kind
    - h100-kind-training
    componentCount: 13
    deployedComponents:
    - name: nodewright-operator
      namespace: nodewright
      version: v0.21.0
    - name: prometheus-operator-crds
      namespace: monitoring
      version: 28.0.1
    # cert-manager, kube-prometheus-stack and nfd follow
    pendingComponents:
    - name: gpu-operator
      waitingFor: nfd
    - name: kai-scheduler
      waitingFor: gpu-operator
    # six more follow
```

| Key | Meaning |
|---|---|
| `recipeName` | The recipe's identifier, built from the criteria. |
| `recipeVersion` | The version of AICR's Go module, `aicr.run`, that the function was built with. It pins the embedded recipe data. |
| `recipeSource` | The `repository@digest` of the [recipe source](../operators/recipe-source.md). Present only when the function serves one. |
| `profile` | The [recipe variant](variants.md) deployed, as `name=value`. It's the selected one or the recipe's default, and it's absent when the recipe doesn't declare a profile. |
| `appliedOverlays` | The catalog overlays the recipe was built from, in order. An overlay a recipe source adds shows up here. |
| `componentCount` | How many components the recipe deploys for this composite resource. A component that composes no resource isn't counted. |
| `deployedComponents` | The components whose `Release` is in the desired state, in AICR's deployment order, each with its chart version and namespace. A manifest-only component is listed once all its `Object`s are. |
| `pendingComponents` | The components the [rollout order](../operators/rollout-order.md) still withholds, each with the dependency or pre-manifest `Object` it waits for. Empty once everything is emitted. |

The path must be `status` or begin with `status.`. The keys are set one by
one, and keys already present at the path are kept.

Crossplane prunes any status field the XRD doesn't declare. The XRD needs a
matching subschema. [`example/clusterstack/definition.yaml`][definition]
has one to copy.

[definition]: https://github.com/jonasz-lasut/function-aicr/blob/main/example/clusterstack/definition.yaml
