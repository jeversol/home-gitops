# LINSTOR Volume Backup Solution (Volsync + restic → S3)

## Context

LINSTOR/Piraeus volumes currently have **no backup** — only DRBD 3-way replication
(`placementCount: 3`), which protects against node/disk failure but **not** accidental
deletion (StorageClass `reclaimPolicy: Delete`), corruption, or ransomware (all replicas
written synchronously). The only existing data backups are etcd (s3cmd CronJob) and
`spoolman-postgres` (CNPG/Barman). Everything else — media *arr configs, Plex, homebridge,
spoolman, scrutiny, Grafana — is unprotected.

**Goal:** declarative, GitOps-managed, per-volume backups to the existing S3 endpoint
(`https://nas.i.recompiled.org:3900`) with file-level restore.

## Decision: Volsync (restic), not LINSTOR-native S3 backup

LINSTOR-native scheduled backup (`linstor remote` / `linstor backup schedule`) is
**imperative controller state that lives outside git** — bootstrapped via `linstor` CLI in
the controller pod — which violates the GitOps requirement, and its restore is whole-volume
and clunky. Volsync is declarative (CRs per volume, all in git), gives deduplicated,
encrypted, file-level restic backups with tiered retention, and is the community standard for
exactly this. The only database (postgres) is already Barman-backed, so everything left is
file-level app config — a perfect fit for restic.

This requires the Kubernetes **snapshot-controller**, which is *not* installed (verified:
`kubectl get crd | grep snapshot.storage.k8s.io` → none). The pool is `lvmThinPool`, which
LINSTOR explicitly supports for snapshots, and the `linstor-csi-controller` pod **already runs
the `csi-snapshotter` sidecar** (verified) — so CSI snapshots will work once the cluster-side
controller and a `VolumeSnapshotClass` exist. This unlocks `copyMethod: Snapshot` for
point-in-time consistent backups.

### Verified facts (no guesses)
- Pool `pool1` = `lvmThinPool` → LINSTOR supports snapshots for LVM_THIN ✓ (Piraeus docs).
- `linstor-csi-controller` already includes the `csi-snapshotter` sidecar ✓ (live cluster).
- snapshot-controller: Piraeus publishes chart `snapshot-controller` **v5.1.1** (appVersion
  `v8.6.0`) at `oci://ghcr.io/piraeusdatastore/helm-charts` (the repo already wired as the
  `piraeus-charts` HelmRepository).
- VolumeSnapshotClass for LINSTOR is minimal: `driver: linstor.csi.linbit.com`,
  `deletionPolicy: Delete` (per Piraeus snapshots tutorial).
- Volsync chart `volsync` **v0.16.0**, repo `https://backube.github.io/helm-charts`. CRDs are
  shipped in the chart's `templates/` with `manageCRDs: true` → chart-managed & upgraded.
- S3 settings (from working Loki/Mimir configs): endpoint `nas.i.recompiled.org:3900`,
  **region `us-east-1`**, path-style, TLS on port 3900. → restic needs
  `AWS_DEFAULT_REGION=us-east-1`.
- **Leftover state to reconcile:** `volsync-system` namespace exists but is *empty* (no
  controller, no HelmRelease); Volsync CRDs exist from 2025-08-30, applied *outside* Helm
  (no ownership metadata). Because the chart manages CRDs via `templates/`, Helm's **first**
  install will refuse to adopt pre-existing unmanaged CRDs ("invalid ownership metadata") →
  must delete the two stale CRDs before first install (see step 2).

## Prerequisite (user action, outside this repo)
Create an S3 bucket (e.g. `volsync`) and an access key/secret with read-write on it.
These go into the SOPS-encrypted restic secrets below. (DR note: restoring also requires the
**age private key stored off-cluster** — it decrypts the restic repo password.)

## Volumes to back up (confirmed scope)
All linstor-r3 PVCs **except** o11y working dirs (loki/mimir — data already in S3) and
`spoolman-postgres-1` (CNPG/Barman). One ReplicationSource each:

| Namespace  | PVC                            | Size  |
|------------|--------------------------------|-------|
| media      | config-sonarr-0                | 2Gi   |
| media      | config-radarr-0                | 2Gi   |
| media      | config-prowlarr-0              | 2Gi   |
| media      | config-tautulli-0              | 5Gi   |
| media      | config-overseerr-0             | 256Mi |
| media      | config-plex-0                  | 20Gi  |
| homebridge | storage-homebridge-0           | 10Gi  |
| spoolman   | data-spoolman-0                | 2Gi   |
| spoolman   | bambu-spoolman-config          | 1Gi   |
| o11y       | config-scrutiny-0              | 500Mi |
| o11y       | influxdb                       | 1Gi   |
| o11y       | kube-prometheus-stack-grafana  | 10Gi  |

> **Flag:** `kube-prometheus-stack-grafana` was not in the original list but holds Grafana's
> UI-managed dashboards/alerts (SQLite, *not* in S3). Included as recommended — remove if
> undesired.

**Schedule/retention (confirmed):** daily; `retain: { daily: 7, weekly: 5, monthly: 6 }`.

## Implementation

### 1. snapshot-controller + VolumeSnapshotClass
- New `flux/infrastructure/snapshot-controller.yaml`: HelmRelease `snapshot-controller`
  v5.1.1 from the existing `piraeus-charts` HelmRepository, into a `snapshot-controller`
  namespace (`createNamespace: true`), chart installs the snapshot CRDs + controller. Follow
  the `piraeus.yaml` HelmRelease idiom (`driftDetection: enabled`, remediation retries).
- `infrastructure/volsync/volume-snapshot-class.yaml`: a `VolumeSnapshotClass` named
  `linstor-r3` — `driver: linstor.csi.linbit.com`, `deletionPolicy: Delete`.

### 2. Volsync operator
- New `flux/infrastructure/volsync.yaml`: HelmRepository `backube`
  (`https://backube.github.io/helm-charts`) + HelmRelease `volsync` v0.16.0 →
  `volsync-system` (adopts existing empty namespace).
- **Stale-CRD pre-step (one-time, unconditional — do BEFORE the HelmRelease reconciles):**
  the existing volsync CRDs lack Helm ownership metadata, so Helm will *not* error-and-retry
  — it will hard-fail adoption. Since no CRs exist (verified), proactively
  `kubectl delete crd replicationsources.volsync.backube replicationdestinations.volsync.backube`
  so the chart installs them fresh with proper ownership. After this, `manageCRDs: true`
  keeps them upgraded automatically.

### 3. Backup config (centralized, SOPS-encrypted)
New `infrastructure/volsync/` directory, applied by a new Flux Kustomization
`flux/infrastructure/volsync-config.yaml` **with SOPS decryption** (copy the
`etcd-backup.yaml` Kustomization idiom: `decryption.provider: sops`, `secretRef: sops-age`,
`prune: true`; **no** `targetNamespace` — each resource declares its own namespace):

- **restic repo Secrets** (one per PVC, SOPS-encrypted, in the PVC's namespace). Each shares
  `RESTIC_PASSWORD`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_DEFAULT_REGION:
  us-east-1`, and a **unique** `RESTIC_REPOSITORY:
  s3:https://nas.i.recompiled.org:3900/volsync/<namespace>-<app>`. Encrypt only
  `stringData` per `.sops.yaml`; age recipient already configured.
- **ReplicationSource** per PVC (in the PVC's namespace):
  ```yaml
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata: { name: <app>, namespace: <ns> }
  spec:
    sourcePVC: <pvc-name>
    trigger: { schedule: "0 3 * * *" }   # staggered per app to avoid thundering herd
    restic:
      repository: <app>-restic            # the Secret name
      copyMethod: Snapshot
      volumeSnapshotClassName: linstor-r3
      cacheCapacity: 2Gi                  # restic metadata cache PVC (uses default SC linstor-r3)
      pruneIntervalDays: 14
      retain: { daily: 7, weekly: 5, monthly: 6 }
  ```
  Stagger `schedule` across apps (e.g. 3:00, 3:10, …) so snapshots/movers don't all fire at once.

### 4. Wire into Flux
Add `snapshot-controller.yaml`, `volsync.yaml`, `volsync-config.yaml` to
`flux/infrastructure/kustomization.yaml`. Recommended ordering via `dependsOn`:
volsync-config depends on both the `volsync` and `snapshot-controller` HelmReleases.

## Files to create
- `flux/infrastructure/snapshot-controller.yaml` (HelmRelease)
- `flux/infrastructure/volsync.yaml` (HelmRepository + HelmRelease)
- `flux/infrastructure/volsync-config.yaml` (Flux Kustomization w/ SOPS)
- `infrastructure/volsync/kustomization.yaml`
- `infrastructure/volsync/volume-snapshot-class.yaml`
- `infrastructure/volsync/<app>-secret.yaml` ×12 (SOPS-encrypted)
- `infrastructure/volsync/<app>-replicationsource.yaml` ×12
- Edit: `flux/infrastructure/kustomization.yaml`

## Pre-rollout gate: thin-pool headroom
`copyMethod: Snapshot` provisions a transient **clone PVC from the snapshot** per run
(replicated 3×, on the `linstor-thinpool` that lives on the system NVMe). Before the large
first backups (plex 20Gi, homebridge/grafana 10Gi), check free space:
`kubectl -n piraeus-datastore exec deploy/linstor-controller -- linstor storage-pool list`
and confirm free capacity comfortably exceeds the largest staggered clone. Staggering the
schedules keeps at most one large clone live at a time.

## Verification (staged — prove the riskiest link first)
1. **Snapshot path in isolation** (before any Volsync): after snapshot-controller + VSC are
   Ready, manually `VolumeSnapshot` the smallest PVC (`config-overseerr-0`):
   `kubectl wait volumesnapshot ... --for=jsonpath='{.status.readyToUse}'=true` and confirm
   `kubectl -n piraeus-datastore exec deploy/linstor-controller -- linstor snapshot list`.
   **If this fails, stop** — Volsync cannot work until it does.
2. **One backup end-to-end:** deploy only the overseerr ReplicationSource. Confirm
   `kubectl get replicationsource -n media` shows `lastSyncTime`/no errors, and the restic
   repo populates in S3 (`s3cmd ls s3://volsync/media-overseerr/` or
   `restic snapshots`).
3. **Restore test:** create a `ReplicationDestination` (restic) restoring that repo into a
   scratch PVC; mount and verify file contents match. Document this as the restore runbook.
4. **Roll out** the remaining 11 ReplicationSources; confirm each completes one cycle and the
   first (largest) plex/homebridge/grafana backups finish.
5. **Drift/GitOps:** `flux get kustomizations` and `flux get helmreleases -A` all Ready.

## Notes / trade-offs
- restic `copyMethod: Snapshot` gives crash-consistent point-in-time copies; for the sqlite-
  based *arr apps this is safe (WAL recovers on open). No app downtime required.
- Plex (20Gi) backs up whole-volume incl. thumbnail cache — large first backup, cheap
  thereafter via restic dedup. Confirmed in scope.
- Each ReplicationSource creates a transient snapshot + clone PVC + mover pod during its run;
  thin-pool snapshots are cheap but ensure thin-pool headroom for concurrent runs (staggering
  mitigates).
- Restore depends on the off-cluster age private key (decrypts SOPS → restic password). Keep
  it backed up independently of the cluster.

## Recommended follow-up (makes it "complete", not just "backups exist")
- **Alert on backup failure.** Volsync exports `volsync_*` metrics (last-sync time, duration,
  failures). Add a `PrometheusRule` (in `apps/o11y/`, like the existing recording rules) that
  fires when any ReplicationSource has had no successful sync in ~36h, surfaced through the
  existing Grafana alerting. Without this, a silently-stopped backup is invisible — the same
  failure class as the kube-state-metrics outage.

## Scope note
`kube-prometheus-stack-grafana` (10Gi) is included **beyond your three selections** because
its UI-managed dashboards/alerts are SQLite-only and not in S3. Drop its ReplicationSource +
secret if you'd rather exclude it.
