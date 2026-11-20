# Schema validation

The function builds composed resources as plain maps, not typed provider
structs. It checks each one against the CRD schema Crossplane resolves for
it, through Crossplane's required-schemas protocol, and doesn't emit a
resource that fails.

| State | What happens |
|---|---|
| Crossplane doesn't advertise the required-schemas capability | Fatal. The function needs Crossplane v2.2 or later, or a `crossplane render` given `--required-schemas`. |
| The function has asked for a schema that Crossplane hasn't resolved yet | The function doesn't emit composed resources this pass. Crossplane calls again once the schema is resolved. |
| Crossplane doesn't find a CRD of that kind | Fatal: `no schema for helm.m.crossplane.io/v1beta1, Kind=Release: is provider-helm installed?` |
| Crossplane resolves the schema | Every composed resource of that kind is checked against it. A violation is Fatal. |

## Schemas the recipe needs

The `Release` schema is required when the recipe has a chart, and the
`Object` schema when one of its manifest files renders a document. A recipe
whose manifest files don't render anything doesn't oblige you to install
provider-kubernetes. That includes `dra-node-labeler`, which is part of most
stock recipes and whose manifest is empty unless
[DRA eviction](dra-eviction.md) is on.

With `crossplane render`, supply the schemas with `--required-schemas=DIR`.
See [Render locally](../authors/render-locally.md).
