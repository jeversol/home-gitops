# SeaweedFS S3 Storage

This directory contains the SeaweedFS deployment for S3-compatible object storage.

## Architecture

- **Master**: 1 replica (5Gi Longhorn PVC)
- **Volume**: 1 replica (3Ti democratic-iscsi PVC from Synology)
- **Filer**: 1 replica with embedded LevelDB (2Gi Longhorn PVC)
- **S3**: Enabled on filer (port 8333)
- **Total Capacity**: 3Ti (single volume, Synology RAID-5 backend)
- **Replication**: "000" (no replication, data protected by Synology RAID-5)
- **Total Pods**: 3 (master + volume + filer)

## Next Steps (Manual Testing)

### 1. Encrypt S3 Credentials

Edit `s3-credentials-secret.yaml` and replace all `CHANGE_ME_*` placeholders with actual access/secret keys:

```bash
# Generate strong random keys
openssl rand -base64 32  # For each access key
openssl rand -base64 48  # For each secret key

# Edit the file
vi apps/seaweedfs/s3-credentials-secret.yaml

# Encrypt with SOPS
sops -e -i apps/seaweedfs/s3-credentials-secret.yaml

# Verify encryption
head -20 apps/seaweedfs/s3-credentials-secret.yaml
```

### 2. Deploy Manually (Outside GitOps)

```bash
# Apply namespace and secrets first
kubectl apply -f apps/seaweedfs/namespace.yaml
kubectl apply -f apps/seaweedfs/s3-credentials-secret.yaml

# Add Helm repo
helm repo add seaweedfs https://seaweedfs.github.io/seaweedfs/helm
helm repo update

# Install SeaweedFS manually for testing
kubectl create namespace seaweedfs  # if not exists
helm install seaweedfs seaweedfs/seaweedfs \
  --namespace seaweedfs \
  --values <(kubectl kustomize apps/seaweedfs | yq '.spec.values' apps/seaweedfs/helm-release.yaml)

# Apply ingress and certificates
kubectl apply -f apps/seaweedfs/s3-ingressroute.yaml
kubectl apply -f apps/seaweedfs/s3-test-tls-cert.yaml
```

### 3. Verify Deployment

```bash
# Check all pods are running
kubectl get pods -n seaweedfs

# Check PVCs are bound
kubectl get pvc -n seaweedfs

# Check services
kubectl get svc -n seaweedfs

# Check certificate
kubectl get certificate -n seaweedfs

# Check ingress
kubectl get ingressroute -n seaweedfs

# Check DNS resolution
nslookup s3-test.i.recompiled.org
```

### 4. Install weed CLI

```bash
# Download latest release
wget https://github.com/seaweedfs/seaweedfs/releases/latest/download/darwin_amd64.tar.gz
tar -xzf darwin_amd64.tar.gz
sudo mv weed /usr/local/bin/
chmod +x /usr/local/bin/weed

# Test connection
weed shell -master=seaweedfs-master-0.seaweedfs-master.seaweedfs:9333
```

### 5. Test S3 Functionality

```bash
# Install mc (MinIO Client) if needed
brew install minio/stable/mc

# Configure endpoint
mc alias set swfs-test https://s3-test.i.recompiled.org \
  ADMIN_ACCESS_KEY ADMIN_SECRET_KEY

# Create test bucket
mc mb swfs-test/test-bucket

# Upload test file
echo "Hello SeaweedFS" > test.txt
mc cp test.txt swfs-test/test-bucket/

# Download and verify
mc cp swfs-test/test-bucket/test.txt test-downloaded.txt
cat test-downloaded.txt

# Check bucket stats
mc du swfs-test/test-bucket

# Test multipart upload (large file)
dd if=/dev/zero of=largefile.bin bs=1M count=200
mc cp largefile.bin swfs-test/test-bucket/

# Cleanup
rm test.txt test-downloaded.txt largefile.bin
mc rb --force swfs-test/test-bucket
```

### 6. Check Metrics

SeaweedFS exposes Prometheus metrics on port 9327 for all components:
- Master: `seaweedfs-master.seaweedfs.svc:9327/metrics`
- Volume: `seaweedfs-volume.seaweedfs.svc:9327/metrics`
- Filer: `seaweedfs-filer.seaweedfs.svc:9327/metrics`

Test metrics:
```bash
kubectl port-forward -n seaweedfs svc/seaweedfs-master 9327:9327
curl http://localhost:9327/metrics
```

### 7. Add Alloy Scraping (After Testing)

Add to `flux/apps/alloy.yaml` in the configMap content:

```yaml
// Scrape SeaweedFS metrics
prometheus.scrape "seaweedfs" {
  targets = discovery.relabel.seaweedfs.output
  forward_to = [prometheus.relabel.seaweedfs_final.receiver]
}

discovery.relabel "seaweedfs" {
  targets = discovery.kubernetes.services.targets

  rule {
    source_labels = ["__meta_kubernetes_service_name"]
    regex         = "seaweedfs-(master|volume|filer)"
    action        = "keep"
  }

  rule {
    source_labels = ["__meta_kubernetes_namespace"]
    regex         = "seaweedfs"
    action        = "keep"
  }

  rule {
    source_labels = ["__meta_kubernetes_service_name", "__meta_kubernetes_namespace"]
    separator     = ";"
    regex         = "(.+);(.+)"
    target_label  = "__address__"
    replacement   = "$1.$2.svc.cluster.local:9327"
    action        = "replace"
  }

  rule {
    source_labels = ["__meta_kubernetes_service_name"]
    target_label  = "component"
  }
}

prometheus.relabel "seaweedfs_final" {
  forward_to = [prometheus.remote_write.mimir.receiver]

  rule {
    replacement  = "seaweedfs"
    target_label = "job"
  }

  rule {
    replacement  = "talos"
    target_label = "cluster"
  }
}
```

### 8. Import Grafana Dashboard

After successful deployment:
1. Visit Grafana at https://grafana.i.recompiled.org
2. Go to Dashboards → Import
3. Enter dashboard ID: `10423`
4. Select Prometheus data source: Mimir
5. Import

### 9. Enable in GitOps (After Successful Testing)

Uncomment in `flux/apps/kustomization.yaml`:
```yaml
# - helmrepo-seaweedfs.yaml
# - seaweedfs.yaml
```

Commit and push:
```bash
git add -A
git commit -m "feat: add SeaweedFS S3 storage (tested manually)"
git push
```

## Troubleshooting

### Pods not starting
```bash
kubectl describe pod -n seaweedfs <pod-name>
kubectl logs -n seaweedfs <pod-name>
```

### PVC not binding
```bash
kubectl describe pvc -n seaweedfs
# Check if democratic-iscsi and longhorn storage classes exist
kubectl get storageclass
```

### Certificate not issuing
```bash
kubectl describe certificate -n seaweedfs s3-test-tls
kubectl get challenges -n seaweedfs
kubectl logs -n cert-manager deployment/cert-manager
```

### DNS not resolving
```bash
kubectl logs -n external-dns deployment/external-dns
# Check Cloudflare for DNS records
```

### S3 access denied
```bash
# Check secret is created and contains valid JSON
kubectl get secret -n seaweedfs seaweedfs-s3-config -o yaml
# Decode and verify JSON
kubectl get secret -n seaweedfs seaweedfs-s3-config -o jsonpath='{.data.config\.json}' | base64 -d | jq .
```

## Migration from Garage

See plan document at `/Users/joe/.claude/plans/floofy-wibbling-seal.md` for detailed migration strategy.
