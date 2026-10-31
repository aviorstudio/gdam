#!/usr/bin/env bash
set -euo pipefail
cargo fmt --all -- --check
cargo clippy --locked --all-targets -- -D warnings
shellcheck scripts/*.sh
actionlint
