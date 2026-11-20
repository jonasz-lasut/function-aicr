# Components and overrides

A recipe is a set of components. This page says which of them the function
deploys, what each becomes, and how to leave one out or change it.

## Composed resources

A chart-backed component becomes a `helm.m.crossplane.io/v1beta1` `Release`
named after the component (`cert-manager`, `gpu-operator`), so the Helm
release in the target cluster has that name too.

Components also have manifest files: `preManifestFiles` that must exist
before the chart and `manifestFiles` that follow it. A manifest-only
component has no chart at all. The files are Helm-style templates. The
function renders them with AICR's own renderer from the component's
namespace, chart, version and values, and wraps each document in a
`kubernetes.m.crossplane.io/v1alpha1` `Object` named
`<component>-<pre|post>-<kind>-<name>`. The manifests take an override
exactly as the `Release` does. A file that fails to render or parse
is Fatal, which keeps a recipe from being deployed with a piece missing.

The namespace has to exist before an `Object` can be created in it, and for
a pre-manifest file that's before the chart that would create it. A
component with pre-manifest files, and a manifest-only component, gets one
more `Object` for that reason, named `<component>-pre-namespace-<namespace>`. It
creates the component's namespace when it's missing and never changes or
deletes it, as Helm's `--create-namespace` does. A recipe that declares the
namespace in the component's own manifest files keeps its own.

## Which components are deployed

The function deploys the recipe's Helm components that AICR enables. A
component an overlay of the catalog disables remains in AICR's inventory but
is never composed, exactly as AICR's own deployers treat it.

`skipComponents` removes further components by name. A name that matches no
component of the recipe raises a Warning, since a typo would otherwise deploy
the component it meant to skip.

`componentClasses` keeps only the components of the classes it names. AICR's
registry puts every component in one of two: `core` is what a plain GPU pod
needs to run, and `ops` is everything else, workload platforms included. One
composite resource can then own the GPU core and another the ops layer.

A component that isn't deployed, for either reason, counts as satisfied when
another depends on it: you opted to manage it elsewhere. This has one
consequence for `componentClasses`. The [rollout order](../operators/rollout-order.md)
is worked out within one composite resource, so an `ops` component that
depends on a `core` one deployed by another composite resource doesn't wait
for it.

Leaving out a component that's already deployed removes its `Release` and
`Object`s from the desired state, and Crossplane deletes them.

## Overrides

`componentOverrides` changes a deployed component, keyed by its name.

```yaml
componentOverrides:
  gpu-operator:
    version: v26.7.2
    namespace: nvidia
    values:
      driver:
        enabled: false
```

| Field | Effect |
|---|---|
| `version` | The chart version. |
| `namespace` | The namespace the component is installed in. |
| `values` | Deep-merged over the values the recipe resolved. On a conflict the override's value is used. |

An entry for a component that isn't deployed is ignored with a Warning.

## A deployed component keeps its namespace

When the recipe names another namespace than the one a component is deployed
in, as after an AICR release that moves a component's default namespace, the
function doesn't move the live `Release`. It keeps the namespace the observed
`Release` has and reports it as a Warning. A manifest-only component that the
recipe places in the namespace of a component it depends on follows that
component. A custom resource then remains next to its operator.

To move a component yourself, set
`componentOverrides.<component>.namespace`. The function uses that namespace
whatever it observes.

provider-helm then installs the chart in the new namespace and leaves the
release in the old one in place. Uninstall the old release with Helm. A
chart with cluster-scoped resources needs that first, because Helm refuses
the second install with `invalid ownership metadata` while the old release
owns them.
