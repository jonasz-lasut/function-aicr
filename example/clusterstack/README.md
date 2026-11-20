# ClusterStack example

An XRD, a Composition and two composite resources for `crossplane render`.
The XRD declares, under `spec.aicr`, the `Input` fields a `ClusterStack` may
set. The Composition's `overlayFrom` merges that object over its static
`Input`.

[Render locally][render-locally] in the documentation walks through it.

## Render

Run the function, then render:

```shell
go run . --insecure --debug
```

```shell
crossplane render xr.yaml composition.yaml functions.yaml \
  --required-schemas=schemas/ \
  --include-function-results
```

The first render has no observed resources, so only the components without
dependencies appear. Rerun with `--observed-resources` and
`prometheus-operator-crds` marked `Ready` to see `cert-manager` emitted.

`xr-eks.yaml` selects an EKS stack whose manifest-only
`nodewright-customizations` component renders its NodeWright tuning resource
into a provider-kubernetes `Object`:

```shell
crossplane render xr-eks.yaml composition.yaml functions.yaml \
  --required-schemas=schemas/ \
  --include-function-results
```

## The files under `schemas/`

`helm.m.crossplane.io_v1beta1.json` and
`kubernetes.m.crossplane.io_v1alpha1.json` are hand-written stand-ins for
provider-helm's `Release` and provider-kubernetes' `Object` CRD schemas, not
copies of them. They cover only the fields the function sets:

- for the `Release`, `spec.forProvider.chart.{name,repository,version}`,
  `spec.forProvider.namespace`, `spec.forProvider.wait` and
  `spec.forProvider.values`;
- for the `Object`, `spec.forProvider.manifest`;
- for both, `spec.providerConfigRef.{kind,name}`.

Each file is an OpenAPI v3 document, the format
`kubectl get --raw /openapi/v3/<group-version>` returns and
`--required-schemas` reads. Crossplane matches a schema to a kind by its
`x-kubernetes-group-version-kind` extension.

To replace the stand-ins with the real schemas, fetch them from a cluster
that has the providers:

```shell
mkdir -p schemas
kubectl get --raw '/openapi/v3/apis/helm.m.crossplane.io/v1beta1' \
  > schemas/helm.m.crossplane.io_v1beta1.json
kubectl get --raw '/openapi/v3/apis/kubernetes.m.crossplane.io/v1alpha1' \
  > schemas/kubernetes.m.crossplane.io_v1alpha1.json
```

[render-locally]: https://jonasz-lasut.github.io/function-aicr/authors/render-locally.html
