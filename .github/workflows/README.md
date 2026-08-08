# GitHub Actions Workflows

This directory contains automation workflows for the home-gitops repository.

## Table of Contents
- [docker-build.yml](#docker-buildyml) - Build and push cluster-multitool Docker image
- [validate.yml](#validateyml) - Validate GitOps manifests on PRs
- [gitguardian.yaml](#gitguardianyaml) - Scan for secrets in commits
- [ghcr-prune-cluster-multitool.yml](#ghcr-prune-cluster-multitoolyml) - Clean up old container images

---

## docker-build.yml

**Purpose:** Builds and pushes the cluster-multitool Docker image to GHCR when the Dockerfile changes.

**Triggers:** Push to main (when `tools/cluster-multitool/Dockerfile` changes)

**What it does:** Builds the cluster-multitool image (debugging tools for Kubernetes) and pushes it to `ghcr.io/jeversol/cluster-multitool:latest`. Tool versions in the Dockerfile are managed by Renovate.

---

## validate.yml

**Purpose:** Validates GitOps manifests before they reach the cluster.

**Triggers:** Push to main or pull requests that modify YAML files

**What it does:**
- Lints all YAML files with yamllint
- Tests all kustomize builds (infrastructure and apps)
- Checks that workloads have resource limits defined
- Validates Helm templates if any charts are present

---

## gitguardian.yaml

**Purpose:** Scans commits for accidentally committed secrets and credentials.

**Triggers:** All pushes and pull requests

**What it does:** Uses GitGuardian to scan for API keys, passwords, private keys, and other secrets. Fails the build if secrets are detected. If secrets are found, they must be revoked/rotated and removed from git history (not just the latest commit).

---

## ghcr-prune-cluster-multitool.yml

**Purpose:** Cleans up old Docker image versions from GitHub Container Registry.

**Triggers:** Weekly on Sundays at 03:00 UTC

**What it does:** Deletes old untagged versions of the cluster-multitool image, keeping only the 2 most recent versions. Tagged images like `latest` are preserved.
