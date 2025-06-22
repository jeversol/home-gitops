# home-argocd

This repo provides GitOps functionality for my home kubernetes cluster via ArgoCD.

The first revision of this was using FluxCD and k3s on a cheap Intel N150-based mini PC from Amazon. 
This now-current version will be a 3 node cluster running Talos Linux for the OS.

| Node | HW Model | CPU | RAM | Storage | Network |
| ---- | -------- | --- | --- | ------- | ------- |
| Node 1 | GMKTek NucBox G3 Plus | Intel N150 | 16GB | 512GB SSD | 2.5Gbit |
| Node 2 | HP EliteDesk 800 G4 Mini | Intel Core i5-8500T | 16GB | 256GB SSD | 1Gbit |
| Node 3 | HP ProDesk 600 G4 Mini | Intel Core i5-8500T | 16GB | 256GB SSD | 1Gbit |

## Deployed and Migrated Services:
 - ArgoCD
 - MetalLB
 - Traefik
 - external-secrets
 - cert-manager
 - harry-botter
  - python script in a configmap, using a github app bot to create issues.

## To migrate from cluster 1.0
- democratic-csi 
  - Allows mounting and creation of iSCSI volumes and SMB shares on Synology
- tautulli

## Up Next:
- prom/grafana/loki/mimir/alloy

## To Migrate:
* Grafana (from Grafana Cloud) - in progress
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
