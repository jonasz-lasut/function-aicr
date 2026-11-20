# Rollout order

The function withholds composed resources from the desired state until what
AICR orders before them is observed `Ready`. This is the gate. It orders
creation only.

Every `Release` sets `spec.forProvider.wait: true`. provider-helm then
reports a `Release` `Ready` only once the Helm release's resources are up,
and that's the signal the gate reads.

## The rules

For each component:

- its pre-manifest `Object`s wait for the components it depends on;
- its `Release` waits for those dependencies and for its pre-manifest
  `Object`s;
- its post-manifest `Object`s wait for its `Release`.

A manifest-only component has no `Release`. Its `Object`s all wait for its
dependencies, and as somebody else's dependency it's ready when every
`Object` it emitted is.

A dependency counts only when the function deploys it. One that's skipped,
turned off by the recipe or left out by `componentClasses` is satisfied.

Every resource the gate withholds raises a Normal result that says what it
waits for. The [status summary](../authors/summary.md) lists a component
under `pendingComponents` while its `Release`, or an `Object` of a
manifest-only component, is withheld.

## Live resources

A resource that has already been observed is never withheld. The gate must
not tear down a live resource because a sibling went unready for a moment.

## Turning it off

`skipDeploymentOrder: true` emits every component at once. Use it only when
something else orders the installs, or when the order doesn't matter.
