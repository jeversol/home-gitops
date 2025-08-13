# home-gitops

This repo manages my home kubernetes cluster using Flux.

| Node | HW Model | CPU | RAM | Storage | Network |
| ---- | -------- | --- | --- | ------- | ------- |
| Node 1 | GMKTek NucBox G3 Plus | Intel N150 | 16GB | 512GB SSD | 2.5Gbit |
| Node 2 | HP EliteDesk 800 G4 Mini | Intel Core i5-8500T | 16GB | 256GB SSD | 2.5Gbit |
| Node 3 | HP ProDesk 600 G4 Mini | Intel Core i5-8500T | 16GB | 256GB SSD | 2.5Gbit |

## Deployed and Migrated Services:
- Infrastructure
  - [FluxCD](https://fluxcd.io/) - GitOps continuous delivery
  - [SOPS with age](https://getsops.io) - Secret encryption/decryption
  - [MetalLB](https://metallb.io) - Load balancer for bare metal
  - [Traefik](https://doc.traefik.io/traefik/) - Reverse proxy and ingress controller
  - [cert-manager](https://cert-manager.io) - Automatic TLS certificate management
  - [harry-botter](https://github.com/jeversol/harry-botter) - Certificate expiry monitoring
  - [democratic-csi](https://github.com/democratic-csi/democratic-csi) - Synology iSCSI storage integration
  - [Longhorn](https://longhorn.io) - Distributed block storage
  - [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) - Cloudflare tunnel for secure ingress
  - [external-dns](https://github.com/kubernetes-sigs/external-dns) - Automatic DNS record management
  - [VPA](https://github.com/kubernetes/autoscaler/tree/master/vertical-pod-autoscaler) - Vertical Pod Autoscaler for resource optimization
  - [Descheduler](https://github.com/kubernetes-sigs/descheduler) - Pod rescheduling for better cluster utilization
  - [Node Feature Discovery](https://kubernetes-sigs.github.io/node-feature-discovery/) - Hardware feature detection
  - [Intel GPU Plugin](https://intel.github.io/intel-device-plugins-for-kubernetes/) - Intel GPU resource management
  - [NFS CSI Driver](https://github.com/kubernetes-csi/csi-driver-nfs) - NFS storage support
  - [metrics-server](https://github.com/kubernetes-sigs/metrics-server) - Cluster resource metrics API
  - [etcd-backup](https://etcd.io) - Automated etcd backups to S3-compatible storage
  - [flux-webhook](https://fluxcd.io/flux/components/notification/receivers/) - GitHub webhook receiver for Flux
  - [traefik-forward-auth](https://github.com/thomseddon/traefik-forward-auth) - OAuth authentication middleware
- Observability
  - [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
    - [Grafana](https://grafana.com/) - Visualization and dashboards with Auth0 integration
    - [Prometheus](https://prometheus.io/) - Metrics collection and alerting
    - [Alertmanager](https://prometheus.io/docs/alerting/latest/alertmanager/) - Alert routing and management
- Applications
  - [Tautulli](https://tautulli.com) - Plex monitoring and analytics
  - [Scrutiny](https://github.com/AnalogJ/scrutiny) - Hard drive health monitoring with InfluxDB backend
  - [Homebridge](https://homebridge.io) - HomeKit bridge for smart home integration
  - **Media Services**
    - [Plex](https://www.plex.tv/) - Media server
    - [Overseerr](https://overseerr.dev/) - Media request management
    - [Sonarr](https://sonarr.tv/) - TV show automation
    - [Radarr](https://radarr.video/) - Movie automation
    - [Prowlarr](https://prowlarr.com/) - Indexer manager
    - [FlareSolverr](https://github.com/FlareSolverr/FlareSolverr) - Web scraping helper for media automation

### Deployed but backed out and replaced

- ArgoCD (replaced by FluxCD)
- external-secrets (replaced by SOPS in FluxCD)
- VictoriaMetrics/VictoriaLogs/Vector Logs (replaced by kube-prometheus-stack)

<details> 
<summary>Replaced ArgoCD with FluxCD</summary>   

I originally started out using Flux for GitOps as it had a lower learning curve. When I decided to switch from k8s on Ubuntu to a Talos Linux cluster, I decided to also use Argo, because it has broad adoption in the enterprise landscape. 

Initially, it was going well. I got a good flow of being able to test my deployments before committing them, dealing with some issues, etc. However, it completely collapsed after converting to a 3 node cluster.

After upgrading the cluster from 1 control-plane and 1 worker to 3 control-plane nodes, I started having permissions issues internally... logging into the webui as admin and trying to drill into an application would kick me back to the login page. 
  
Using ChatGPT to help drill through some diagnostic steps, it appeard to be some sort of service account and token issue. Deleting argocd from the k8s cluster and reinstalling it from scratch wouldn't fix it. A workaround involved creating a custom service account, generating a token for it, extracting the jwt token and giving it to a Secret.

Honestly, this felt too painful. I had spent hours ruling out SSO, RBAC, a broken Redis cache, problems with the ArgoCD HA deployment versus non-HA. ChatGPT suggested a bootstrapping script that created the secret and all of that, but then it said "Oh, that token is only good for 1 hour. Do you want a token that lasts a year?" 

That was when I decided it was too much and went back to Flux.  

</details>