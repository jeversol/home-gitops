# Talos Cluster Configuration

This directory contains the Talos configuration files for the home Kubernetes cluster.

## Configuration Structure

The Talos configuration is split into multiple files to separate secrets from version-controlled configuration:

### Files in Git (this repository)

- **`base-controlplane.yaml`** - Base control plane configuration including:
  - Kubernetes component versions (kubelet, apiserver, controller-manager, proxy, scheduler)
  - Feature flags and cluster settings
  - Network configuration
  - API server configuration
  - **Does NOT contain any secrets or certificates**

- **`bare-metal.yaml`** - System extensions for bare metal deployment:
  - btrfs, drbd, i915, intel-ucode, iscsi-tools, realtek-firmware, util-linux-tools

- **`k8s-node1.yaml`** - Node-specific configuration for k8s-node1:
  - Hostname: k8s-node1
  - IP address: 192.168.1.221/24
  - VIP: 192.168.1.235
  - Disk: /dev/nvme0n1
  - Longhorn mount configuration

- **`k8s-node2.yaml`** - Node-specific configuration for k8s-node2:
  - Hostname: k8s-node2
  - IP address: 192.168.1.222/24
  - VIP: 192.168.1.235
  - Disk: /dev/nvme0n1
  - Longhorn mount configuration

- **`k8s-node3.yaml`** - Node-specific configuration for k8s-node3:
  - Hostname: k8s-node3
  - IP address: 192.168.1.223/24
  - VIP: 192.168.1.235
  - Disk: /dev/nvme0n1
  - Longhorn mount configuration

### Files Outside Git (local only)

- **`../talos/secrets.yaml`** - Contains cluster secrets and certificates:
  - Machine CA and token
  - Cluster CA, aggregator CA, and etcd CA
  - Service account keys
  - Cluster ID and secrets
  - **NEVER commit this file to git**

- **`../talos/controlplane.yaml`** - Original monolithic config (deprecated)
  - This file is no longer used
  - Kept for reference only
  - All functionality has been split into secrets.yaml + base-controlplane.yaml

## Applying Configuration Changes

Talos uses a layered configuration system where patches are applied on top of a base configuration.

### General Pattern

```bash
talosctl apply-config --nodes <node-ip> \
  --file ../talos/secrets.yaml \
  --config-patch @tools/cluster/base-controlplane.yaml \
  --config-patch @tools/cluster/bare-metal.yaml \
  --config-patch @tools/cluster/<node-specific>.yaml
```

### Apply to Specific Nodes

**k8s-node1 (192.168.1.221):**
```bash
talosctl apply-config --nodes 192.168.1.221 \
  --file ../talos/secrets.yaml \
  --config-patch @tools/cluster/base-controlplane.yaml \
  --config-patch @tools/cluster/bare-metal.yaml \
  --config-patch @tools/cluster/k8s-node1.yaml
```

**k8s-node2 (192.168.1.222):**
```bash
talosctl apply-config --nodes 192.168.1.222 \
  --file ../talos/secrets.yaml \
  --config-patch @tools/cluster/base-controlplane.yaml \
  --config-patch @tools/cluster/bare-metal.yaml \
  --config-patch @tools/cluster/k8s-node2.yaml
```

**k8s-node3 (192.168.1.223):**
```bash
talosctl apply-config --nodes 192.168.1.223 \
  --file ../talos/secrets.yaml \
  --config-patch @tools/cluster/base-controlplane.yaml \
  --config-patch @tools/cluster/bare-metal.yaml \
  --config-patch @tools/cluster/k8s-node3.yaml
```

### Apply to All Nodes

```bash
for node in 192.168.1.221 192.168.1.222 192.168.1.223; do
  echo "Applying config to $node..."
  talosctl apply-config --nodes $node \
    --file ../talos/secrets.yaml \
    --config-patch @tools/cluster/base-controlplane.yaml \
    --config-patch @tools/cluster/bare-metal.yaml \
    --config-patch @tools/cluster/k8s-node${node##*.}.yaml
done
```

Note: The node number extraction in the loop above works because node IPs end in .221, .222, .223 and node files are named k8s-node1.yaml, k8s-node2.yaml, k8s-node3.yaml. Adjust if this pattern changes.

## Version Management

### Kubernetes Component Versions

Kubernetes component versions are tracked in `base-controlplane.yaml` and automatically updated by Renovate:

- **kubelet:** `ghcr.io/siderolabs/kubelet`
- **apiserver:** `registry.k8s.io/kube-apiserver`
- **controller-manager:** `registry.k8s.io/kube-controller-manager`
- **proxy:** `registry.k8s.io/kube-proxy`
- **scheduler:** `registry.k8s.io/kube-scheduler`

Renovate will create PRs for new Kubernetes versions with these constraints:
- **Minimum age:** 7 days after release (gives community time to find issues)
- **Allowed versions:** Constrained to versions supported by current Talos (see below)

### Talos Version Compatibility

Talos and Kubernetes versions must be compatible. The current constraints are:

- **Talos 1.11.x** supports **Kubernetes 1.34.x only**
- See support matrix: https://docs.siderolabs.com/talos/getting-started/support-matrix/

The `allowedVersions` constraint in `.github/renovate.json5` prevents Renovate from creating PRs for unsupported Kubernetes versions.

### Upgrading Talos

When upgrading Talos to a new minor version (e.g., 1.11.x → 1.12.x):

1. **Check the support matrix** for the new Talos version:
   - https://docs.siderolabs.com/talos/getting-started/support-matrix/
   - Identify which Kubernetes versions are supported

2. **Update Renovate constraints** in `.github/renovate.json5`:
   ```json5
   {
     groupName: 'kubernetes-components',
     // Update this line based on support matrix:
     allowedVersions: '/^v1\\.(34|35)\\./',  // Example: if 1.12.x supports both 1.34 and 1.35
   }
   ```

3. **Upgrade Talos** on all nodes using system-upgrade-controller or manual talosctl commands

4. **Upgrade Kubernetes** (if desired) by merging Renovate PRs for new k8s versions

## Common Scenarios

### Updating Kubernetes Versions

When Renovate creates a PR to update Kubernetes versions in `base-controlplane.yaml`:

1. Review the PR to ensure all 5 components are being updated together
2. Verify the version is supported by your current Talos version
3. Merge the PR
4. Apply the configuration to all nodes (see "Apply to All Nodes" above)
5. Monitor the system-upgrade-controller or manually verify the updates

### Changing Node Configuration

To change node-specific settings (IP, disk, mounts, etc.):

1. Edit the appropriate `k8s-nodeX.yaml` file
2. Commit and push the changes
3. Apply the configuration to the specific node

### Changing Cluster-Wide Settings

To change cluster-wide settings (features, network config, API server settings, etc.):

1. Edit `base-controlplane.yaml`
2. Commit and push the changes
3. Apply the configuration to all nodes

### Rotating Secrets or Certificates

If you need to rotate cluster secrets or certificates:

1. Generate new secrets using `talosctl gen secrets`
2. Update `../talos/secrets.yaml` with the new values
3. **Do NOT commit secrets.yaml to git**
4. Apply the configuration to all nodes

## Troubleshooting

### Configuration not applying

- Verify the node IP is correct
- Ensure you have network connectivity to the node
- Check talosctl configuration: `talosctl config info`
- Review node logs: `talosctl logs --nodes <node-ip>`

### Version mismatch errors

- Ensure the Kubernetes version in `base-controlplane.yaml` is supported by your Talos version
- Check the Talos support matrix

### Secrets not found

- Verify `../talos/secrets.yaml` exists and has correct permissions
- Ensure the file path in the apply-config command is correct

## Migration Notes

This configuration structure was created to:
1. Separate secrets from version-controlled configuration
2. Enable Renovate to automatically update Kubernetes versions
3. Prevent accidentally applying old Kubernetes versions when making configuration changes

The previous monolithic `controlplane.yaml` file is deprecated but kept for reference at `../talos/controlplane.yaml`.
