#!/usr/bin/env bash
# Split the `backend/` subdirectory of the PoC-GoLive monorepo into a new
# standalone `golive-backend` repository under the GoLiveLabs GitHub org.
# History is rewritten with `git filter-repo --subdirectory-filter backend`
# against a FRESH clone (never the persistent local PoC-GoLive checkout).
#
# Real run (default): clones PoC-GoLive from GitHub into a fresh mktemp dir,
# creates the destination repo via `gh repo create`, and pushes the rewritten
# history to main.
#
# Fixture/test overrides (env):
#   SOURCE_REPO_URL  override clone source (use a file:// URI for the local
#                    fixture repository). Default: the GitHub remote.
#   SOURCE_DIR       use an existing clone directly instead of cloning. Used
#                    by IT-002 to feed the dirty-working-tree guard. With this
#                    set, SOURCE_REPO_URL is ignored.
#   NO_PUSH=1        skip `gh repo create` and `git push` (for fixture runs
#                    against a local fixture that has no GitHub destination).
#   KEEP_DIR=1       when set, do not delete a script-created mktemp work dir
#                    on exit, and print `WORK_DIR=<path>` to stdout once the
#                    work dir is established, so a test harness can inspect it.
#   DEST_REPO        override the destination repository name
#                    (default: GoLiveLabs/golive-backend).
#   SUBDIR           override the subdirectory to extract
#                    (default: backend).
#
# Refuses to proceed when the work tree is dirty (US-001.EC-2) before running
# `git filter-repo`. Re-runnable from a fresh clone (US-001.EC-3): the script
# never mutates the persistent PoC-GoLive checkout, only its own fresh mktemp
# clone.
set -euo pipefail

SUBDIR="${SUBDIR:-backend}"
DEST_REPO="${DEST_REPO:-GoLiveLabs/golive-backend}"
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

# US-001.EC-2: refuse to run against a dirty working tree.
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