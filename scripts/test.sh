#!/bin/bash

# Test script for Venom project
set -e

echo "Running Venom tests..."

# Go tests
echo "Running Go tests..."
go test ./daemon/...
go test ./clients/go/...

# Python tests (if available)
if [ -d "clients/python/tests" ]; then
    echo "Running Python tests..."
    cd clients/python && python -m pytest tests/ -v
fi

echo "All tests passed!"