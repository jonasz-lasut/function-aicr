# Upgrade approval

* Owner: @jonasz-lasut (Jonasz Małecki)
* Reviewers: function-aicr maintainers
* Status: draft (the design is open for review; an issue for the work follows once this record is merged)

## Summary

A release of the function embeds one AICR catalog. Installing it changes chart
versions on every stack the function reconciles, on the next pass. The
function looks each change up in AICR's upgrade records and raises a Warning
for every one AICR doesn't rate `safe`. It applies the change in the same
pass.

This record adds an approval that a Composition opts in to. With
`upgradeApproval` set, a component whose version change isn't rated `safe`
keeps what it has deployed until an approver writes that change into the
status of the composite resource. Components that depend on it wait with it.
First installs and changes rated `safe` aren't delayed.

The approval is a status field, as in
[function-approve](https://github.com/upbound/function-approve). The rule for
what needs one comes from the AICR Operator
([ADR-027](https://github.com/NVIDIA/aicr/blob/v1.3.0/docs/design/027-aicr-operator.md)):
a change runs unattended only when AICR rates it `safe`.

## Background

`stack.warnUnsafeUpgrades` reports AICR's verdicts and delays nothing. The
[upgrades page](operators/upgrades.md) lists what that meant for a stack
moving from the catalog of AICR v0.20.0 to the one of v1.3.0: seven changes
that aren't `safe`, of which AICR rates three `blocked`, three `manual` and
one `unknown`. The function applied the seven together with the rest.

With approval on, that catalog change would have needed an approval for four
components and a version set by hand for the three `blocked` ones. AICR
v1.3.0 has upgrade records for 24 of its 58 registry components, and a
version change of a component without a record is `unknown`. The 43 recorded
transitions are 26 `manual` and 17 `safe`.

## Design

### Opting in

```yaml
upgradeApproval:
  fieldPath: status.upgrades
```

`fieldPath` is an object on the composite resource. The function writes what
it delays there and reads the approvals from it. The path must begin with
`status`, as `target.fieldPath` must. The XRD has to declare the object,
because Crossplane prunes a field the XRD doesn't declare.

Without `upgradeApproval` the function behaves as it does today.

### Which components are held

On every pass the function compares the chart version of each observed
`Release` with the version it's about to compose and hands the differences to
`upgrade.Match`. That part exists today. With approval on, a component is
held when one of these is true:

- Its version change isn't rated `safe` (`ComponentResult.FailsRun`) and
  the status doesn't approve it.
- It depends on a held component, directly or through other components.

The second rule follows the edges the gate already reads
(`ComponentRef.DependencyRefs`). Without it `nvidia-dra-driver-gpu` would
move to the version of the new catalog while `gpu-operator` waits at the old
one. The driver depends on the operator, and the two charts share the
ComputeDomain CRD.

A component that isn't deployed has no version change, and a first install
waits only when the component depends on a held one. When AICR's records
can't be loaded, every version change is held: a change nobody assessed isn't
`safe`.

### Resources of a held component

The function composes the whole stack as usual and then replaces the held
components' part of it:

- Each observed composed resource of a held component is emitted with its
  observed `spec`.
- A resource of the component that isn't observed yet isn't created.
- An observed resource that the new recipe doesn't render any more remains.

The providers don't see a change. They keep reconciling what's deployed, and
its readiness is still the live one. The rest of the stack follows the new
catalog.

A resource belongs to the component its name starts with: the `Release` has
the component's name, and an `Object` is `<component>-pre-...` or
`<component>-post-...`. The longest component name that fits is the owner.
Names are already a pure function of the component and the document, because
the gate correlates by name.

This is the second place where observed state changes a composed resource.
The first is a deployed `Release` keeping its namespace. It has the same
justification: the `spec` of an observed resource is what the function itself
last emitted.

### The approval

With the catalog change from the Background section, the composite resource
shows this:

```yaml
status:
  upgrades:
    held:
    - component: nfd
      from: 0.19.0
      to: 0.20.1
      verdict: manual
    - component: gpu-operator
      from: v26.3.3
      to: v26.7.2
      verdict: blocked
      waitingFor: nfd
```

Each entry with a verdict also has AICR's `explanation` and the summary of
every record the change crosses, which are the texts of today's Warning.
`nvidia-dra-driver-gpu` is in the list too, waiting for `gpu-operator`.

An approver writes the change under `approved`:

```shell
kubectl patch clusterstack gpu-training-stack --subresource=status --type=merge \
  -p '{"status":{"upgrades":{"approved":{"nfd":{"from":"0.19.0","to":"0.20.1"}}}}}'
```

On the next pass the entry equals the pending change of `nfd`, and the
function composes `nfd` from the new catalog. The rules:

- An entry approves one change of one component, from one version to another.
  A new release of the function with another target needs a new entry.
- The function never writes `approved`. An entry that's been used matches
  only the same change again.
- An entry for a held component with other versions raises a Warning that
  quotes both.
- `blocked` can't be approved. AICR's records say the change mustn't happen
  in one step. `componentOverrides.<component>.version` sets a target AICR
  doesn't block, and the function assesses that change like any other. For
  `gpu-operator` in the example the target is v26.7.0, where AICR's records
  begin. AICR rates the change to v26.7.0 `unknown`, which an approver can
  accept. It rates the change from there to v26.7.2 `safe`.
- A component that only waits for another doesn't need an entry.

Kubernetes RBAC separates the roles. Updating the status of a composite
resource is a permission of its own (`clusterstacks/status` for the example),
apart from editing the resource. The audit log records who approved.

### Results and status

The function raises today's Warning for each held change and adds where the
approval goes. A component that waits for another gets a Normal result, as a
component the gate delays does. `held` is an empty list when nothing is held.
In the [status summary](authors/summary.md), `deployedComponents` lists a
held component with the version it's deployed with.

### Next to the AICR Operator

| | AICR Operator | This design |
|---|---|---|
| Unit that waits | the whole transition | a component and its dependents |
| A chart that renders a CRD | needs approval | not assessed: the function doesn't render charts |
| The approval is bound to | the live engine objects, the plan and its steps | the component and both versions |
| Expiry and revocation | both | neither: an entry is removed by hand before it's used |
| Steps after the change | a second acknowledgement | the operator's to do, from the Warning |
| Validation after the rollout | deployment validation, bound as evidence | none |
| `blocked` | no approval overrides it | the same |

## Code shape

- `input/v1beta1`: `UpgradeApproval` with `FieldPath`, and the same check
  `validateTarget` makes.
- `internal/compose`: `Owner`, which attributes a composed resource's name to
  a component, and `Keep`, which builds the desired resource from an observed
  one.
- `fn.go`: `stack.holdUpgrades` runs after `composeObjects` and before the
  gate. It takes over the assessment from `warnUnsafeUpgrades`, which remains
  for a stack without approval. The `held` list is written through
  `setStatus`.

`aicr.run/pkg/upgrade` is in AICR's internal tier. The warnings already
depend on it.

## Tests

- `TestHoldUpgrades` drives `stack.holdUpgrades` through hand-built maps, as
  `TestGate` does. Its cases are a held change and an approved one, a
  `blocked` change, a dependent, an approval with other versions and a
  resource the new recipe drops.
- `TestRunFunction` gets cases with goldens for a held and an approved
  change.
- `TestCatalogInvariants` asserts that `compose.Owner` attributes every name
  a leaf renders to the component that rendered it.
- End to end, catalog profile: a second chart version and an upgrade record
  with a `manual` transition in `test/e2e/catalog`. The feature overrides the
  version of a deployed component and expects the `Release` unchanged and the
  change under `held`. After it patches the status, the release is upgraded.
  The feature also shows that the function's status writes keep the field an
  approver wrote, which function-approve relies on too.

## Documentation

A section in [Upgrades](operators/upgrades.md), the field in the
[Input reference](operators/input-reference.md), an entry in
[Design decisions](project/design-decisions.md) and one in
[Troubleshooting](operators/troubleshooting.md) for a stack that doesn't
upgrade. The example XRD declares `status.upgrades`.

## Compatibility

The field is optional and nothing changes without it. Setting it on a running
stack delays nothing until a version change is pending. Removing it applies
every held change on the next pass.

## Alternatives

### A pipeline step with function-approve

function-approve hashes one field of the desired composite resource and
returns a Fatal result until a status flag approves the hash. That fits a
rule that any change to one field needs approval. It can't express AICR's
verdicts: a list of pending changes is empty again once they're applied,
which is a new hash. It also asks for an approval of the first install and
stops the whole composite resource while it waits.

### Holding the whole stack

The AICR Operator waits with the entire transition when one component isn't
`safe`. Here the function would emit every observed resource unchanged until
one approval covers the transition. The attribution by name and the rule for
dependents would go away, and a stack would be on one catalog at a time.
Other changes would wait as well while an approval is pending: a toleration
that has to be fixed today waits for the upgrade to be approved.

### The approval in the Input

A list of approved changes in the `Input`, set on the composite resource
through the overlay, doesn't need a status field and fits a stack managed
from Git. Whoever edits the composite resource could then approve its
upgrades.

### Pausing the resources

The function could emit the new `spec` with
`crossplane.io/paused`. The pending change would be visible on the `Release`.
The provider would stop correcting drift. The observed `spec` would also
stop saying what's deployed, and the assessment reads the deployed version
from it.

### A boolean approval

function-approve's `approved: true` approves whatever is pending when the
function next runs. An entry with both versions approves what the approver
read.

## Open questions

1. Whether a held component takes its dependents with it, as designed here,
   or the whole stack waits, as with the AICR Operator.
2. Whether `blocked` can be approved. The three `blocked` changes in the
   Background section start from versions older than AICR's records, and
   each needs an intermediate version set by hand.
3. Whether `unknown` needs an approval. It's the verdict of every version
   change of the 34 components without a record, and it's a Warning today.
4. Whether the approval remains opt-in after a release or two.

A component with a short-lived credential is a second use for the hold:
[issue 24](https://github.com/jonasz-lasut/function-aicr/issues/24#issuecomment-7047994219)
needs a fresh token in a Secret before such a component is upgraded, and an
approval is the moment to write one.
