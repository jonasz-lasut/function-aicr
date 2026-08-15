# ClusterStack example

Demonstrates `function-aicr` serving an XR it knows nothing about: the criteria
live on `spec` under names of the XRD author's choosing and reach the function
through `criteriaFrom`.

The function validates every composed resource against the provider CRD schema
Crossplane resolves for it, so `render` must be given those schemas.

Run the function locally, then render:

```shell
go run . --insecure --debug
```

```shell
crossplane render xr.yaml composition.yaml functions.yaml \
  --required-schemas=schemas/ \
  --include-function-results
```

Gating is on and nothing is observed, so only the dependency-free components
appear. That is correct, not a bug: rerun with `--observed-resources` marking
`cert-manager` Ready to see `gpu-operator` emitted.

`xr-eks.yaml` selects an EKS tier that also carries manifest files: the
manifest-only `nodewright-customizations` component renders its Skyhook tuning
CR into a provider-kubernetes `Object`, so the function requires the `Object`
schema as well and `schemas/` supplies it:

```shell
crossplane render xr-eks.yaml composition.yaml functions.yaml \
  --required-schemas=schemas/ \
  --include-function-results
```

## About the files under `schemas/`

`helm.m.crossplane.io_v1beta1.json` and `kubernetes.m.crossplane.io_v1alpha1.json`
are **hand-written stand-ins** for provider-helm's `Release` and
provider-kubernetes' `Object` CRD schemas, not copies of them. They cover only
the fields this function sets (`spec.forProvider.chart.{name,repository,version}`,
`spec.forProvider.namespace`, `spec.forProvider.wait`, `spec.forProvider.values`
for the Release; `spec.forProvider.manifest` for the Object;
`spec.providerConfigRef.{kind,name}` for both), and are missing everything else
the real CRDs declare.

Their structure — a full OpenAPI v3 document under `components.schemas`, with
each schema tagged `x-kubernetes-group-version-kind: [{group, version, kind}]`
so it can be matched against the `helm.m.crossplane.io/v1beta1` and
`kubernetes.m.crossplane.io/v1alpha1` GVKs — was verified by reading the actual
code `crossplane render` and the Crossplane core reconciler both use to resolve
required schemas:

- `github.com/crossplane/cli/v2@v2.3.0`'s
  `cmd/crossplane/render/schemas.go` (`LoadRequiredSchemas`) parses every
  `.json` file under `--required-schemas=DIR` as a `spec3.OpenAPI` document —
  exactly what `kubectl get --raw /openapi/v3/<group-version>` returns.
- `github.com/crossplane/crossplane/v2`'s `internal/render/schemas.go`
  (`NewInMemoryOpenAPIClient`) and `internal/xfn/required_schemas.go`
  (`OpenAPISchemasFetcher.Fetch`) then walk `components.schemas`, matching by
  the `x-kubernetes-group-version-kind` extension, and hand the function the
  single matched schema object.

To replace them with the real schemas against a cluster where the providers
are installed:

```shell
mkdir -p schemas
kubectl get --raw '/openapi/v3/apis/helm.m.crossplane.io/v1beta1' \
  > schemas/helm.m.crossplane.io_v1beta1.json
kubectl get --raw '/openapi/v3/apis/kubernetes.m.crossplane.io/v1alpha1' \
  > schemas/kubernetes.m.crossplane.io_v1alpha1.json
```
