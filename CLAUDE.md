# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is a **home Kubernetes cluster GitOps repository** managing a 3-node Talos Linux cluster using FluxCD. The architecture follows enterprise GitOps patterns with proper secret management and infrastructure automation.

## Architecture & GitOps Patterns

### Directory Structure
- `flux/` - FluxCD configurations (Kustomization references)
  - `flux-system/` - Core Flux components
  - `infrastructure/` - Infrastructure component references
  - `apps/` - Application references
- `infrastructure/` - Actual infrastructure manifests
- `apps/` - Application manifests

### Deployment Pattern
Two-tier GitOps deployment:
1. `flux/` directory contains FluxCD Kustomization references
2. `infrastructure/` and `apps/` contain actual Kubernetes manifests
3. FluxCD watches main branch every 2 minutes and auto-deploys changes

## Key Technologies

- **FluxCD** - GitOps operator
- **Talos Linux** - Kubernetes-focused OS
  - Expected version of Talos and Kubernetes are referenced in @infrastructure/cluster/track-versions.yaml
- **Kustomize** - Configuration management
- **SOPS with age** - Secret encryption
- **MetalLB** - Bare-metal load balancer
- **Traefik** - Ingress controller
- **Longhorn** - Distributed storage
- **democratic-csi** - Provision iSCSI storage from Synology NAS
- **cert-manager** - TLS certificate automation
- **Victoria Metrics** - Prometheus-compatible metrics
- **Grafana** - Observability dashboards

## Secret Management

All secrets use **SOPS encryption** with age keys:
- Secrets are encrypted in git with `sops -e -i secret.yaml`
- FluxCD decrypts using the `sops-age` secret
- Secrets have expiration annotations for tracking
- **Harry Botter** (custom Python app) monitors secret expiration and creates GitHub issues based off the recompiled.org/expiry-date annotation

## Common Workflows

### Adding New Applications
1. Create manifest files in `apps/[app-name]/`
2. Add Kustomization in `flux/apps/[app-name].yaml`
3. Update `flux/apps/kustomization.yaml` to include new app
4. Flux automatically deploys within 2 minutes

### Infrastructure Changes
1. Modify files in `infrastructure/[component]/`
2. Update corresponding `flux/infrastructure/[component].yaml` if needed
3. Changes are automatically applied by Flux

### Secret Operations
1. Create secret YAML with proper expiration annotations
2. Encrypt: `sops -e -i secret.yaml`
3. Commit encrypted secret to git
4. Harry Botter monitors and alerts on expiration
  1. This is accomplished by the recompiled.org/expiry-date and recompiled.org/expiry-note annotations.
  2. An example of this is infrastructure/cert-manager/cert-manager-secret.yaml

## Renovate Configuration

The repository uses Renovate for dependency updates with specific manager configurations:
- **Flux manager** - Handles Flux component versions
- **Kubernetes manager** - Excludes `flux/flux-system/gotk*.yaml` files to prevent conflicts
- **Custom managers** - Track Talos and Kubernetes versions, which are manually updated outside of Flux

## Hardware Context

3-node bare-metal cluster with control plane scheduling enabled:
- Mixed Intel hardware (N150, i5-8500T CPUs)
- 16GB RAM, SSD storage per node
- Custom Talos system extensions for hardware support

## Build & Development

- **Flux manages deployments** - Both manifests via kustomize and Helm charts via helmreleases crd.
- **GitHub Actions** - Builds custom containers (Harry Botter)
- **Git-based workflow** - GitHub as source of truth
- **Encrypted secrets** safely committed to git

## Miscellaneous items to remember
- We are not working on the cluster nodes directly, so `kubectl port-forward` is not viable.
- We use traefik IngressRoutes for exposing services outside the cluster where feasible
- Github Pre-commit hooks exist to run yamllint
