#!/usr/bin/env bash
# Reusable bash implementations of the two release-workflow guards so they
# can be exercised by unit tests without running a full GitHub Actions
# workflow. The inline `run:` blocks in
#   backend/.github/workflows/release.yml
#   frontend/.github/workflows/release.yml
# implement the same contract verbatim; those workflows live in the split
# target repos after `git filter-repo --subdirectory-filter` and cannot source
# this library, so the two must stay in sync.
#
# Functions:
#   validate_tag            - enforces ADR-005's ^v[0-9]+\.[0-9]+\.[0-9]+$
#                             pattern against $GITHUB_REF_NAME.
#   check_frontend_version  - compares package.json's `version` field
#                             (prefixed with `v`) against $GITHUB_REF_NAME;
#                             rejects empty/missing version fields rather than
#                             comparing a bare "v" against the tag.
#
# Both functions emit `::error::...` (GitHub Actions annotation syntax) on
# failure and `exit 1`. They `return 0` on success so callers can compose them.

set -o errexit
set -o nounset
set -o pipefail

TAG_PATTERN='^v[0-9]+\.[0-9]+\.[0-9]+$'

validate_tag() {
  local tag="${GITHUB_REF_NAME:-}"
  if [ -z "$tag" ]; then
    echo "::error::GITHUB_REF_NAME is empty; tag-validation step requires a vMAJOR.MINOR.PATCH tag"
    return 1
  fi
  if ! [[ "$tag" =~ $TAG_PATTERN ]]; then
    echo "::error::Tag '$tag' does not match required vMAJOR.MINOR.PATCH format"
    return 1
  fi
  return 0
}

check_frontend_version() {
  local pkg_path="${1:-./package.json}"
  local tag="${GITHUB_REF_NAME:-}"
  if [ -z "$tag" ]; then
    echo "::error::GITHUB_REF_NAME is empty; version-consistency step requires a vMAJOR.MINOR.PATCH tag"
    return 1
  fi
  if [ ! -f "$pkg_path" ]; then
    echo "::error::package.json not found at $pkg_path"
    return 1
  fi
  local raw_version
  raw_version="$(node -p "require(process.argv[1]).version" "$pkg_path" 2>/dev/null || true)"
  if [ -z "$raw_version" ] || [ "$raw_version" = "undefined" ] || [ "$raw_version" = "null" ]; then
    echo "::error::package.json version field is missing or empty"
    return 1
  fi
  local pkg_version="v${raw_version}"
  if [ "$pkg_version" != "$tag" ]; then
    echo "::error::package.json version ($pkg_version) does not match tag ($tag)"
    return 1
  fi
  return 0
}