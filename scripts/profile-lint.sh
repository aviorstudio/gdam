#!/usr/bin/env bash
set -euo pipefail
export GOTOOLCHAIN=local
unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then printf '%s\n' "$unformatted"; exit 1; fi
go vet ./...
shellcheck scripts/*.sh
actionlint
