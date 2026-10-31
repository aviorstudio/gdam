#!/usr/bin/env bash
set -euo pipefail
python3 scripts/verify-vendor.py
cargo fmt --package gdam -- --check
cargo clippy --locked --all-targets -- -D warnings
shellcheck scripts/*.sh
actionlint
