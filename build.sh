#!/bin/bash
set -e

echo "Building ROProxy..."
go build -o roproxy cmd/roproxy/main.go

echo "Build complete: ./roproxy"
echo ""
echo "Usage:"
echo "  sudo ./roproxy"
echo ""
echo "Make sure config.json exists (copy from config.example.json if needed)"
