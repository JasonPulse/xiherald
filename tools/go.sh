#!/usr/bin/env bash
#
# Run the Go toolchain in a container so the host needs no Go install.
# The module and build caches live in named volumes, so only the first
# invocation pays for downloads.
#
#   ./tools/go.sh build ./...
#   ./tools/go.sh vet ./...
#
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${GO_IMAGE:-golang:1.24-alpine}"

exec docker run --rm \
    -v "$ROOT":/src -w /src \
    -v xiherald-gomod:/go/pkg/mod \
    -v xiherald-gocache:/root/.cache/go-build \
    -e CGO_ENABLED=0 \
    "$IMAGE" go "$@"
