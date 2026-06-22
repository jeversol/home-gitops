# Linstor PVC Backups + CNPG barman-cloud Migration Plan

## Context

The cluster runs 3-way replicated Linstor storage via Piraeus but has no off-cluster backup for PVC data. The S3-compatible NAS at `nas.i.recompiled.org:3900` (SeaweedFS) will become the backup target. CNPG v1.29.1 is deployed and its current `barmanObjectStore` config is deprecated — this migration adds the `barman-cloud` plugin.

Volsync CRDs (`replicationsources.volsync.backube`, `replicationdestinations.volsync.backube`) are already installed on the cluster but the controller is not deployed. VolumeSnapshot CRDs are also installed but no snapshot-controller or VolumeSnapshotClass is configured.

---

## Scope: PVCs to Back Up via Volsync

| Namespace  | PVC                           | Size  |
|------------|-------------------------------|-------|
| homebridge | storage-homebridge-0          | 10Gi  |
| media      | config-plex-0                 | 20Gi  |
| media      | config-radarr-0               | 2Gi   |
| media      | config-sonarr-0               | 2Gi   |
| media      | config-prowlarr-0             | 2Gi   |
| media      | config-tautulli-0             | 5Gi   |
| media      | config-overseerr-0            | 256Mi |
| o11y       | config-scrutiny-0             | 500Mi |
| o11y       | kube-prometheus-stack-grafana | 10Gi  |
| spoolman   | data-spoolman-0               | 2Gi   |
| spoolman   | bambu-spoolman-config         | 1Gi   |

**Excluded from Volsync**: `spoolman/spoolman-postgres-1` is protected by CNPG's barman WAL archiving. Observability TSDBs (Mimir/Loki/InfluxDB) excluded as reconstructible.

Grafana's PVC contains UI-managed alerts (not in git) — included in scope.

---

## Step 1: NAS Setup (user action — before implementation)

Create the following on the NAS SeaweedFS instance:

1. **S3 key** for Volsync — save the AccessKeyID and SecretAccessKey
2. **Bucket** `linstor-volsync` — grant the Volsync key read/write/owner access
3. **Existing bucket** `pg-spoolman` — no changes needed; CNPG uses the existing `spoolman-postgres-backup` secret

---

## Step 2: Deploy snapshot-controller

Volsync needs a snapshot-controller to create crash-consistent point-in-time copies (`copyMethod: Snapshot`) before uploading to S3.

**New files**:

`flux/infrastructure/snapshot-controller.yaml` — HelmRepository (`oci://ghcr.io/piraeusdatastore/helm-charts`) + HelmRelease for `snapshot-controller` chart, namespace `snapshot-controller`, targeting `infrastructure/snapshot-controller`.

`infrastructure/snapshot-controller/volumesnapshotclass.yaml`:
```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: linstor-csi-snapclass
  annotations:
    snapshot.storage.kubernetes.io/is-default-class: "true"
driver: linstor.csi.linbit.com
deletionPolicy: Delete
```

> **Verify**: Confirm `snapshot-controller` chart exists in the Piraeus OCI registry; fallback is `https://piraeus.io/helm-charts/`.
>
> **CRD ownership**: VolumeSnapshot CRDs already exist on-cluster. Set `installCRDs: false` in the HelmRelease values to prevent ownership conflicts.

---

## Step 3: Deploy Volsync Operator

`flux/infrastructure/volsync.yaml` — HelmRepository (`https://backube.github.io/volsync-charts/`) + HelmRelease for `volsync` chart, namespace `volsync-system`, `dependsOn: piraeus-operator`.

---

## Step 4: Per-App Volsync Configuration

### Secrets

Volsync requires **one restic repository per ReplicationSource** — shared repos cause lock contention and prune/retention corruption. For multi-PVC namespaces, use one secret per PVC with a unique repository path; AWS key material can be shared, only `RESTIC_REPOSITORY` and `RESTIC_PASSWORD` differ.

Example (`apps/media/volsync-secret-plex.yaml`, SOPS-encrypted):
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: volsync-restic-plex
  namespace: media
stringData:
  RESTIC_REPOSITORY: s3:https://nas.i.recompiled.org:3900/linstor-volsync/media-plex
  RESTIC_PASSWORD: <unique-passphrase>
  AWS_ACCESS_KEY_ID: <k8s-volsync-key-id>
  AWS_SECRET_ACCESS_KEY: <k8s-volsync-secret>
```

`media` namespace secrets: `volsync-restic-plex`, `-radarr`, `-sonarr`, `-prowlarr`, `-tautulli`, `-overseerr` — each with path `linstor-volsync/media-<app>`.

Single-PVC namespaces (`homebridge`, `scrutiny`, etc.) use one secret per namespace.

### ReplicationSource template

```yaml
apiVersion: volsync.backube/v1alpha1
kind: ReplicationSource
metadata:
  name: <pvc-name>
  namespace: <app-ns>
spec:
  sourcePVC: <pvc-name>
  trigger:
    schedule: "0 <hour> * * *"
  restic:
    repository: <secret-name>
    copyMethod: Snapshot
    volumeSnapshotClassName: linstor-csi-snapclass
    storageClassName: linstor-r3
    retain:
      daily: 7
      weekly: 4
      monthly: 3
```

### Staggered schedule

| Time         | Namespace / Apps                          |
|--------------|-------------------------------------------|
| `0 1 * * *`  | homebridge                                |
| `0 2 * * *`  | spoolman (data-spoolman-0, bambu-spoolman)|
| `0 3 * * *`  | media/plex                                |
| `0 4 * * *`  | media/radarr, media/sonarr                |
| `0 5 * * *`  | media/prowlarr, tautulli, overseerr       |
| `0 6 * * *`  | o11y/scrutiny, o11y/grafana               |

### Files per app

| Location              | Files                                                      |
|-----------------------|------------------------------------------------------------|
| `apps/homebridge/`    | `volsync-secret.yaml`, `replication-source.yaml`          |
| `apps/media/`         | `volsync-secret-{plex,radarr,sonarr,prowlarr,tautulli,overseerr}.yaml`, 6× `replication-source-*.yaml` |
| `apps/scrutiny/`      | `volsync-secret.yaml`, `replication-source.yaml`          |
| `apps/o11y/`          | `volsync-secret-grafana.yaml`, `replication-source-grafana.yaml` |
| `apps/spoolman/`      | `volsync-secret-{data,bambu}.yaml`, 2× `replication-source-*.yaml` |

Add each new file to the namespace `kustomization.yaml`.

---

## Step 5: CNPG barman-cloud Plugin Migration

CNPG 1.29.1 deprecates `backup.barmanObjectStore` in favor of a separately-deployed plugin that communicates with the operator via gRPC.

### a) Deploy the plugin

> **Verify**: Confirm whether `cnpg-plugin-barman-cloud` is in the existing `cloudnative-pg.github.io/charts` repo or needs a separate HelmRepository.
>
> **cert-manager**: The plugin uses cert-manager for gRPC TLS — add `dependsOn: cert-manager` to the HelmRelease.

Add to `flux/infrastructure/cloudnative-pg.yaml`:
```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: cnpg-plugin-barman-cloud
  namespace: flux-system
spec:
  interval: 30m
  targetNamespace: cloudnative-pg
  dependsOn:
    - name: cloudnative-pg
    - name: cert-manager
  chart:
    spec:
      chart: cnpg-plugin-barman-cloud
      sourceRef:
        kind: HelmRepository
        name: cloudnative-pg   # verify source
```

### b) Create ObjectStore CR (`apps/spoolman/objectstore.yaml`)

```yaml
apiVersion: barmancloud.cnpg.io/v1
kind: ObjectStore
metadata:
  name: spoolman-postgres-backup
  namespace: spoolman
spec:
  configuration:
    destinationPath: "s3://pg-spoolman/spoolman/"
    endpointURL: "https://nas.i.recompiled.org:3900"
    s3Credentials:
      accessKeyId:
        name: spoolman-postgres-backup
        key: AWS_ACCESS_KEY_ID
      secretAccessKey:
        name: spoolman-postgres-backup
        key: AWS_SECRET_ACCESS_KEY
    wal:
      compression: gzip
    data:
      compression: gzip
  retentionPolicy: 14d
```

> **Verify exact schema** against the plugin chart version — `configuration` nesting and `retentionPolicy` placement shift between versions.

### c) Update Cluster CR (`apps/spoolman/postgres-cluster.yaml`)

Replace `backup.barmanObjectStore` with:
```yaml
spec:
  plugins:
    - name: barman-cloud.cloudnative-pg.io
      parameters:
        barmanObjectName: spoolman-postgres-backup
```

### d) Update ScheduledBackup (`apps/spoolman/spoolman-postgres-scheduled-backup.yaml`)

The `ScheduledBackup` CR must also declare the plugin method — without this it defaults to `barmanObjectStore` (removed) and silently stops working:
```yaml
spec:
  schedule: "30 3 * * *"
  backupOwnerReference: self
  method: plugin
  pluginConfiguration:
    name: barman-cloud.cloudnative-pg.io
  cluster:
    name: spoolman-postgres
```

### e) After deploy

Force a new base backup and confirm WAL archiving resumes:
```
kubectl cnpg backup spoolman-postgres -n spoolman
```

Watch PVC free space during cutover — the postgres PVC is 2Gi and WAL can accumulate if archiving stalls:
```
kubectl exec -it spoolman-postgres-1 -n spoolman -- df -h /var/lib/postgresql/data
```

---

## Verification

1. `kubectl get volumesnapshotclass` → `linstor-csi-snapclass` present
2. `kubectl get replicationsource -A` → `LAST SYNC TIME` populates after first run
3. Manual trigger: annotate a source with `volsync.backube/trigger-immediate-sync: "true"`
4. **Restore test**: Create a `ReplicationDestination` for `bambu-spoolman-config` (smallest PVC) targeting a scratch PVC — confirm restic restores cleanly
5. `kubectl get objectstore -n spoolman` → Ready; check S3 for new base backup directory
6. If barman or restic returns region errors: set `region: us-east-1` (any non-empty value) — SeaweedFS requires a region but ignores its content
