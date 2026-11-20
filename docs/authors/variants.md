# Recipe variants and configuration

`profile` and `configuration` change a recipe after the criteria have
selected it. AICR checks both, and rejects either on a recipe it doesn't
apply to. That makes them natural [overlay](overlay.md) fields: the composite
resource that needs one sets it.

## Profile

`profile` selects a variant the recipe declares, as `{<name>: <value>}`. The
GKE COS recipes declare `gpuStack` with `gke-default`, `bundle-installer` and
`dra`. Unset, the recipe's default variant is
deployed.

```yaml
profile:
  gpuStack: dra
```

AICR accepts one selection for a recipe, so more than one entry is Fatal.
The [status summary](summary.md) reports the variant that was deployed.

## Configuration

`configuration` holds typed choices, with the same fields as AICR's own
recipe configuration.

`gke.tcpxoInterfaces` maps the guest interfaces `eth1` to `eth8` to the
cluster's GKE Network objects. The `h100` / `gke` / `cos` / `training` /
`kubeflow` recipe requires it, and without it the function is Fatal with
AICR's own message. The names differ for every cluster, so set this field on
the composite resource.

`runtimeInventory.mode` is `enabled` or `disabled`. `enabled` adds
`k8s-aibom`, AICR's runtime AI workload inventory, to a GKE recipe that
doesn't include it. `disabled` removes it from one that does.

The XRD has to declare `profile` and `configuration` under the overlay path
for a composite resource to set them, as the example XRD does.

```yaml
spec:
  aicr:
    criteria:
      accelerator: h100
      service: gke
      os: cos
      intent: training
      platform: kubeflow
    configuration:
      gke:
        tcpxoInterfaces:
        - interfaceName: eth1
          network: my-cluster-gpu-nic-0
        # eth2 to eth8
```
