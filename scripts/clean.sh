#!/bin/bash

# Clean script for Venom project
set -e

echo "Cleaning Venom project..."

# Remove binaries
rm -rf bin/

# Remove generated code
rm -f proto/generated/go/*.go
rm -rf proto/generated/python/*

# Remove Python build artifacts
rm -rf clients/python/dist/
rm -rf clients/python/build/
rm -rf clients/python/*.egg-info/

# Remove Go build artifacts
go clean -cache -modcache -testcache

echo "Clean complete!"