# Talos Cluster Configuration

This directory contains the version-controlled Talos machine configuration for
the home Kubernetes cluster. The configuration uses the multi-document format
introduced incrementally by Talos and expanded substantially in Talos 1.14.

## Configuration Structure

The configuration is layered so credentials remain encrypted at rest in Git:

- `secrets.sops.yaml` is the SOPS-encrypted base configuration. It contains the
  cluster credentials, PKI, cluster name, and control-plane endpoint. Decrypt it
  only at execution time with the dedicated operator-held Talos age identity;
  never commit that private identity or a decrypted copy of this file.
- `base-controlplane.yaml` contains settings shared by all three control-plane
  nodes. It retains a minimal legacy `v1alpha1` document for settings that Talos
  1.14 has not moved to typed documents, followed by typed configuration
  documents for discovery, installation, DNS, Kubernetes components, and node
  behavior.
- `k8s-node1.yaml`, `k8s-node2.yaml`, and `k8s-node3.yaml` contain typed documents
  for hostname, network link selection, static addressing, DHCPv6, the
  control-plane VIP, kernel modules, installation disk, and storage volumes.
- `bare-metal.yaml` is an Image Factory schematic, not a machine configuration
  patch. It lists the system extensions included in the bare-metal image.

The physical network interfaces are assigned the stable alias `lan0`. Node 1
selects its Intel interface with the `igc` driver; nodes 2 and 3 select their USB
interfaces with the `r8152` driver. The remaining network documents refer to the
alias instead of kernel-assigned interface names.

## Talos 1.14 Migration

Talos 1.14 still reads the legacy `v1alpha1` document for compatibility, but it
rejects configurations that define the same setting in both legacy and typed
documents. In particular, `secrets.sops.yaml` owns the cluster name and API
endpoint, so this repository intentionally does not add a `KubeClusterConfig`
document.

Upgrade every node to Talos 1.14 before applying these patches. Talos 1.13 does
not understand all of the new document kinds. The safe order is:

1. Allow the existing legacy configuration to remain in place while tuppr
   upgrades Talos.
2. After all nodes run Talos 1.14, compose and validate the new configuration.
3. Apply one node at a time and verify node readiness before continuing.

## Compose and Validate

Use Talos 1.14 tooling. The following creates a complete configuration in a
temporary directory; the output contains credentials and must not be committed:

Set `SOPS_AGE_KEY_FILE` to the dedicated Talos age identity before running these
commands. This identity is intentionally separate from the Flux SOPS identity
stored inside the cluster.

```fish
set -gx SOPS_AGE_KEY_FILE "$HOME/.config/sops/age/talos.txt"

mkdir -p /tmp/talos-config

sops exec-file tools/cluster/secrets.sops.yaml \
  'umask 077; talosctl machineconfig patch {} \
    --patch @tools/cluster/base-controlplane.yaml \
    --patch @tools/cluster/k8s-node1.yaml \
    --output /tmp/talos-config/k8s-node1.yaml'

talosctl validate \
  --config /tmp/talos-config/k8s-node1.yaml \
  --mode metal
```

Repeat with the node 2 and node 3 patch files. The rendered files contain
plaintext credentials; delete them when validation is finished:

```fish
rm -f /tmp/talos-config/k8s-node1.yaml \
  /tmp/talos-config/k8s-node2.yaml \
  /tmp/talos-config/k8s-node3.yaml
```

## Apply Configuration

Apply the layered configuration directly without writing a combined file:

```fish
sops exec-file tools/cluster/secrets.sops.yaml \
  'talosctl apply-config \
    --nodes 192.168.1.221 \
    --file {} \
    --config-patch @tools/cluster/base-controlplane.yaml \
    --config-patch @tools/cluster/k8s-node1.yaml'
```

Use `192.168.1.222` with `k8s-node2.yaml` and `192.168.1.223` with
`k8s-node3.yaml`. Apply and verify one node before moving to the next.

## Version Management

Renovate tracks the Kubernetes component images in `base-controlplane.yaml` and
groups them with the Kubernetes version used by tuppr and the cluster multitool.
The Talos installer image is also tracked in `base-controlplane.yaml`.

Talos upgrades are driven by `infrastructure/tuppr/talos-upgrade.yaml`. Review
Talos release notes before merging an upgrade, especially changes to machine
configuration, storage mounts, workload isolation, and Kubernetes component
defaults.

## Secret Rotation

Edit the encrypted base with `sops tools/cluster/secrets.sops.yaml`, or encrypt a
replacement using `--filename-override tools/cluster/secrets.sops.yaml` so SOPS
selects the dedicated Talos creation rule. Validate a fully composed
configuration before applying it. Never commit a combined configuration, Talos
PKI, kubeconfig, decrypted secret file, or age private identity. Keep a tested,
encrypted backup of the Talos age identity outside the cluster.
