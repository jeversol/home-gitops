# This tool has (already) been deprecated in favor of system-upgrade-controller

# Talos Automation Deployment

This directory contains everything needed to deploy the talos-automation webhook service.

## Quick Setup on Synology

1. **Copy this entire directory to Synology:**
   ```bash
   rsync -av tools/talos-automation/ user@synology:/volume2/docker/talos-automation/
   ```

2. **On Synology, complete the setup:**
   ```bash
   cd /volume2/docker/talos-automation
   
   # Create the actual .env file from the example
   cp .env.example .env
   # Edit .env with your actual secrets
   
   # Copy your talosconfig file
   cp /path/to/your/talosconfig ./talosconfig
   
   # Create logs directory
   mkdir -p logs
   
   # Build the image
   docker build -t talos-automation .
   
   # Start the service
   docker-compose up -d
   
   # Check logs
   docker-compose logs -f
   ```

## Environment Variables Required

- `GITHUB_WEBHOOK_SECRET`: Secret for GitHub webhook verification
- `GITHUB_TOKEN`: Personal access token for private repo access
- `GITHUB_OWNER`: Your GitHub username (jeversol)
- `GITHUB_REPO`: Repository name (home-gitops)
- `TALOS_CONFIG_PATH`: Path to talosconfig inside container (/talos/talosconfig)
- `LOG_PATH`: Path for upgrade logs (/app/logs)
- `DIAGNOSTICS_TOKEN`: Bearer token for accessing diagnostics endpoint

## Testing

The primary testing method is the `/diagnostics` endpoint which runs comprehensive integration tests in the actual containerized environment with real dependencies.

### Integration Testing with /diagnostics Endpoint

The service provides a protected `/diagnostics` endpoint for comprehensive integration testing without executing actual upgrade commands.

#### Basic Testing
```bash
# Set your diagnostics token
export DIAGNOSTICS_TOKEN="your-secret-token-here"

# Test all components with no upgrades needed (default versions match repo)
curl -H "Authorization: Bearer $DIAGNOSTICS_TOKEN" https://talos-webhook.recompiled.org/diagnostics

# Test health check (no auth required)
curl https://talos-webhook.recompiled.org/health
```

#### Testing Upgrade Scenarios
You can simulate different version scenarios using URL parameters to test what would happen if your cluster was running different versions:

```bash
# Test Kubernetes upgrade (simulate cluster currently running older version)
curl -H "Authorization: Bearer $DIAGNOSTICS_TOKEN" "https://talos-webhook.recompiled.org/diagnostics?current_k8s=1.32.0"

# Test Talos upgrade (simulate cluster currently running older version) 
curl -H "Authorization: Bearer $DIAGNOSTICS_TOKEN" "https://talos-webhook.recompiled.org/diagnostics?current_talos=1.9.0"

# Test both upgrades needed
curl -H "Authorization: Bearer $DIAGNOSTICS_TOKEN" "https://talos-webhook.recompiled.org/diagnostics?current_k8s=1.32.0&current_talos=1.9.0"

# Test downgrade protection
curl -H "Authorization: Bearer $DIAGNOSTICS_TOKEN" "https://talos-webhook.recompiled.org/diagnostics?current_k8s=1.34.0"

# Test invalid version handling
curl -H "Authorization: Bearer $DIAGNOSTICS_TOKEN" "https://talos-webhook.recompiled.org/diagnostics?current_k8s=invalid.version"

# Use legacy scenario parameter (optional, parameters override scenarios)
curl -H "Authorization: Bearer $DIAGNOSTICS_TOKEN" "https://talos-webhook.recompiled.org/diagnostics?scenario=both-upgrade"
```

#### Understanding Test Results
The `/diagnostics` endpoint returns JSON with these sections:
- `github_api`: Tests GitHub API connection and version fetching
- `talos_config`: Tests talosconfig parsing and node detection  
- `bare_metal_config`: Tests fetching bare-metal configuration
- `image_factory`: Tests Image Factory API integration
- `cluster_versions`: Tests current cluster version detection
- `k8s_upgrade_test`: Tests Kubernetes upgrade logic (dry-run)
- `talos_upgrade_test`: Tests Talos upgrade logic (dry-run)  
- `upgrade_decisions`: Shows what upgrades would be performed
- `summary`: Overall test status and parameters used

#### Key Testing Features
- **Identical Logic**: Test mode uses the exact same code paths as production
- **Safe Testing**: `executeCommands=false` prevents actual command execution
- **Real Integration**: Still calls GitHub API and Image Factory for real testing
- **Custom Versions**: Set any current version to test upgrade/downgrade scenarios
- **Comprehensive Coverage**: Tests all components from API calls to upgrade decisions

## Files in this directory

- `Dockerfile`: Container build instructions
- `docker-compose.yaml`: Service definition
- `.env.example`: Template for environment variables
- `main.go`: Main webhook server
- `internal/`: Internal packages (talos config, GitHub API)
- `upgrades/`: Upgrade logic for Kubernetes and Talos
