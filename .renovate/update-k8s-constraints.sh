#!/usr/bin/env bash
#
# Renovate postUpgradeTasks wrapper for updating Kubernetes version constraints
#
# This script is called by Renovate after updating Talos version.
# It validates the Talos version argument and calls the Python script to update
# the Kubernetes allowedVersions constraint in renovate.json5.
#
# Usage: ./update-k8s-constraints.sh <talos-version>
# Example: ./update-k8s-constraints.sh 1.12
#
# Exit codes:
#   0 - Success (constraint updated or already up to date)
#   1 - Error (validation failed or script execution failed)

set -euo pipefail

TALOS_VERSION="${1:-}"

# Validate that Talos version was provided
if [ -z "$TALOS_VERSION" ]; then
    echo "ERROR: Talos version not provided" >&2
    echo "Usage: $0 <talos-version>" >&2
    echo "Example: $0 1.12" >&2
    exit 1
fi

# Validate version format (should be X.Y)
if ! [[ "$TALOS_VERSION" =~ ^[0-9]+\.[0-9]+$ ]]; then
    echo "ERROR: Invalid Talos version format: $TALOS_VERSION" >&2
    echo "Expected format: X.Y (e.g., 1.12)" >&2
    exit 1
fi

echo "=== Updating Kubernetes constraints for Talos v${TALOS_VERSION}.x ==="
echo ""

# Get script directory (so we can find the Python script)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Call Python script with the Talos version
# Don't let set -e kill us if exit code is 2 (no changes)
set +e
python3 "$SCRIPT_DIR/update-k8s-constraints.py" "$TALOS_VERSION"
exit_code=$?
set -e

echo ""

# Handle exit codes
if [ $exit_code -eq 2 ]; then
    echo "=== No changes needed - constraint already up to date ==="
    exit 0  # Renovate treats this as success
elif [ $exit_code -eq 0 ]; then
    echo "=== Successfully updated Kubernetes version constraints ==="
    exit 0
else
    echo "=== ERROR: Failed to update constraints ===" >&2
    exit 1
fi
