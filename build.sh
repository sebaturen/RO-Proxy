#!/bin/bash
set -e

echo "Building RO-Proxy for Linux amd64..."
GOOS=linux GOARCH=amd64 go build -o proxy ./cmd/proxy
GOOS=linux GOARCH=amd64 go build -o analyzer ./cmd/analyzer
echo "Done: proxy, analyzer"
