# DRA eviction

When the GPU Operator replaces a driver in place, the DRA kubelet plugin has
to leave the node first. AICR's contract for that spans three components, and
`draEviction: true` wires it.

| Component | Setting |
|---|---|
| `dra-node-labeler` | Enabled. Its manifest is empty by default. Enabled, it labels every GPU node `nvidia.com/dra-kubelet-plugin=true`. |
| `nvidia-dra-driver-gpu` | The kubelet plugin is scheduled only on nodes with that label. |
| `gpu-operator` | The Driver Manager is told the label's key. It changes the label's value while it replaces the driver, which makes the plugin leave the node, and restores it afterward. |

The wiring is applied after `componentOverrides`, so an override can't leave
the three disagreeing. Other node selectors and environment entries are kept.

A recipe without all three components has no contract to wire. The function
raises a Warning and wires nothing. The OpenShift variants of the components
aren't covered, as in AICR.
