#!/usr/bin/env bash
# Unit-style checks for the tag-validation regex (UT-001..UT-005) and the
# frontend version-consistency check (UT-006..UT-008) from
# .compozy/tasks/separar-projetos/_tests.md.
#
# Sources scripts/lib/release_checks.sh and exercises validate_tag() and
# check_frontend_version() with the exact inputs the contract names. Each
# case reports pass/fail and the script exits non-zero if any case failed.
#
# Run: bash scripts/tests/test_release_checks.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=../lib/release_checks.sh
. "$HERE/../lib/release_checks.sh"

PASS=0
FAIL=0
FAILURES=()

run_case() {
  local id="$1"
  local expected_result="$2"   # pass | fail
  local fn="$3"
  shift 3
  local actual
  local out
  # capture stdout for the !error:: annotation if any; suppress propagation of
  # the function's `exit 1` (validate_tag/check_frontend_version use return, not
  # exit, so this is belt-and-braces).
  out="$("$fn" "$@" 2>&1)" && actual=pass || actual=fail
  if [ "$actual" = "$expected_result" ]; then
    PASS=$((PASS + 1))
    echo "[PASS] $id"
  else
    FAIL=$((FAIL + 1))
    FAILURES+=("$id (expected=$expected_result actual=$actual out=${out})")
    echo "[FAIL] $id (expected=$expected_result actual=$actual out=${out})"
  fi
}

# --- UT-001..UT-005: tag-validation regex ------------------------------

export GITHUB_REF_NAME="v1.2.3"; run_case UT-001 pass validate_tag
export GITHUB_REF_NAME="v1.2";   run_case UT-002 fail validate_tag
export GITHUB_REF_NAME="1.2.3";  run_case UT-003 fail validate_tag
export GITHUB_REF_NAME="v1.2.3-rc1"; run_case UT-004 fail validate_tag
export GITHUB_REF_NAME="v0.0.0"; run_case UT-005 pass validate_tag

# --- UT-006..UT-008: frontend version-consistency check ----------------

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cat > "$TMP/match.json" <<'JSON'
{ "name": "frontend", "version": "0.1.0" }
JSON
cat > "$TMP/mismatch.json" <<'JSON'
{ "name": "frontend", "version": "0.1.0" }
JSON
cat > "$TMP/empty.json" <<'JSON'
{ "name": "frontend" }
JSON
cat > "$TMP/blank.json" <<'JSON'
{ "name": "frontend", "version": "" }
JSON

# UT-006 (happy): package.json 0.1.0 vs tag v0.1.0 -> pass
export GITHUB_REF_NAME="v0.1.0"; run_case UT-006 pass check_frontend_version "$TMP/match.json"
# UT-007 (mismatch): package.json 0.1.0 vs tag v0.2.0 -> fail
export GITHUB_REF_NAME="v0.2.0"; run_case UT-007 fail check_frontend_version "$TMP/mismatch.json"
# UT-008 (missing): package.json with no version field -> fail
export GITHUB_REF_NAME="v0.1.0"; run_case UT-008-missing fail check_frontend_version "$TMP/empty.json"
# UT-008 (blank): package.json with empty version string -> fail
export GITHUB_REF_NAME="v0.1.0"; run_case UT-008-blank fail check_frontend_version "$TMP/blank.json"

echo ""
echo "release_checks unit summary: PASS=$PASS FAIL=$FAIL"
if [ "$FAIL" -ne 0 ]; then
  printf '  - %s\n' "${FAILURES[@]}"
  exit 1
fi
exit 0