#!/bin/bash

# Configuration
GRAFANA_URL="https://grafana.i.recompiled.org"
DASHBOARD_DIR="./out/dashboards"

# Get API token from encrypted file or environment variable
if [ -f "./grafana-token.txt" ]; then
    echo "Loading API token from encrypted file..."
    GRAFANA_API_TOKEN=$(sops -d grafana-token.txt)
elif [ -n "$GRAFANA_API_TOKEN" ]; then
    echo "Using API token from environment variable..."
else
    echo "Error: No API token found"
    echo "Either set GRAFANA_API_TOKEN environment variable or create encrypted grafana-token.txt"
    echo "To create encrypted token file:"
    echo "  echo 'your-token-here' > grafana-token.txt"
    echo "  sops -e -i grafana-token.txt"
    exit 1
fi


# Skip unwanted dashboards  
UNWANTED_DASHBOARDS=("nodes-aix" "k8s-resources-windows-cluster" "k8s-resources-windows-namespace" "k8s-resources-windows-pod" "k8s-windows-cluster-rsrc-use" "k8s-windows-node-rsrc-use" "node-cluster-rsrc-use" "node-rsrc-use" "proxy")

# Import each dashboard
echo "Importing dashboards from $DASHBOARD_DIR..."
for dashboard_file in "$DASHBOARD_DIR"/*.json; do
    if [ ! -f "$dashboard_file" ]; then
        echo "No dashboard files found in $DASHBOARD_DIR"
        exit 1
    fi
    
    dashboard_name=$(basename "$dashboard_file" .json)
    
    # Skip unwanted dashboards
    skip=false
    for unwanted in "${UNWANTED_DASHBOARDS[@]}"; do
        if [ "$dashboard_name" = "$unwanted" ]; then
            echo "⏭ Skipping unwanted dashboard: $dashboard_name"
            skip=true
            break
        fi
    done
    
    if [ "$skip" = true ]; then
        continue
    fi
    
    echo "Importing $dashboard_name..."
    
    # Read dashboard JSON and prepare import payload
    dashboard_json=$(cat "$dashboard_file")
    
    # Prepare import payload
    import_payload=$(echo "$dashboard_json" | jq '{
        dashboard: (. | del(.id)),
        overwrite: true,
        message: "Imported from mixin"
    }')
    
    # Import dashboard
    response=$(curl -s -X POST \
        -H "Authorization: Bearer $GRAFANA_API_TOKEN" \
        -H "Content-Type: application/json" \
        -d "$import_payload" \
        "$GRAFANA_URL/api/dashboards/db")
    
    # Check response
    status=$(echo "$response" | jq -r '.status // empty')
    if [ "$status" = "success" ]; then
        echo "✓ Successfully imported $dashboard_name"
    else
        echo "✗ Failed to import $dashboard_name"
        echo "Response: $response"
    fi
done

echo "Dashboard import complete!"
echo ""
echo "To update dashboards in the future:"
echo "0. Run: jb update"
echo "1. Modify ~/code/grafana-mixins/mixin.libsonnet"
echo "2. Run: cd ~/code/grafana-mixins && mixtool generate dashboards -d out/dashboards mixin.libsonnet"  
echo "3. Run: ./import-dashboards.sh"
