# home-gitops

This repo manages my home kubernetes cluster using Flux.

| Node | HW Model | CPU | RAM | Storage | Network |
| ---- | -------- | --- | --- | ------- | ------- |
| Node 1 | GMKTek NucBox G3 Plus | Intel N150 | 16GB | 512GB SSD | 2.5Gbit |
| Node 2 | HP EliteDesk 800 G4 Mini | Intel Core i5-8500T | 16GB | 256GB SSD | 2.5Gbit |
| Node 3 | HP ProDesk 600 G4 Mini | Intel Core i5-8500T | 16GB | 256GB SSD | 2.5Gbit |

## About This Repository

This serves as a learning environment for cloud-native technologies and GitOps practices. The focus is on implementing production-ready infrastructure patterns, security practices, and operational complexity using real-world workloads.

The infrastructure implements:
- Complete GitOps workflow with FluxCD (via the Flux Operator)
- Production security practices (SOPS encryption, cert-manager, OAuth)
- Comprehensive observability (Mimir, Grafana, Loki, Alerting)
- Storage orchestration (Piraeus/LINSTOR, NFS CSI, Volsync backups)
- Advanced Kubernetes features (VPA, descheduler, node feature discovery)
- Automated Talos and Kubernetes upgrades

## Deployed and Migrated Services:
- Infrastructure
  - [FluxCD](https://fluxcd.io/) via [Flux Operator](https://github.com/controlplaneio-fluxcd/flux-operator) - GitOps continuous delivery, with a web UI
  - [SOPS with age](https://getsops.io) - Secret encryption/decryption
  - [Cilium](https://cilium.io/) - CNI, network policy, load balancing for bare metal, and Hubble network observability UI
  - [Traefik](https://doc.traefik.io/traefik/) - Reverse proxy and ingress controller
  - [cert-manager](https://cert-manager.io) - Automatic TLS certificate management
  - [harry-botter](https://github.com/jeversol/harry-botter) - Kubernetes secret expiry monitoring with GitHub issue alerts
  - [Piraeus/LINSTOR](https://github.com/piraeusdatastore/piraeus-operator) - Distributed block storage (DRBD-backed), with a LINSTOR GUI
  - [NFS CSI Driver](https://github.com/kubernetes-csi/csi-driver-nfs) - NFS storage support
  - [Snapshot Controller](https://github.com/kubernetes-csi/external-snapshotter) - CSI volume snapshot support
  - [Volsync](https://volsync.readthedocs.io/) - Automated PVC backup/restore for stateful apps
  - [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) - Cloudflare tunnel for secure ingress
  - [external-dns](https://github.com/kubernetes-sigs/external-dns) - Automatic DNS record management
  - [CloudNative-PG](https://cloudnative-pg.io/) - PostgreSQL operator for Kubernetes
  - [VPA](https://github.com/kubernetes/autoscaler/tree/master/vertical-pod-autoscaler) - Vertical Pod Autoscaler for resource optimization
  - [Descheduler](https://github.com/kubernetes-sigs/descheduler) - Pod rescheduling for better cluster utilization
  - [Node Feature Discovery](https://kubernetes-sigs.github.io/node-feature-discovery/) - Hardware feature detection
  - [Intel GPU Plugin](https://intel.github.io/intel-device-plugins-for-kubernetes/) - Intel GPU resource management
  - [tuppr](https://github.com/home-operations/tuppr) - Automated Talos and Kubernetes version upgrades
  - [metrics-server](https://github.com/kubernetes-sigs/metrics-server) - Cluster resource metrics API
  - [etcd-backup](https://etcd.io) - Automated etcd backups to S3-compatible storage
  - [flux-webhook](https://fluxcd.io/flux/components/notification/receivers/) - GitHub webhook receiver for Flux
  - [traefik-forward-auth](https://github.com/thomseddon/traefik-forward-auth) - OAuth authentication middleware
  - [Renovate](https://docs.renovatebot.com/) - Automated dependency updates with webhook integration
- Observability
  - [Grafana](https://grafana.com/) - Visualization and dashboards with Auth0 integration
  - [Mimir](https://grafana.com/oss/mimir/) - Long-term metrics storage, querying, and alerting
  - [Loki](https://grafana.com/oss/loki/) - Log aggregation with S3 backend and 30-day retention
  - [Alloy](https://grafana.com/docs/alloy/) - Observability data collector for logs and metrics
  - [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics) - Kubernetes object metrics
  - [Smartctl Exporter](https://github.com/prometheus-community/smartctl_exporter) - Disk SMART metrics for Prometheus
- Applications
  - [Tautulli](https://tautulli.com) - Plex monitoring and analytics
  - [Scrutiny](https://github.com/AnalogJ/scrutiny) - Hard drive health monitoring with InfluxDB backend
  - [Homebridge](https://homebridge.io) - HomeKit bridge for smart home integration
  - [Spoolman](https://github.com/Donkie/Spoolman) - 3D printing filament inventory management, with automatic Bambu printer filament tracking
  - **Media Services**
    - [Plex](https://www.plex.tv/) - Media server
    - [Overseerr](https://overseerr.dev/) - Media request management
    - [Sonarr](https://sonarr.tv/) - TV show automation
    - [Radarr](https://radarr.video/) - Movie automation
    - [Prowlarr](https://prowlarr.com/) - Indexer manager
    - [FlareSolverr](https://github.com/FlareSolverr/FlareSolverr) - Web scraping helper for media automation

