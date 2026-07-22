#!/usr/bin/env bash
# scripts/check-compose-pins.sh
#
# US-006 AC-1 enforcement for the infra/deploy repo (task_04 subtask 4.4).
# Fails and names the offending service if the `backend` or `frontend`
# service in docker-compose.yml references an image ending in `:latest` or
# carrying no explicit version tag. Exits 0 when both services are pinned
# to an explicit tag (any tag that is not `:latest`).
#
# Scope is limited to the `backend` and `frontend` services on purpose:
# `mediamtx` is a third-party image deliberately floated on `:latest`, and
# `nginx` is a base image reference, not a versioned GoLive component. The
# compatibility record this script guards is the GoLive pair only (ADR-002).
#
# Usage: bash scripts/check-compose-pins.sh [path/to/docker-compose.yml]
# Exit:  0 = both backend and frontend pinned with explicit non-latest tags
#        1 = at least one service is `:latest` or untagged (offender named)
#        2 = docker-compose.yml not found or unreadable

set -uo pipefail

COMPOSE_FILE="${1:-./docker-compose.yml}"

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "::error::docker-compose.yml not found at $COMPOSE_FILE"
  exit 2
fi

FAIL=0
OFFENDERS=()

# Extract the `image:` value scoped to a single top-level service block,
# then enforce the no-`:latest` / must-have-tag rule.
check_service() {
  local service="$1"
  local image_line image_ref name_part

  image_line="$(awk -v svc="$service" '
    $0 ~ "^  " svc ":" { in_svc=1; next }
    /^  [A-Za-z][A-Za-z0-9_-]*:[[:space:]]*$/ && in_svc { in_svc=0 }
    in_svc && /^    image:/ { print; exit }
  ' "$COMPOSE_FILE")"

  if [ -z "$image_line" ]; then
    echo "::error::service '"$service"' has no image: reference in $COMPOSE_FILE"
    FAIL=1
    OFFENDERS+=("$service")
    return
  fi

  # Strip the "    image:" key, surrounding quotes, and trailing whitespace.
  image_ref="${image_line##*: }"
  image_ref="${image_ref#\"}"
  image_ref="${image_ref%\"}"
  image_ref="${image_ref#\'}"
  image_ref="${image_ref%\'}"
  image_ref="${image_ref%"${image_ref##*[![:space:]]}"}"

  if [[ "$image_ref" =~ :latest$ ]]; then
    echo "::error::service '"$service"' image '$image_ref' uses :latest; pin an explicit vX.Y.Z tag"
    FAIL=1
    OFFENDERS+=("$service")
    return
  fi

  # A tag exists iff the image-name segment after the last '/' contains ':'.
  name_part="${image_ref##*/}"
  if [[ "$name_part" != *:* ]]; then
    echo "::error::service '"$service"' image '$image_ref' has no explicit version tag"
    FAIL=1
    OFFENDERS+=("$service")
    return
  fi

  return
}

check_service backend
check_service frontend

if [ "$FAIL" -ne 0 ]; then
  echo "check-compose-pins: FAIL - offending services: ${OFFENDERS[*]}"
  exit 1
fi

echo "check-compose-pins: PASS - backend and frontend pinned with explicit version tags"
exit 0