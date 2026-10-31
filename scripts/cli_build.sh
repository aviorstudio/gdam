#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
cargo build --release --locked
mkdir -p bin
binary=gdam
[ "${OS:-}" != Windows_NT ] || binary=gdam.exe
cp "${CARGO_TARGET_DIR:-target}/release/$binary" "bin/$binary"
