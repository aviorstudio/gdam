#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

mkdir -p "$ROOT_DIR/bin"

cd "$ROOT_DIR"

# No credentials are injected at build time any more. The CLI reads the public
# registry API and authenticates publishes with the user's own secret key, so a
# locally built binary behaves exactly like a released one.
go build -o "$ROOT_DIR/bin/gdam" ./cmd/gdam
