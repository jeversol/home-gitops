# home-gitops

This repo manages my home kubernetes cluster using Flux.


| Node | HW Model | CPU | RAM | Storage | Network |
| ---- | -------- | --- | --- | ------- | ------- |
| Node 1 | GMKTek NucBox G3 Plus | Intel N150 | 16GB | 512GB SSD | 2.5Gbit |
| Node 2 | HP EliteDesk 800 G4 Mini | Intel Core i5-8500T | 16GB | 256GB SSD | 1Gbit |
| Node 3 | HP ProDesk 600 G4 Mini | Intel Core i5-8500T | 16GB | 256GB SSD | 1Gbit |

## Deployed and Migrated Services:
- Infrastructure
  - [FluxCD](https://fluxcd.io/)
  - [SOPS with age](https://getsops.io)
  - [MetalLB](https://metallb.io)
  - [Traefik](https://doc.traefik.io/traefik/)
  - [external-secrets](https://external-secrets.io/latest/)
  - [cert-manager](https://cert-manager.io)
  - [harry-botter](https://github.com/apps/harry-botter-lumos)
    - homegrown script for secrets due to expire, based on annotations
  - [democratic-csi](https://github.com/democratic-csi/democratic-csi)
  - [longhorn](https://longhorn.io)
- Observability
  - Mimir
  - Alloy
  - Grafana
- Applications
  - [tautulli](https://tautulli.com)

### Deployed but backed out/replaced

<details> 
<summary>Replaced ArgoCD with FluxCD</summary>   

I originally started out using Flux for GitOps as it had a lower learning curve. When I decided to switch from k8s on Ubuntu to a Talos Linux cluster, I decided to also use Argo, because it has broad adoption in the enterprise landscape. 

Initially, it was going well. I got a good flow of being able to test my deployments before committing them, dealing with some issues, etc. However, it completely collapsed after converting to a 3 node cluster.

After upgrading the cluster from 1 control-plane and 1 worker to 3 control-plane nodes, I started having permissions issues internally... logging into the webui as admin and trying to drill into an application would kick me back to the login page. 
  
Using ChatGPT to help drill through some diagnostic steps, it appeard to be some sort of service account and token issue. Deleting argocd from the k8s cluster and reinstalling it from scratch wouldn't fix it. A workaround involved creating a custom service account, generating a token for it, extracting the jwt token and giving it to a Secret.

Honestly, this felt too painful. I had spent hours ruling out SSO, RBAC, a broken Redis cache, problems with the ArgoCD HA deployment versus non-HA. ChatGPT suggested a bootstrapping script that created the secret and all of that, but then it said "Oh, that token is only good for 1 hour. Do you want a token that lasts a year?" 

That was when I decided it was too much and went back to Flux.  

</details>

## Up Next:

- o11y: prom/grafana/loki/mimir/alloy

## To Migrate:
* Grafana (from Grafana Cloud)
* Scrutiny (from NAS to grow to a hub/spoke)
* Homebridge (from NAS)
### Media Stack:
* Plex first with hardware decoding
* Request Stack: overseerr/doplarr
* Search Stack: flaresolverr/prowlarr
* Management: Radarr/Sonarr
* sabNZBd (might stay on NAS?)
* qbittorrent (stay on NAS with sabnzbd?)

## To keep where they are:
* minio on NAS
* public website on linode
* airsonic on NAS (sonos is too fragile)