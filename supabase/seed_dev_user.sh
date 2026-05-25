#!/usr/bin/env bash
set -euo pipefail

SUPABASE_URL="${SUPABASE_URL:-http://127.0.0.1:54421}"
SUPABASE_PUBLISHABLE_KEY="${SUPABASE_PUBLISHABLE_KEY:-sb_publishable_ACJWlzQHlZjBrEguHvfOxg_3BJgxAaH}"

DEV_EMAIL="${GDAM_DEV_EMAIL:-dev@gdam.local}"
DEV_PASSWORD="${GDAM_DEV_PASSWORD:-password123}"
DEV_USERNAME="${GDAM_DEV_USERNAME:-dev}"
DEV_NAME="${GDAM_DEV_NAME:-GDAM Dev}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

json_escape() {
  jq -Rn --arg value "$1" '$value'
}

auth_request() {
  local endpoint="$1"
  curl -sS \
    -H "apikey: $SUPABASE_PUBLISHABLE_KEY" \
    -H "Content-Type: application/json" \
    -d "{\"email\":$(json_escape "$DEV_EMAIL"),\"password\":$(json_escape "$DEV_PASSWORD")}" \
    "$SUPABASE_URL/auth/v1/$endpoint"
}

upsert_row() {
  local table="$1"
  local conflict_target="$2"
  local payload="$3"

  curl -sS -f \
    -H "apikey: $SUPABASE_PUBLISHABLE_KEY" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "Content-Type: application/json" \
    -H "Prefer: resolution=merge-duplicates" \
    -d "$payload" \
    "$SUPABASE_URL/rest/v1/$table?on_conflict=$conflict_target" \
    >/dev/null
}

require_cmd curl
require_cmd jq

signup_response="$(auth_request signup)"
if jq -e '.error_code == "user_already_exists" or (.msg // "" | test("already registered"; "i"))' >/dev/null <<<"$signup_response"; then
  auth_response="$(auth_request token?grant_type=password)"
else
  auth_response="$signup_response"
fi

USER_ID="$(jq -r '.user.id // empty' <<<"$auth_response")"
ACCESS_TOKEN="$(jq -r '.access_token // empty' <<<"$auth_response")"

if [[ -z "$USER_ID" || -z "$ACCESS_TOKEN" ]]; then
  printf 'failed to create or sign in dev user:\n%s\n' "$auth_response" >&2
  exit 1
fi

profile_payload="$(jq -cn \
  --arg id "$USER_ID" \
  --arg name "$DEV_NAME" \
  '{id:$id,name:$name}')"

username_payload="$(jq -cn \
  --arg display "$DEV_USERNAME" \
  --arg normal "$(tr '[:upper:]' '[:lower:]' <<<"$DEV_USERNAME")" \
  --arg user_id "$USER_ID" \
  '{username_display:$display,username_normal:$normal,user_id:$user_id,org_id:null}')"

upsert_row profiles id "$profile_payload"
upsert_row usernames username_normal "$username_payload"

printf 'Seeded local dev user:\n'
printf '  email: %s\n' "$DEV_EMAIL"
printf '  password: %s\n' "$DEV_PASSWORD"
printf '  username: @%s\n' "$DEV_USERNAME"
