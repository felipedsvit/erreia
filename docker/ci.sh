#!/usr/bin/env bash
set -euo pipefail

# Return ownership of generated files to the host user (only under compose).
if [ -n "${HOST_UID:-}" ]; then
	trap 'chown -R "${HOST_UID}:${HOST_GID:-$HOST_UID}" "$PWD" 2>/dev/null || true' EXIT
fi

GOLANGCI_VERSION="${GOLANGCI_VERSION:-v2.3.0}"

echo "==> templ generate"
go tool templ generate

echo "==> go vet"
go vet ./...

echo "==> golangci-lint ${GOLANGCI_VERSION}"
# Build from source with the active Go toolchain (1.25). The prebuilt binaries
# are compiled with an older Go and refuse a module targeting go 1.25.
if ! command -v golangci-lint >/dev/null 2>&1; then
	go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_VERSION}"
fi
"$(go env GOPATH)/bin/golangci-lint" run --timeout=5m

echo "==> unit tests"
make test

echo "==> integration tests"
make test-integration

echo "CI OK"
