#!/usr/bin/env sh
set -eu

REPO="${GDAM_REPO:-aviorstudio/gdam}"
VERSION="${VERSION:-${GDAM_VERSION:-latest}}"
INSTALL_DIR="${INSTALL_DIR:-}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

detect_os() {
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    darwin) printf 'Darwin' ;;
    linux) printf 'Linux' ;;
    *)
      printf 'unsupported OS: %s\n' "$os" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) printf 'x86_64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *)
      printf 'unsupported architecture: %s\n' "$arch" >&2
      exit 1
      ;;
  esac
}

# A token for the GitHub API, if the caller has one. Empty is fine and is the
# normal case for a person installing by hand.
#
# GDAM_GITHUB_TOKEN first so a caller can point this at a different token than
# whatever GITHUB_TOKEN happens to hold; then the two names CI and the gh CLI
# already set, so most callers need do nothing.
api_token() {
  printf '%s' "${GDAM_GITHUB_TOKEN:-${GITHUB_TOKEN:-${GH_TOKEN:-}}}"
}

# The tag of the newest release, or nothing.
#
# AUTHENTICATED when a token is available, because the unauthenticated GitHub
# API allows 60 requests an hour PER IP -- and CI runners share addresses.
# GitHub-hosted macOS runners share them heavily enough that this call returns
# 403 several times an hour, which made "latest" installs fail intermittently
# for every repository using the action. A token scopes the limit to the token
# rather than to the address: 1000/hour per repository for a workflow's
# GITHUB_TOKEN, 5000/hour for a personal access token. The denominator is what
# matters more than the number -- one repository's runs stop competing with
# every other repository sharing that runner.
#
# The token reaches curl through a config on STDIN rather than as -H on the
# command line: arguments are visible in the process list to every other user
# on the machine, and this script runs on shared boxes as well as in CI.
#
# --retry covers the transient half of the same problem: 429 and 5xx are
# retried, and a 403 from the rate limiter is not, so a genuinely exhausted
# quota still fails fast rather than sleeping through three attempts.
latest_tag() {
  _url="https://api.github.com/repos/$REPO/releases/latest"
  _token="$(api_token)"
  if [ -n "$_token" ]; then
    printf 'header = "Authorization: Bearer %s"\n' "$_token" \
      | curl -fsSL --retry 3 --retry-delay 2 -K - "$_url"
  else
    curl -fsSL --retry 3 --retry-delay 2 "$_url"
  fi
}

pick_install_dir() {
  if [ -n "$INSTALL_DIR" ]; then
    printf '%s' "$INSTALL_DIR"
    return
  fi

  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    printf '/usr/local/bin'
    return
  fi

  printf '%s/.local/bin' "$HOME"
}

verify_checksum() {
  artifact="$1"
  checksums="$2"
  artifact_name="$(basename "$artifact")"
  checksum_line="$(grep "  $artifact_name$" "$checksums" || true)"
  expected="${checksum_line%% *}"

  if command -v sha256sum >/dev/null 2>&1; then
    actual_line="$(sha256sum "$artifact")"
    actual="${actual_line%% *}"
  elif command -v shasum >/dev/null 2>&1; then
    actual_line="$(shasum -a 256 "$artifact")"
    actual="${actual_line%% *}"
  else
    printf 'missing checksum command: install sha256sum or shasum\n' >&2
    exit 1
  fi

  if [ -z "$expected" ]; then
    printf 'checksum not found for %s\n' "$artifact_name" >&2
    exit 1
  fi
  if [ "$expected" != "$actual" ]; then
    printf 'checksum mismatch for %s\n' "$artifact_name" >&2
    exit 1
  fi
}

need_cmd curl
need_cmd basename
need_cmd chmod
need_cmd grep
need_cmd mktemp
need_cmd mkdir
need_cmd mv
need_cmd sed
need_cmd tar
need_cmd tr
need_cmd uname

OS="$(detect_os)"
ARCH="$(detect_arch)"

if [ "$VERSION" = "latest" ]; then
  # `|| true` because the failure is reported below with a cause attached. A
  # bare pipeline would swallow curl's status anyway -- sed exits 0 on empty
  # input -- so this makes that explicit rather than accidental.
  API_RESPONSE="$(latest_tag || true)"
  TAG="$(printf '%s' "$API_RESPONSE" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')"
  if [ -z "$TAG" ]; then
    printf 'failed to resolve latest release for %s\n' "$REPO" >&2
    # Name the likely cause. Without this the message says only that the
    # release could not be resolved, which reads as "there is no release" --
    # and sends you looking at the wrong repository. The rate limit was the
    # actual cause every time this fired in CI.
    if [ -z "$(api_token)" ]; then
      printf '\n' >&2
      printf 'The GitHub API allows 60 unauthenticated requests an hour per IP address,\n' >&2
      printf 'and CI runners share addresses. If this is CI, set GITHUB_TOKEN (or\n' >&2
      printf 'GH_TOKEN) in the environment to scope the limit to the token instead\n' >&2
      printf '(1000/hour per repository), or install a pinned VERSION rather than\n' >&2
      printf '"latest".\n' >&2
    fi
    exit 1
  fi
else
  case "$VERSION" in
    v*) TAG="$VERSION" ;;
    *) TAG="v$VERSION" ;;
  esac
fi

ARTIFACT="gdam_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/$REPO/releases/download/$TAG"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

curl -fsSL "$BASE_URL/$ARTIFACT" -o "$TMP_DIR/$ARTIFACT"
curl -fsSL "$BASE_URL/checksums.txt" -o "$TMP_DIR/checksums.txt"
verify_checksum "$TMP_DIR/$ARTIFACT" "$TMP_DIR/checksums.txt"

tar -xzf "$TMP_DIR/$ARTIFACT" -C "$TMP_DIR" gdam

DEST_DIR="$(pick_install_dir)"
mkdir -p "$DEST_DIR"
if [ ! -w "$DEST_DIR" ]; then
  printf 'install directory is not writable: %s\n' "$DEST_DIR" >&2
  printf 'rerun with INSTALL_DIR=$HOME/.local/bin or use sudo with INSTALL_DIR=/usr/local/bin\n' >&2
  exit 1
fi

mv "$TMP_DIR/gdam" "$DEST_DIR/gdam"
chmod +x "$DEST_DIR/gdam"

printf 'Installed gdam %s to %s/gdam\n' "$TAG" "$DEST_DIR"
case ":$PATH:" in
  *":$DEST_DIR:"*) ;;
  *) printf 'Add %s to PATH if gdam is not found.\n' "$DEST_DIR" ;;
esac
