#!/usr/bin/env bash
set -e

echo "==> Setting up Go compiler..."
export GO_VERSION="1.22.7"

if [ ! -f "/tmp/go/bin/go" ]; then
    echo "==> Downloading Go ${GO_VERSION} for Linux amd64..."
    curl -sL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -xz -C /tmp
fi

export PATH="/tmp/go/bin:$PATH"
export CGO_ENABLED=0

echo "==> Go version: $(go version)"
echo "==> Building server binary..."
go build -ldflags="-s -w" -o bin/server ./cmd/server

echo "==> Build successful: bin/server ready!"
