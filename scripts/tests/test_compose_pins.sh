#!/usr/bin/env bash
# Unit-style checks for the docker-compose pin-lint guard (UT-009, UT-010)
# from .compozy/tasks/separar-projetos/_tests.md.
#
# Drives scripts/check-compose-pins.sh against fixture docker-compose files
# so the test never depends on GHCR or on the real docker-compose.yml. Each
# case reports pass/fail with the exact contract IDs; the script exits
# non-zero if any case failed.
#
# Run: bash scripts/tests/test_compose_pins.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
CHECK="$HERE/../check-compose-pins.sh"

PASS=0
FAIL=0
FAILURES=()

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Write a docker-compose fixture with the given backend and frontend image refs.
write_fixture() {
  local backend_image="$1"
  local frontend_image="$2"
  cat > "$TMP/compose.yml" <<YML
services:
  mediamtx:
    image: bluenviron/mediamtx:latest
    ports:
      - "8554:8554"
  backend:
    image: $backend_image
    environment:
      HTTP_ADDR: ":8080"
  frontend:
    image: $frontend_image
    ports:
      - "4200:4200"
  nginx:
    image: nginx:alpine
YML
}

# run_case <id> <expected pass|fail> <must-match-regex>
# Runs the check against the current $TMP/compose.yml, compares the exit
# status to expected AND (when present) requires the named service to
# appear in the combined output — that is the UT-010 "names the offending
# service" contract.
run_case() {
  local id="$1"
  local expected="$2"        # pass | fail
  local must_match="${3:-}"
  local actual out
  out="$("$CHECK" "$TMP/compose.yml" 2>&1)" && actual=pass || actual=fail

  if [ "$actual" != "$expected" ]; then
    FAIL=$((FAIL + 1))
    FAILURES+=("$id (expected=$expected actual=$actual out=$out)")
    echo "[FAIL] $id (exit: expected=$expected actual=$actual)"
    echo "       out: $out"
    return
  fi

  if [ -n "$must_match" ]; then
    if ! printf '%s' "$out" | grep -qE "$must_match"; then
      FAIL=$((FAIL + 1))
      FAILURES+=("$id (expected output to match /$must_match/ but got: $out)")
      echo "[FAIL] $id (output missing required match /$must_match/)"
      echo "       out: $out"
      return
    fi
  fi

  PASS=$((PASS + 1))
  echo "[PASS] $id"
}

# --- UT-009 (happy): both services pinned to explicit vX.Y.Z tags --------
write_fixture "ghcr.io/golivelabs/golive-backend:v0.1.0" \
              "ghcr.io/golivelabs/golive-frontend:v0.1.0"
run_case UT-009 pass

# --- UT-010 (error): :latest and untagged references named --------------

# UT-010a: backend on :latest -> fail and name "backend"
write_fixture "ghcr.io/golivelabs/golive-backend:latest" \
              "ghcr.io/golivelabs/golive-frontend:v0.1.0"
run_case UT-010a-latest fail "backend"

# UT-010b: frontend untagged -> fail and name "frontend"
write_fixture "ghcr.io/golivelabs/golive-backend:v0.1.0" \
              "ghcr.io/golivelabs/golive-frontend"
run_case UT-010b-untagged fail "frontend"

# UT-010c: both offending -> fail and name both
write_fixture "ghcr.io/golivelabs/golive-backend:latest" \
              "ghcr.io/golivelabs/golive-frontend"
run_case UT-010c-both fail "backend.*frontend|frontend.*backend"

echo ""
echo "compose_pins unit summary: PASS=$PASS FAIL=$FAIL"
if [ "$FAIL" -ne 0 ]; then
  printf '  - %s\n' "${FAILURES[@]}"
  exit 1
fi
exit 0