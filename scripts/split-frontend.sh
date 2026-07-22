#!/usr/bin/env bash
# Split the `frontend/` subdirectory of the PoC-GoLive monorepo into a new
# standalone `golive-frontend` repository under the GoLiveLabs GitHub org.
# History is rewritten with `git filter-repo --subdirectory-filter frontend`
# against a FRESH clone (never the persistent local PoC-GoLive checkout).
#
# Real run (default): clones PoC-GoLive from GitHub into a fresh mktemp dir,
# creates the destination repo via `gh repo create`, and pushes the rewritten
# history to main.
#
# Fixture/test overrides (env): see scripts/split-backend.sh for the same set
# (SOURCE_REPO_URL, SOURCE_DIR, NO_PUSH, KEEP_DIR, DEST_REPO, SUBDIR) with
# defaults adjusted to the frontend repo: DEST_REPO=GoLiveLabs/golive-frontend,
# SUBDIR=frontend.
set -euo pipefail

SUBDIR="${SUBDIR:-frontend}"
DEST_REPO="${DEST_REPO:-GoLiveLabs/golive-frontend}"
SOURCE_REPO_URL="${SOURCE_REPO_URL:-https://github.com/GoLiveLabs/PoC-GoLive.git}"
SOURCE_DIR="${SOURCE_DIR:-}"
NO_PUSH="${NO_PUSH:-0}"
KEEP_DIR="${KEEP_DIR:-0}"

WORK_DIR=""
OWN_WORK_DIR=0

cleanup() {
  if [ "$OWN_WORK_DIR" = "1" ] && [ -n "$WORK_DIR" ] && [ -d "$WORK_DIR" ] && [ "$KEEP_DIR" != "1" ]; then
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT

if [ -n "$SOURCE_DIR" ]; then
  if [ ! -d "$SOURCE_DIR/.git" ]; then
    echo "::error::SOURCE_DIR ($SOURCE_DIR) is not a git repository" >&2
    exit 1
  fi
  WORK_DIR="$(cd "$SOURCE_DIR" && pwd)"
  OWN_WORK_DIR=0
else
  WORK_DIR="$(mktemp -d)"
  OWN_WORK_DIR=1
  echo "Cloning $SOURCE_REPO_URL into $WORK_DIR ..." >&2
  git clone --quiet "$SOURCE_REPO_URL" "$WORK_DIR"
fi

if [ "$KEEP_DIR" = "1" ]; then
  echo "WORK_DIR=$WORK_DIR"
fi

cd "$WORK_DIR"

# Mirror the backend script's dirty-tree guard (US-001.EC-2 analog).
if [ -n "$(git status --porcelain)" ]; then
  echo "::error::Working tree is dirty; commit or stash changes before splitting." >&2
  exit 1
fi

echo "Running git filter-repo --subdirectory-filter $SUBDIR ..." >&2
git filter-repo --subdirectory-filter "$SUBDIR"

if [ "$NO_PUSH" = "1" ]; then
  echo "NO_PUSH=1: skipping gh repo create and git push." >&2
  exit 0
fi

echo "Creating destination repo $DEST_REPO (private) ..." >&2
gh repo create "$DEST_REPO" --private --source=. --remote=split-origin

echo "Pushing rewritten history to main ..." >&2
git push split-origin HEAD:main --tags

echo "Done: $DEST_REPO created and populated." >&2