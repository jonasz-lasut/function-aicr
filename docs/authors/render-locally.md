# Render locally

`crossplane render` runs the pipeline on your machine, without a cluster. The
repository's [`example/clusterstack`][example] directory has the files it
needs.

| File | Contents |
|---|---|
| `xr.yaml` | An `h100` / `kind` / `training` stack. |
| `xr-eks.yaml` | An EKS stack whose manifest-only `nodewright-customizations` component renders its NodeWright tuning resource into an `Object`. |
| `composition.yaml`, `definition.yaml` | The Composition and XRD from [Write a Composition](composition.md). |
| `functions.yaml` | The pipeline's functions. function-aicr uses the Development runtime, so render talks to the one you run. |
| `schemas/` | Stand-ins for the provider CRD schemas. |

Run the function, then render:

```shell
go run . --insecure --debug
```

```shell
cd example/clusterstack
crossplane render xr.yaml composition.yaml functions.yaml \
  --required-schemas=schemas/ \
  --include-function-results
```

The function checks composed resources against the provider schemas, so
render must be given those schemas.

## The first render

The first render has no observed resources, so only the components without
dependencies appear: the [rollout order](../operators/rollout-order.md)
withholds the rest. Pass `--observed-resources` with a file that marks
`prometheus-operator-crds` `Ready` to see `cert-manager` emitted:

```yaml
apiVersion: helm.m.crossplane.io/v1beta1
kind: Release
metadata:
  name: prometheus-operator-crds
  namespace: default
  annotations:
    crossplane.io/composition-resource-name: prometheus-operator-crds
spec:
  forProvider:
    namespace: monitoring
status:
  conditions:
  - type: Ready
    status: "True"
    reason: Available
    lastTransitionTime: "2026-11-20T00:00:00Z"
```

## The schemas

The files under `schemas/` are hand-written stand-ins for provider-helm's
`Release` and provider-kubernetes' `Object` CRD schemas, not copies of them.
They cover only the fields the function sets. Each is an OpenAPI v3 document,
the format `kubectl get --raw /openapi/v3/<group-version>` returns, which is
what `--required-schemas` reads.

To render against the real schemas, fetch them from a cluster where the
providers are installed:

```shell
mkdir -p schemas
kubectl get --raw '/openapi/v3/apis/helm.m.crossplane.io/v1beta1' \
  > schemas/helm.m.crossplane.io_v1beta1.json
kubectl get --raw '/openapi/v3/apis/kubernetes.m.crossplane.io/v1alpha1' \
  > schemas/kubernetes.m.crossplane.io_v1alpha1.json
```

[example]: https://github.com/jonasz-lasut/function-aicr/tree/main/example/clusterstack
