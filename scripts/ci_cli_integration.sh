#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="${GDAM_CI_WORK_DIR:-$(mktemp -d)}"
SUPABASE_URL="${SUPABASE_URL:-http://127.0.0.1:54421}"
SUPABASE_PUBLISHABLE_KEY="${SUPABASE_PUBLISHABLE_KEY:-sb_publishable_ACJWlzQHlZjBrEguHvfOxg_3BJgxAaH}"
export SUPABASE_URL
export SUPABASE_PUBLISHABLE_KEY
DEV_EMAIL="${GDAM_DEV_EMAIL:-dev@gdam.local}"
DEV_PASSWORD="${GDAM_DEV_PASSWORD:-password123}"
ADDON_NAME="${GDAM_TEST_ADDON_NAME:-gdam-test-addon}"
RUNTIME_ADDON_NAME="${GDAM_TEST_RUNTIME_ADDON_NAME:-gdam-test-runtime}"
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
    -H "Prefer: return=representation" \
    -d "$payload" \
    "$url"
}

api_patch() {
  local url="$1"
  local payload="$2"
  curl -sS -f -X PATCH \
    -H "apikey: $SUPABASE_PUBLISHABLE_KEY" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "Content-Type: application/json" \
    -H "Prefer: return=representation" \
    -d "$payload" \
    "$url"
}

api_upsert() {
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

upsert_plugin() {
  local name="$1"
  local editor_plugin="$2"
  local payload name_encoded existing existing_id response

  payload="$(jq -cn \
    --arg user_id "$USER_ID" \
    --arg name "$name" \
    --arg repo "$ADDON_REPO" \
    --argjson editor_plugin "$editor_plugin" \
    '{user_id:$user_id,org_id:null,name:$name,repo:$repo,editor_plugin:$editor_plugin}')"
  name_encoded="$(jq -rn --arg value "$name" '$value|@uri')"
  existing="$(api_get "$SUPABASE_URL/rest/v1/plugins?select=id&user_id=eq.$USER_ID&name=eq.$name_encoded&limit=1")"
  existing_id="$(jq -r '.[0].id // empty' <<<"$existing")"
  if [[ -n "$existing_id" ]]; then
    response="$(api_patch "$SUPABASE_URL/rest/v1/plugins?id=eq.$existing_id" "$payload")"
  else
    response="$(api_post "$SUPABASE_URL/rest/v1/plugins" "$payload")"
  fi

  jq -r '.[0].id' <<<"$response"
}

upsert_version() {
  local plugin_id="$1"
  local release_tag="$2"
  local asset_name="$3"
  local payload

  payload="$(jq -cn \
    --arg plugin_id "$plugin_id" \
    --arg release_tag "$release_tag" \
    --arg asset_name "$asset_name" \
    '{plugin_id:$plugin_id,major:0,minor:1,patch:0,release_tag:$release_tag,asset_name:$asset_name}')"
  api_upsert "$SUPABASE_URL/rest/v1/plugin_versions?on_conflict=plugin_id,major,minor,patch" "$payload" >/dev/null
}

assert_project_has_plugin() {
  local addon_name="$1"
  if ! grep -q "res://addons/@dev_${addon_name}/plugin.cfg" project.godot; then
    printf 'expected editor plugin entry for %s in project.godot\n' "$addon_name" >&2
    exit 1
  fi
}

assert_project_lacks_plugin() {
  local addon_name="$1"
  if grep -q "res://addons/@dev_${addon_name}/plugin.cfg" project.godot; then
    printf 'unexpected editor plugin entry for %s in project.godot\n' "$addon_name" >&2
    exit 1
  fi
}

expect_failure() {
  if "$@" >/dev/null 2>&1; then
    printf 'expected command to fail: %s\n' "$*" >&2
    exit 1
  fi
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
PLUGIN_ID="$(upsert_plugin "$ADDON_NAME" true)"
RUNTIME_PLUGIN_ID="$(upsert_plugin "$RUNTIME_ADDON_NAME" false)"
upsert_version "$PLUGIN_ID" "$ADDON_SHA" "@aviorstudio_gdam-test-addon.zip"
upsert_version "$RUNTIME_PLUGIN_ID" "$ADDON_SHA" "@aviorstudio_gdam-test-addon.zip"

cd "$GODOT_DIR"
"$ROOT_DIR/cli/bin/gdam" init
test -f gdam.json

expect_failure "$ROOT_DIR/cli/bin/gdam" add "@dev/$ADDON_NAME@0.1"
expect_failure "$ROOT_DIR/cli/bin/gdam" add "@dev/does-not-exist@0.1.0"

"$ROOT_DIR/cli/bin/gdam" add "@dev/$ADDON_NAME@0.1.0"
test -f "addons/@dev_${ADDON_NAME}/plugin.cfg"
jq -e --arg addon "@dev/$ADDON_NAME" '.addons[$addon].editor_plugin == true' gdam.json >/dev/null
assert_project_has_plugin "$ADDON_NAME"

rm -rf "addons/@dev_${ADDON_NAME}"
"$ROOT_DIR/cli/bin/gdam" install
test -f "addons/@dev_${ADDON_NAME}/plugin.cfg"
assert_project_has_plugin "$ADDON_NAME"

"$ROOT_DIR/cli/bin/gdam" add "@dev/$RUNTIME_ADDON_NAME@0.1.0"
test -f "addons/@dev_${RUNTIME_ADDON_NAME}/plugin.cfg"
jq -e --arg addon "@dev/$RUNTIME_ADDON_NAME" '.addons[$addon].editor_plugin != true' gdam.json >/dev/null
assert_project_lacks_plugin "$RUNTIME_ADDON_NAME"

rm -rf "addons/@dev_${RUNTIME_ADDON_NAME}"
"$ROOT_DIR/cli/bin/gdam" install
test -f "addons/@dev_${RUNTIME_ADDON_NAME}/plugin.cfg"
assert_project_lacks_plugin "$RUNTIME_ADDON_NAME"

"$ROOT_DIR/cli/bin/gdam" remove "@dev/$ADDON_NAME"
test ! -e "addons/@dev_${ADDON_NAME}"
assert_project_lacks_plugin "$ADDON_NAME"

"$ROOT_DIR/cli/bin/gdam" remove "@dev/$RUNTIME_ADDON_NAME"
test ! -e "addons/@dev_${RUNTIME_ADDON_NAME}"

LOCAL_ADDON="$WORK_DIR/local-addon"
mkdir -p "$LOCAL_ADDON"
printf '[plugin]\nname="Local Test"\n' > "$LOCAL_ADDON/plugin.cfg"

BAD_LOCAL_ADDON="$WORK_DIR/not-addon"
mkdir -p "$BAD_LOCAL_ADDON"
expect_failure "$ROOT_DIR/cli/bin/gdam" link @dev/bad-local "$BAD_LOCAL_ADDON"
expect_failure "$ROOT_DIR/cli/bin/gdam" link @dev/local-addon

"$ROOT_DIR/cli/bin/gdam" link @dev/local-addon "$LOCAL_ADDON"
test -L addons/@dev_local-addon
jq -e --arg addon '@dev/local-addon' --arg path "$LOCAL_ADDON" '.addons[$addon].enabled == true and .addons[$addon].path == $path' gdam.link.json >/dev/null

"$ROOT_DIR/cli/bin/gdam" unlink @dev/local-addon
test ! -e addons/@dev_local-addon
jq -e --arg addon '@dev/local-addon' --arg path "$LOCAL_ADDON" '.addons[$addon].enabled == false and .addons[$addon].path == $path' gdam.link.json >/dev/null

"$ROOT_DIR/cli/bin/gdam" link @dev/local-addon
test -L addons/@dev_local-addon

"$ROOT_DIR/cli/bin/gdam" unlink @dev/local-addon
test ! -e addons/@dev_local-addon

printf 'CLI integration passed in %s\n' "$GODOT_DIR"
