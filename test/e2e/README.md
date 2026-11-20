# End-to-end suite

The suite runs function-aicr the way a control plane does. The function is
built from this tree and installed as a Function package by Crossplane in a
kind cluster, where provider-helm and provider-kubernetes compose into the
cluster itself.

It follows [the suite of `crossplane/crossplane`][crossplane-e2e]. Features
are built with [`sigs.k8s.io/e2e-framework`][e2e-framework], run through
Crossplane's e2e environment (`test/e2e/config`) and assembled from its
helpers (`test/e2e/funcs`). Each feature keeps its manifests in a directory
of its own under `manifests/`. The suite is a Go module of its own, so
Crossplane's dependency tree doesn't enter the function's `go.mod`.

## Run it

Docker has to be running. From the repository root:

```shell
git submodule update --init   # the Crossplane build submodule, once
make e2e.up                   # kind, Crossplane, the providers, the registry, the function
make e2e.test                 # the suite
make e2e.function             # after a code change: rebuild and reinstall the function
make e2e.down
```

`make e2e` does up, test and down in one go. After a failed run,
`make e2e.dump` writes the cluster's state under `_output/controlplane-dump`.

One feature runs with Go's own filter:

```shell
make e2e.test E2E_TEST_FLAGS='-v -count=1 -run TestFunctionInstalled'
```

## The cluster

`make e2e.up` provisions it with the make library of the Crossplane `build`
submodule (pinned `kind`, `kubectl`, `helm` and `crossplane` under
`.cache/tools`):

| Piece | Where it comes from |
|---|---|
| kind and Crossplane | `controlplane.up`, at `CROSSPLANE_VERSION` |
| An OCI registry at `registry.e2e.svc:5443` | `manifests/setup/registry.yaml`, behind TLS from a CA `make e2e.certs` creates under `_output/e2e/certs` |
| provider-helm, provider-kubernetes, `function-auto-ready` | `manifests/setup/packages.yaml`, at the versions the Makefile pins |
| A `ClusterProviderConfig` named `e2e` for each provider | `manifests/setup/provider-configs.yaml`. Each provider uses its own service account, bound to `cluster-admin`, which makes the kind cluster the target cluster. |
| function-aicr | `make e2e.function`. It builds the image from the Dockerfile and a package from the image, and installs the package from Crossplane's cache with `packagePullPolicy: Never`. |

The function runs with `manifests/setup/function-runtime.yaml`. That turns
on debug logging and puts the registry's CA in the trust store, because AICR
pulls an OCI recipe source over HTTPS only.

## Add a feature

A feature is a `Test...` function in this package that hands
`environment.Test` a `features.NewWithDescription(...)`, as
`install_test.go` does. Label it with `LabelArea`, `LabelSize` and
`config.LabelTestSuite`. The suite fails a feature without the last one. Put
its manifests under `manifests/<area>/<feature>/` and apply them with
`funcs.ApplyResources`.

[crossplane-e2e]: https://github.com/crossplane/crossplane/tree/main/test/e2e
[e2e-framework]: https://github.com/kubernetes-sigs/e2e-framework
