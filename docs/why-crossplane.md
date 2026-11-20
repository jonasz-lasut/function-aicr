# Why Crossplane

AICR renders a recipe into a bundle for a deployer such as Helm, Argo CD,
Flux, Helmfile, Terraform or Fleet. The bundle is generated once, for one
cluster, and regenerated when the recipe or its inputs change.

As a Crossplane composition the recipe is resolved by the control plane, from
each composite resource, every time that resource is reconciled.

**One API, many clusters.** Define an XRD of your own and let each composite
resource state its accelerator, service, intent and target cluster. One
Composition serves every workload cluster, without a bundle to generate or
store for each.

**The recipe follows its inputs.** A change to a composite resource, to the
Composition or to the function's version takes effect on the next reconcile.
provider-helm and provider-kubernetes then converge every `Release` and
`Object` on the result.

**Ordered rollout on observed readiness.** A component is withheld until what
AICR orders before it reports `Ready`, read from the providers' observed
conditions.

**Pipeline composability.** Add functions before or after, to derive values
from the composite resource or to enforce policy. Each is an independent step
that leaves the recipe untouched.

**A local loop.** `crossplane render` runs the function on your machine, and
every composed resource is checked against the provider CRD schema before
anything is applied.
