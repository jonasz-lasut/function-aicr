# End-to-end suite

The suite runs function-aicr the way a control plane does. The function is
built from this tree and installed as a Function package by Crossplane in a
kind cluster, where provider-helm and provider-kubernetes compose into the
cluster itself.

The suite deploys a catalog of its own, `catalog/`: three small components
with one chart that starts a pause container. The catalog is published to a
registry in the cluster and given to the function as its OCI recipe source.
Besides the packages and the registry's image, a run pulls nothing from
outside the cluster.

It follows [the suite of `crossplane/crossplane`][crossplane-e2e]. Features
are built with [`sigs.k8s.io/e2e-framework`][e2e-framework], run through
Crossplane's e2e environment (`test/e2e/config`) and assembled from its
helpers (`test/e2e/funcs`). Each feature keeps its manifests in a directory
of its own under `manifests/`. The suite is a Go module of its own, so
Crossplane's dependency tree doesn't enter the function's `go.mod`.

## Run it

Docker has to be running. Each profile has a kind cluster of its own,
`function-aicr-e2e-catalog` or `function-aicr-e2e-kwok`, and the Makefile
keeps that cluster's kubeconfig in a file of its own. The targets and the
suite read that file alone, so they can't reach another cluster. To use
`kubectl` on the cluster yourself:

```shell
export KUBECONFIG=$PWD/_output/e2e/kubeconfig-catalog
```

From the repository root:

```shell
git submodule update --init   # the Crossplane build submodule, once
make e2e.up                   # kind, Crossplane, the providers, the registry, the function
make e2e.test                 # the suite
make e2e.function             # after a code change: rebuild and reinstall the function
make e2e.catalog              # after a change to the catalog: publish it again
make e2e.down
```

`make e2e` does up, test and down in one go. After a failed run,
`make e2e.dump` writes the cluster's state under `_output/controlplane-dump`.

One feature runs with Go's own filter:

```shell
make e2e.test E2E_TEST_FLAGS='-v -count=1 -run TestRolloutOrder'
```

## The cluster

`make e2e.up` provisions it with the make library of the Crossplane `build`
submodule (pinned `kind`, `kubectl`, `helm` and `crossplane` under
`.cache/tools`):

| Piece | Where it comes from |
|---|---|
| kind and Crossplane | `controlplane.up`, at `CROSSPLANE_VERSION` and with `CROSSPLANE_ARGS` |
| An OCI registry at `registry.e2e.svc:5443` | `manifests/setup/registry.yaml`, behind TLS from a CA `make e2e.certs` creates under `_output/e2e/certs` |
| provider-helm, provider-kubernetes, `function-auto-ready` | `manifests/setup/packages.yaml`, at the versions the Makefile pins. provider-helm polls every 30 seconds. |
| A `ClusterProviderConfig` named `e2e` for each provider | `manifests/setup/provider-configs.yaml`. Each provider uses its own service account, bound to `cluster-admin`, which makes the kind cluster the target cluster. |
| The charts and the recipe catalog | `make e2e.catalog`. It pushes `catalog/charts` with `helm push` and `catalog/recipe` with `oras`, as [the recipe source page][recipe-source] tells an operator to. |
| function-aicr | `make e2e.function`. It builds the image from the Dockerfile and a package from the image, and installs the package from Crossplane's cache with `packagePullPolicy: Never`. |

The function runs with `manifests/setup/function-runtime.yaml`. That turns
on debug logging and sets the catalog's repository and digest. It also puts
the registry's CA in the trust store, because AICR pulls an OCI recipe
source over HTTPS only. provider-helm has the same CA for the charts.

Crossplane runs with `--circuit-breaker-burst=1000`. With the default of 100
a rollout opens the circuit breaker of its composite resource within
seconds, and each further step then waits up to 30 seconds.

## The catalog

AICR layers the catalog over the one embedded in the function, so
`catalog/recipe` replaces the stock overlays that its criteria would
otherwise also select: `base`, `kind` and `monitoring-hpa`. Only the suite's
own components remain.

| Component | Composed from |
|---|---|
| `e2e-first` | A chart without dependencies. |
| `e2e-second` | A chart that depends on `e2e-first`, with one manifest file before its release and one after. |
| `e2e-notes` | A manifest-only component that depends on `e2e-second`. |
| `e2e-checked` | A chart whose registry entry declares one of AICR's bundle checks. Only the inference recipe has it. |

The criteria `service: kind` select the recipe of the first three. Adding
`intent: training` selects one that places `e2e-first` in another namespace,
and `intent: inference` one that adds `e2e-checked`. `TestEndToEndCatalog`
at the repository root resolves the three recipes and fails when an AICR
upgrade adds a stock overlay that would join them.

## The features

| Feature | Proves |
|---|---|
| `TestFunctionInstalled` | The function, the providers and `function-auto-ready` report `Healthy`. |
| `TestRecipeConverges` | A composite resource deploys its recipe, with charts from the registry and manifest files around them, and the summary describes it. |
| `TestRolloutOrder` | A component is withheld until the component it depends on is ready. |
| `TestRolloutSteps` | Within a component, the `Release` waits for the pre-manifest `Object`s and the post-manifest `Object`s wait for the `Release`. |
| `TestLiveResourcesAreKept` | A deployed component that goes unready doesn't take what depends on it out of the cluster. |
| `TestSkipDeploymentOrder` | `skipDeploymentOrder` composes everything at once. |
| `TestSkippedDependency` | A skipped dependency counts as satisfied. |
| `TestOverlay` | A change to the composite resource upgrades a release and removes a skipped component. |
| `TestKeptNamespace` | A deployed component keeps its namespace when the recipe moves it, and an override moves it. |
| `TestNodeScheduling` | `nodeScheduling` reaches the pods. |
| `TestBundleChecks` | A recipe AICR would refuse to bundle isn't deployed, and a stack that's already deployed isn't refused. |
| `TestStockRecipeOnSimulatedNodes` | The `kwok` profile's feature, described in the next section. |

A feature makes a component wait without touching what the function
composed. A chart's pod is pending on a node selector until the feature
gives a node the label, and a quota keeps a pre-manifest ConfigMap from
being created until the feature deletes it. One state can't be produced that
way: provider-helm keeps a `Release` ready after its pods stop, so
`TestLiveResourcesAreKept` pauses the `Release` and sets its condition
itself.

## The KWOK profile

The suite's own catalog can't show whether a stock recipe deploys. The
`kwok` profile does that on a cluster of its own, the way
[AICR's KWOK lane][aicr-kwok] does. [KWOK][kwok] adds nodes that have no
kubelet and reports a pod scheduled on one as running, without starting a
container.

```shell
make e2e.up E2E_PROFILE=kwok
make e2e.test E2E_PROFILE=kwok
make e2e.down
```

`make e2e.kwok` applies the KWOK release at `KWOK_VERSION` and the nodes in
`manifests/kwok/nodes.yaml`. They're two system and four accelerated nodes
with the labels and capacity of EKS `h100` instances, which is what AICR's
lane creates for the recipe. The function runs without a recipe source and serves the
embedded catalog.

`TestStockRecipeOnSimulatedNodes` deploys the EKS `h100` training recipe with
its real charts. It proves that the charts are pulled from their registries,
that the API server accepts what they render, that `nodeScheduling` gives
every pod a node, and that each release becomes ready in AICR's order. No
operator reconciles anything, because no container starts.

The Composition under `manifests/kwok/setup` has the node placement and the
chart values AICR's lane passes to `aicr bundle`. They turn off what waits
for a webhook or a volume, which a simulated node can't serve. It skips
`nodewright-customizations`, as AICR's lane does, and `nktp`, whose
operator's webhook also guards the deletion of its resources.

Features of this profile have the `kwok` test suite label, and the default
profile doesn't run them.

## Add a feature

A feature is a `Test...` function in this package that hands
`environment.Test` a `features.NewWithDescription(...)`, as
`install_test.go` does. Label it with `LabelArea`, `LabelSize` and
`config.LabelTestSuite`. The suite fails a feature without the last one. Put
its manifests under `manifests/<area>/<feature>/` and apply them with
`funcs.ApplyResources`.

The recipe features share the XRD and the Composition under
`manifests/recipe/setup` and each creates one composite resource. They run
one at a time, and each deletes its composite resource and waits for the
releases to be gone: two composite resources with the same recipe would
install the same Helm releases. The namespaces of the charts remain, as
they do after `helm uninstall`.

[crossplane-e2e]: https://github.com/crossplane/crossplane/tree/main/test/e2e
[aicr-kwok]: https://github.com/NVIDIA/aicr/tree/main/kwok
[kwok]: https://kwok.sigs.k8s.io
[recipe-source]: https://jonasz-lasut.github.io/function-aicr/operators/recipe-source.html
[e2e-framework]: https://github.com/kubernetes-sigs/e2e-framework
