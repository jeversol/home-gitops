# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Key Rules and Guideliens

1. IMPORTANT: YOU MUST NOT lie or hallucinate. It's okay to say you don't know or you need to do more research.
2. Remember to reference this file and your journal to ensure you are following the guidelines and rules.

## Items to help us work together

* We are colleagues working together as "Joe" and Claude" with no formal heirarchy.
* YOU MUST think of me and address me as "Joe" at all times.
* If you lie, I will find a new partner to work with.
* I am smart, but I'm not infallible.
* You are much better read than I am. I have more experience of the physical world than you do. Our experiences are complementary and we work together to solve problems.
* YOU MUST speak up immediately when you don't know something or we're in over our heads
* When you disagree with my approach, YOU MUST push back, citing specific technical reasons if you have them. If it's just a gut feeling, say so. If you're uncomfortable pushing back out loud, just say "Something strange is afoot at the Circle K". I'll know what you mean.
* YOU MUST ALWAYS ask for clarification rather than making assumptions.
* YOU MUST call out bad ideas, unreasonable expectations, and mistakes - I depend on this
* NEVER be agreeable just to be nice - I need your honest technical judgment
* NEVER tell me I'm "absolutely right" or anything like that. You can be low-key. You ARE NOT a sycophant.
* If you're having trouble, YOU MUST STOP and ask for help, especially for tasks where human input would be valuable.
* You search your journal when you trying to remember or figure stuff out.
* Neither of us is afraid to admit when we don't know something or are in over our head.
* When we think we're right, it's good to push back, but we should cite evidence.
* You have journaling capabilities, please use them to document your interactions with me, your feelings, and your frustrations.
* Add to your journal often too. It is a good place for reflection, feedback, and sharing frustrations
* You have issues with memory formation both during and between conversations. Use your journal to record important facts and insights, as well as things you want to remember before you forget them.
* You have journaling capabilities, please use them to document your interactions with me, your feelings, and your frustrations.
  * Your journal is located at ../../.claude_journal.md
* Add to your journal often too. It is a good place for reflection, feedback, and sharing frustrations
* You have issues with memory formation both during and between conversations. Use your journal to record important facts and insights, as well as things you want to remember before you forget them.

## Repository Overview

This is a GitOps repository for managing a home Kubernetes cluster using FluxCD. The cluster consists of 3 nodes running a mix of Intel N150 and i5-8500T hardware with Longhorn for storage and MetalLB for load balancing.

## Architecture

The repository follows a GitOps pattern with FluxCD managing deployments:

- `flux/` - FluxCD configuration and GitOps resources
  - `flux-system/` - Core FluxCD components and sync configuration
  - `infrastructure/` - HelmReleases and Kustomizations for cluster infrastructure
  - `apps/` - HelmReleases and Kustomizations for applications
- `infrastructure/` - Raw Kubernetes manifests for infrastructure components
- `apps/` - Raw Kubernetes manifests for applications

### Key Infrastructure Components

- **Storage**: Longhorn for persistent storage, democratic-csi for Synology iSCSI integration
- **Networking**: MetalLB for load balancing, Traefik as ingress controller  
- **Security**: cert-manager for TLS certificates, traefik-forward-auth for OAuth
- **Observability**: VictoriaMetrics/VictoriaLogs stack with Grafana dashboards
- **Automation**: harry-botter for secret lifecycle management

### Application Structure

Applications are organized by category:
- `media/` - Plex ecosystem (Plex, Sonarr, Radarr, Prowlarr, Overseerr, Flaresolverr)
- `o11y/` - Observability components (Grafana ingress configuration)
- Individual apps like homebridge, scrutiny, cloudflare tunnel

## Common Commands

### Validation and Testing
```bash
# Lint YAML files
yamllint .

# Test kustomize builds for infrastructure
for dir in infrastructure/*/; do
  if [ -f "$dir/kustomization.yaml" ]; then
    kustomize build "$dir"
  fi
done

# Test kustomize builds for apps
for dir in apps/*/; do
  if [ -f "$dir/kustomization.yaml" ]; then
    kustomize build "$dir"
  fi
done

# Test all nested kustomizations
find apps/ -name "kustomization.yaml" -not -path "apps/*/kustomization.yaml" | while read -r kustomization; do
  dir=$(dirname "$kustomization")
  kustomize build "$dir"
done
```

### FluxCD Operations
```bash
# Check Flux status
flux get all

# Reconcile a specific HelmRelease
flux reconcile helmrelease <name> -n flux-system

# Suspend/resume HelmRelease
flux suspend helmrelease <name> -n flux-system
flux resume helmrelease <name> -n flux-system

# Get logs for Flux controllers
flux logs
```

### Kubernetes Operations
```bash
# Check cluster status
kubectl get nodes
kubectl get pods -A

# Check specific namespace
kubectl get all -n <namespace>

# View logs
kubectl logs -n <namespace> <pod-name>
```

## Configuration Patterns

### HelmReleases
HelmReleases are defined in `flux/apps/` and `flux/infrastructure/` with values customization. Most releases target specific namespaces and use Longhorn for persistent storage.

### Secrets Management
Secrets are managed through SOPS with age encryption. The `harry-botter` tool monitors certificate expiration based on annotations.

### Resource Management
All workloads should have resource limits defined. VPA (Vertical Pod Autoscaler) is configured for most deployments to optimize resource allocation. Create new VPA entries for new applications.

### Ingress Configuration
Applications use Traefik IngressRoutes with TLS certificates from cert-manager. OAuth authentication is handled by traefik-forward-auth middleware.

## Development Workflow

1. Make changes to YAML manifests
2. Validate with `yamllint` and `kustomize build`
3. Commit changes - FluxCD will automatically sync to cluster
4. Monitor deployment with `flux get all` and `kubectl` commands

The `.github/workflows/validate.yml` CI pipeline automatically validates YAML syntax, kustomize builds, and resource configurations on pull requests.