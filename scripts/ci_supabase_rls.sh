#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SUPABASE_URL="${SUPABASE_URL:-http://127.0.0.1:54421}"
SUPABASE_PUBLISHABLE_KEY="${SUPABASE_PUBLISHABLE_KEY:-sb_publishable_ACJWlzQHlZjBrEguHvfOxg_3BJgxAaH}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

sign_in() {
  local email="$1"
  local password="$2"
  curl -sS -f \
    -H "apikey: $SUPABASE_PUBLISHABLE_KEY" \
    -H "Content-Type: application/json" \
    -d "$(jq -cn --arg email "$email" --arg password "$password" '{email:$email,password:$password}')" \
    "$SUPABASE_URL/auth/v1/token?grant_type=password"
}

rest_status() {
  local method="$1"
  local token="$2"
  local path="$3"
  local payload="${4:-}"
  local tmp status

  tmp="$(mktemp)"
  if [[ -n "$payload" ]]; then
    status="$(curl -sS -o "$tmp" -w '%{http_code}' -X "$method" \
      -H "apikey: $SUPABASE_PUBLISHABLE_KEY" \
      -H "Authorization: Bearer $token" \
      -H "Content-Type: application/json" \
      -H "Prefer: return=representation" \
      -d "$payload" \
      "$SUPABASE_URL/rest/v1/$path")"
  else
    status="$(curl -sS -o "$tmp" -w '%{http_code}' -X "$method" \
      -H "apikey: $SUPABASE_PUBLISHABLE_KEY" \
      -H "Authorization: Bearer $token" \
      "$SUPABASE_URL/rest/v1/$path")"
  fi
  printf '%s\n' "$status"
  cat "$tmp"
  rm -f "$tmp"
}

expect_status() {
  local expected="$1"
  local actual="$2"
  local label="$3"

  if [[ "$actual" != "$expected" ]]; then
    printf 'expected %s for %s, got %s\n' "$expected" "$label" "$actual" >&2
    exit 1
  fi
}

expect_rejected_status() {
  local actual="$1"
  local label="$2"

  case "$actual" in
    401|403|404) ;;
    *)
      printf 'expected RLS rejection for %s, got %s\n' "$label" "$actual" >&2
      exit 1
      ;;
  esac
}

require_cmd curl
require_cmd jq

GDAM_DEV_EMAIL=owner@gdam.local GDAM_DEV_PASSWORD=password123 GDAM_DEV_USERNAME=owner "$ROOT_DIR/scripts/db_seed.sh" >/dev/null
GDAM_DEV_EMAIL=attacker@gdam.local GDAM_DEV_PASSWORD=password123 GDAM_DEV_USERNAME=attacker "$ROOT_DIR/scripts/db_seed.sh" >/dev/null

owner_auth="$(sign_in owner@gdam.local password123)"
attacker_auth="$(sign_in attacker@gdam.local password123)"
OWNER_TOKEN="$(jq -r '.access_token' <<<"$owner_auth")"
OWNER_ID="$(jq -r '.user.id' <<<"$owner_auth")"
ATTACKER_TOKEN="$(jq -r '.access_token' <<<"$attacker_auth")"
ATTACKER_ID="$(jq -r '.user.id' <<<"$attacker_auth")"

if [[ -z "$OWNER_TOKEN" || "$OWNER_TOKEN" == "null" || -z "$ATTACKER_TOKEN" || "$ATTACKER_TOKEN" == "null" ]]; then
  printf 'failed to sign in RLS test users\n' >&2
  exit 1
fi

RUN_ID="${GDAM_RLS_TEST_RUN_ID:-$(date +%s)}"
OWNER_ADDON_NAME="rls-owner-addon-$RUN_ID"
ATTACKER_ADDON_NAME="rls-attacker-addon-$RUN_ID"
STOLEN_ADDON_NAME="rls-stolen-addon-$RUN_ID"
ANON_ADDON_NAME="rls-anon-addon-$RUN_ID"

owner_addon_payload="$(jq -cn \
  --arg user_id "$OWNER_ID" \
  --arg name "$OWNER_ADDON_NAME" \
  '{user_id:$user_id,org_id:null,name:$name,repo:"https://github.com/aviorstudio/gdam-test-addon",editor_plugin:false}')"
owner_addon_response="$(rest_status POST "$OWNER_TOKEN" addons "$owner_addon_payload")"
owner_addon_status="$(sed -n '1p' <<<"$owner_addon_response")"
owner_addon_body="$(sed -n '2,$p' <<<"$owner_addon_response")"
expect_status 201 "$owner_addon_status" 'owner addon insert'
OWNER_ADDON_ID="$(jq -r '.[0].id' <<<"$owner_addon_body")"

attacker_own_payload="$(jq -cn \
  --arg user_id "$ATTACKER_ID" \
  --arg name "$ATTACKER_ADDON_NAME" \
  '{user_id:$user_id,org_id:null,name:$name,repo:"https://github.com/aviorstudio/gdam-test-addon",editor_plugin:false}')"
attacker_own_response="$(rest_status POST "$ATTACKER_TOKEN" addons "$attacker_own_payload")"
expect_status 201 "$(sed -n '1p' <<<"$attacker_own_response")" 'attacker own addon insert'

attacker_owner_payload="$(jq -cn \
  --arg user_id "$OWNER_ID" \
  --arg name "$STOLEN_ADDON_NAME" \
  '{user_id:$user_id,org_id:null,name:$name,repo:"https://github.com/aviorstudio/gdam-test-addon",editor_plugin:false}')"
attacker_owner_response="$(rest_status POST "$ATTACKER_TOKEN" addons "$attacker_owner_payload")"
expect_rejected_status "$(sed -n '1p' <<<"$attacker_owner_response")" 'attacker addon insert for owner'

attacker_version_payload="$(jq -cn \
  --arg addon_id "$OWNER_ADDON_ID" \
  '{addon_id:$addon_id,major:9,minor:9,patch:9,tag:"v9.9.9",asset:"addon.zip"}')"
attacker_version_response="$(rest_status POST "$ATTACKER_TOKEN" addon_versions "$attacker_version_payload")"
expect_rejected_status "$(sed -n '1p' <<<"$attacker_version_response")" 'attacker version insert for owner addon'

anon_payload="$(jq -cn \
  --arg user_id "$OWNER_ID" \
  --arg name "$ANON_ADDON_NAME" \
  '{user_id:$user_id,org_id:null,name:$name,repo:"https://github.com/aviorstudio/gdam-test-addon",editor_plugin:false}')"
anon_response="$(rest_status POST "$SUPABASE_PUBLISHABLE_KEY" addons "$anon_payload")"
expect_rejected_status "$(sed -n '1p' <<<"$anon_response")" 'anonymous addon insert'

public_response="$(rest_status GET "$SUPABASE_PUBLISHABLE_KEY" "addons?select=id,name&name=eq.$OWNER_ADDON_NAME")"
expect_status 200 "$(sed -n '1p' <<<"$public_response")" 'public addon select'
jq -e --arg name "$OWNER_ADDON_NAME" 'length == 1 and .[0].name == $name' >/dev/null <<<"$(sed -n '2,$p' <<<"$public_response")"

printf 'Supabase RLS integration passed\n'
