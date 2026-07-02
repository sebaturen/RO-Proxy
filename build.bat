@echo off
echo Building RO-Proxy for Linux amd64...
set GOOS=linux
set GOARCH=amd64
go build -o proxy ./cmd/proxy
go build -o analyzer ./cmd/analyzer
echo Done: proxy, analyzer
