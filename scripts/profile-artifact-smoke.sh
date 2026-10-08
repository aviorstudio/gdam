#!/usr/bin/env bash
set -euo pipefail
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
VERSION="${VERSION:-dev}"
(cd dist && sha256sum --check checksums.txt)
tar -xzf dist/gdam_Linux_x86_64.tar.gz -C "$tmp"
got="$("$tmp/gdam" version 2>&1)"
echo "$got"

# A requested version must be reported exactly -- that is a release, and
# an archive claiming a different version than its tag is the whole
# failure this guards.
#
# The default is NOT checked against the literal "dev". main.go falls
# back to the module version in its build info when nothing was stamped
# (see the `version == "dev"` branch there), so in a git checkout the
# toolchain's VCS pseudo-version is what comes out -- a pull request
# legitimately reports something like
# `gdam v0.0.8-0.20260802025843-da3ac2fda354`. Asserting "dev" here
# failed the first run of this action, and it was the check that was
# wrong rather than the binary.
if [ "$VERSION" != "dev" ]; then
  first_line="$(printf '%s\n' "$got" | head -n 1)"
  if [ "$first_line" != "gdam $VERSION" ]; then
    echo "::error::archived binary version differs from the exact requested version" >&2
    exit 1
  fi
elif [ -z "${got#gdam }" ] || [ "${got#gdam }" = "$got" ]; then
  echo "::error::built binary reports '$got', expected 'gdam <version>'"
  exit 1
fi
