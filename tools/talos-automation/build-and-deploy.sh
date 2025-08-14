#!/bin/bash

# Talos Automation Build and Deploy Script
# This script builds the Docker image and saves it for deployment to Synology

set -e  # Exit on any error

echo "🔍 Checking Docker availability..."

# Check if Docker daemon is running
if ! docker info >/dev/null 2>&1; then
    echo "❌ Docker daemon not running!"
    echo ""
    echo "Please start OrbStack:"
    echo "1. Open OrbStack application"
    echo "2. Wait for Docker to start"
    echo "3. Run this script again"
    echo ""
    echo "Or start from command line:"
    echo "   open -a OrbStack"
    exit 1
fi

echo "✅ Docker daemon is running"
echo "🔨 Building talos-automation Docker image..."
docker build --platform linux/amd64 -t talos-automation .

echo "💾 Saving Docker image to tar file..."
docker save talos-automation > talos-automation.tar

echo "📤 Transferring to Synology..."
scp talos-automation.tar joe@nas:/docker/talos-automation

echo "🚀 Deploying on Synology..."
ssh joe@nas "cd /volume2/docker/talos-automation && /usr/local/bin/docker load < talos-automation.tar && /usr/local/bin/docker-compose down && /usr/local/bin/docker-compose up -d"

echo "📋 Checking deployment status..."
ssh joe@nas "cd /volume2/docker/talos-automation && /usr/local/bin/docker-compose ps"

echo ""
echo "✅ Deployment complete!"
echo ""
echo "To check logs:"
echo "   ssh joe@nas 'cd /volume2/docker/talos-automation && /usr/local/bin/docker-compose logs -f'"