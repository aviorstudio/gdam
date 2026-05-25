#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="${GDAM_CI_WORK_DIR:-$(mktemp -d)}"
SUPABASE_URL="${SUPABASE_URL:-http://127.0.0.1:54421}"
SUPABASE_PUBLISHABLE_KEY="${SUPABASE_PUBLISHABLE_KEY:-sb_publishable_ACJWlzQHlZjBrEguHvfOxg_3BJgxAaH}"
DEV_EMAIL="${GDAM_DEV_EMAIL:-dev@gdam.local}"
DEV_PASSWORD="${GDAM_DEV_PASSWORD:-password123}"
ADDON_NAME="${GDAM_TEST_ADDON_NAME:-gdam-test-addon}"
ADDON_REPO="${GDAM_TEST_ADDON_REPO:-https://github.com/aviorstudio/gdam-test-addon}"
GODOT_REPO="${GDAM_TEST_GODOT_REPO:-https://github.com/aviorstudio/gdam-test-godot}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

api_get() {
  curl -sS -f \
    -H "apikey: $SUPABASE_PUBLISHABLE_KEY" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    "$@"
}

api_post() {
  local url="$1"
  local payload="$2"
  curl -sS -f \
    -H "apikey: $SUPABASE_PUBLISHABLE_KEY" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "Content-Type: application/json" \
    -H "Prefer: resolution=merge-duplicates,return=representation" \
    -d "$payload" \
    "$url"
}

require_cmd curl
require_cmd git
require_cmd jq

"$ROOT_DIR/scripts/db_seed.sh" >/dev/null
"$ROOT_DIR/scripts/cli_build.sh"

auth_response="$(curl -sS -f \
  -H "apikey: $SUPABASE_PUBLISHABLE_KEY" \
  -H "Content-Type: application/json" \
  -d "$(jq -cn --arg email "$DEV_EMAIL" --arg password "$DEV_PASSWORD" '{email:$email,password:$password}')" \
  "$SUPABASE_URL/auth/v1/token?grant_type=password")"
ACCESS_TOKEN="$(jq -r '.access_token' <<<"$auth_response")"
USER_ID="$(jq -r '.user.id' <<<"$auth_response")"

if [[ -z "$ACCESS_TOKEN" || "$ACCESS_TOKEN" == "null" || -z "$USER_ID" || "$USER_ID" == "null" ]]; then
  printf 'failed to sign in seeded user\n' >&2
  exit 1
fi

mkdir -p "$WORK_DIR"
ADDON_DIR="$WORK_DIR/addon"
GODOT_DIR="$WORK_DIR/godot"

git clone --depth 1 "$ADDON_REPO" "$ADDON_DIR"
git clone --depth 1 "$GODOT_REPO" "$GODOT_DIR"

if [[ ! -f "$ADDON_DIR/plugin.cfg" ]]; then
  printf 'test addon repo must contain plugin.cfg at its root: %s\n' "$ADDON_REPO" >&2
  exit 1
fi

ADDON_SHA="$(git -C "$ADDON_DIR" rev-parse HEAD)"
plugin_payload="$(jq -cn \
  --arg user_id "$USER_ID" \
  --arg name "$ADDON_NAME" \
  --arg repo "$ADDON_REPO" \
  '{user_id:$user_id,org_id:null,name:$name,repo:$repo,path:null,editor_plugin:true}')"
plugin_response="$(api_post "$SUPABASE_URL/rest/v1/plugins?on_conflict=user_id,name" "$plugin_payload")"
PLUGIN_ID="$(jq -r '.[0].id' <<<"$plugin_response")"

version_payload="$(jq -cn \
  --arg plugin_id "$PLUGIN_ID" \
  --arg sha "$ADDON_SHA" \
  '{plugin_id:$plugin_id,major:0,minor:1,patch:0,sha:$sha}')"
api_post "$SUPABASE_URL/rest/v1/plugin_versions?on_conflict=plugin_id,major,minor,patch" "$version_payload" >/dev/null

cd "$GODOT_DIR"
"$ROOT_DIR/cli/bin/gdam" init
test -f gdam.json

"$ROOT_DIR/cli/bin/gdam" add "@dev/$ADDON_NAME@0.1.0"
test -f "addons/@dev_${ADDON_NAME}/plugin.cfg"

if ! grep -q "res://addons/@dev_${ADDON_NAME}/plugin.cfg" project.godot; then
  printf 'expected editor plugin entry in project.godot\n' >&2
  exit 1
fi

"$ROOT_DIR/cli/bin/gdam" remove "@dev/$ADDON_NAME"
test ! -e "addons/@dev_${ADDON_NAME}"

LOCAL_ADDON="$WORK_DIR/local-addon"
mkdir -p "$LOCAL_ADDON"
printf '[plugin]\nname="Local Test"\n' > "$LOCAL_ADDON/plugin.cfg"

"$ROOT_DIR/cli/bin/gdam" link @dev/local-addon "$LOCAL_ADDON"
test -L addons/@dev_local-addon

"$ROOT_DIR/cli/bin/gdam" unlink @dev/local-addon
test ! -e addons/@dev_local-addon

printf 'CLI integration passed in %s\n' "$GODOT_DIR"
