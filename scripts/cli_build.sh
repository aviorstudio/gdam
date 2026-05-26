#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

mkdir -p "$ROOT_DIR/cli/bin"

cd "$ROOT_DIR/cli"
SUPABASE_PUBLISHABLE_KEY="${GDAM_SUPABASE_PUBLISHABLE_KEY:-${SUPABASE_PUBLISHABLE_KEY:-}}"

ldflags=()
if [[ -n "$SUPABASE_PUBLISHABLE_KEY" ]]; then
  ldflags+=("-X" "github.com/aviorstudio/gdam/cli/internal/gdamdb.DefaultSupabasePublishableKey=$SUPABASE_PUBLISHABLE_KEY")
fi

go build -ldflags "${ldflags[*]}" -o "$ROOT_DIR/cli/bin/gdam" ./cmd/gdam
