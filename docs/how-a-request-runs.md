# A request, step by step

Crossplane calls the function when it reconciles a composite resource. This
page follows one call from the `Input` to the desired state, in the
order the function works. Each step links to the page that covers it.

1. **The Input is resolved.** The static `Input` from the Composition is
   merged with the object at `overlayFrom.fieldPath` on the composite
   resource. An unknown field in either is Fatal. See
   [Values from the composite resource](authors/overlay.md).
2. **The criteria and the provider config are checked.** At least one
   criteria dimension must be set and each value must be one AICR knows. The
   provider config needs a name.
3. **AICR resolves the recipe.** The criteria select it, and `profile` and
   `configuration` shape it. See
   [Recipe variants and configuration](authors/variants.md).
4. **The managed components are selected.** The function deploys the
   recipe's Helm components that AICR enables. `skipComponents` removes some
   by name, and `componentClasses` removes whole classes. See
   [Components and overrides](authors/components.md).
5. **Values and namespaces are settled.** Each component's values are
   resolved and `nodeScheduling` is written into them. A component that
   requires a node selector and has none is Fatal. `draEviction` is wired
   when set, and a component that's already deployed keeps its namespace.
   See [Node scheduling](operators/node-scheduling.md).
6. **The manifest files are rendered.** AICR's manifest files are Helm-style
   templates. The function renders them with the component's values and
   splits them into documents. A component whose manifests need its
   namespace before a chart creates it gets an `Object` for the namespace.
7. **The provider schemas are required.** The function asks Crossplane for
   the `Release` schema when the recipe has a chart, and for the `Object`
   schema when a manifest rendered a document. Until Crossplane has resolved
   them the function composes nothing, and Crossplane calls it again within
   the same reconcile. See
   [Schema validation](operators/schema-validation.md).
8. **The resources are composed and validated.** One `Release` per chart and
   one `Object` per manifest document, each checked against its schema.
9. **The gate withholds what isn't due.** Unless `skipDeploymentOrder` is
   set, a resource whose prerequisites aren't observed `Ready` is left out of
   the desired state. See [Rollout order](operators/rollout-order.md).
10. **Upgrades are assessed.** A chart version change to a deployed `Release`
    is looked up in AICR's upgrade records. See
    [Upgrades](operators/upgrades.md).
11. **The summary is written.** When `target` is set, the function writes the
    recipe's identity and the rollout's progress to the status of the
    composite resource. See [The status summary](authors/summary.md).

## Results

The function reports through the results Crossplane turns into events on the
composite resource.

| Severity | Meaning |
|---|---|
| Fatal | The function composed nothing this pass. The message says what to fix, such as an `Input` field or a missing provider. |
| Warning | The pass went through, with something to look at, such as a namespace that was kept or an upgrade AICR doesn't rate safe. |
| Normal | Progress. Each resource the gate withholds is named with what it waits for. |
