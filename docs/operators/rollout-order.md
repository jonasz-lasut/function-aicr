# Rollout order

The function withholds composed resources from the desired state until what
AICR orders before them is observed `Ready`. This is the gate. It orders
creation only.

Every `Release` sets `spec.forProvider.wait: true`. provider-helm then
reports a `Release` `Ready` only once the Helm release's resources are up,
and that's the signal the gate reads.

The signal is about the installation. provider-helm waits for the resources
while it installs or upgrades a release. Afterward it reports the `Release`
`Ready` for as long as the Helm release is deployed and matches the
`Release`'s specification, even when its pods have stopped.

An upgrade interrupts the signal for longer than it runs. provider-helm
marks a `Release` unready when it starts to upgrade it, and looks at it
again at its next poll, which comes every 10 minutes by default. A component
whose values or version changed reports unready until then, and the
composite resource with it, although Helm finished the upgrade long before.
Nothing is withheld in the meantime, because the gate leaves what's already
deployed alone. provider-helm's `--poll` flag shortens the wait.

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

## Crossplane's circuit breaker

Crossplane limits how often watch events reconcile one composite resource.
It allows a burst of 100 events and one more per second after that, counting
each change to the composite resource and to the resources it composes. The
resources of a rollout are created together and each reports its status more
than once, which can use up the burst within seconds. In the end-to-end
suite a recipe of three components did so three seconds after its composite
resource was created.

Crossplane then sets the condition `Responsive` to `False` with the reason
`WatchCircuitOpen`. For the next five minutes it lets one event through
every 30 seconds. The rollout continues, but each step waits for the next of
those reconciles, and the status summary lags by as much.

If that's too slow, raise `--circuit-breaker-burst` in Crossplane's
arguments. The suite's recipe was ready after 64 seconds with the default
and after 5 seconds with a burst of 1000.

## Live resources

A resource that has already been observed is never withheld. The gate must
not tear down a live resource because a sibling went unready for a moment.

## Turning it off

`skipDeploymentOrder: true` emits every component at once. Use it only when
something else orders the installs, or when the order doesn't matter.
