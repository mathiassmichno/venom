#!/bin/bash

# Build script for Venom project
set -e

echo "Building Venom project..."

# Create bin directory if it doesn't exist
mkdir -p bin

# Build daemon
echo "Building venomd..."
go build -o bin/venomd ./cmd/venomd

# Build client
echo "Building venomctl..."
go build -o bin/venomctl ./clients/go/venomctl

echo "Build complete!"
echo "Binaries:"
ls -la bin/

echo ""
echo "Usage:"
echo "  ./bin/venomd :9988                    # Start daemon"
echo "  ./bin/venomctl --addr localhost:9988  # Use client"
