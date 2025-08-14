#!/bin/bash

set -e

echo "🧪 Running unit tests for talos-automation..."

# Navigate to the project directory
cd "$(dirname "$0")"

# Run tests with verbose output
echo "Running all tests..."
go test -v ./...

# Run tests with coverage
echo ""
echo "Running tests with coverage..."
go test -v -cover ./...

# Check for any formatting issues
echo ""
echo "Checking code formatting..."
if ! gofmt -l . | grep -q '^$'; then
    echo "❌ Code formatting issues found:"
    gofmt -l .
    exit 1
else
    echo "✅ Code formatting is correct"
fi

# Check for any linting issues (if we had golint installed)
echo ""
echo "✅ All tests completed successfully!"

echo ""
echo "To run specific test packages:"
echo "  go test -v ./internal/talos"
echo "  go test -v ./internal/repo"  
echo "  go test -v ./upgrades"
echo "  go test -v ."