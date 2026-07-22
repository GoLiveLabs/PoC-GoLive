#!/usr/bin/env bash
# Fixture-based verification for scripts/split-backend.sh and
# scripts/split-frontend.sh, covering IT-001..IT-004 of the test contract at
# .compozy/tasks/separar-projetos/_tests.md.
#
# Builds a disposable local fixture repository seeded with the commit shapes
# the test contract specifies (commits touching only backend/, only frontend/,
# and both in the same commit), then runs the split scripts against it with
# env overrides that skip `gh repo create` and `git push`, and asserts on the
# resulting filtered history and working tree.
#
# Run: bash scripts/tests/test_split_fixture.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
SPLIT_BACKEND="$REPO_ROOT/scripts/split-backend.sh"
SPLIT_FRONTEND="$REPO_ROOT/scripts/split-frontend.sh"

PASS=0
FAIL=0
FAILURES=()
note_pass() { PASS=$((PASS + 1)); echo "[PASS] $1"; }
note_fail() { FAIL=$((FAIL + 1)); FAILURES+=("$1: $2"); echo "[FAIL] $1: $2"; }

# ---------------------------------------------------------------------------
# Fixture: small repo with backend/ and frontend/ subdirs, three commit shapes.
# ---------------------------------------------------------------------------

build_fixture() {
  local fx="$1"
  git init -q "$fx"
  cd "$fx"
  git config user.name "Fixture"
  git config user.email "fixture@example.com"
  git config commit.gpgsign false

  # Commit 1 (C1): backend skeleton + .gitignore (backend-relevant + infra)
  mkdir -p backend frontend
  cat > .gitignore <<'EOF'
frontend/node_modules/
frontend/.angular/
EOF
  cat > backend/go.mod <<'EOF'
module golive-backend-fixture

go 1.26.4
EOF
  cat > backend/main.go <<'EOF'
package main

import "fmt"

func main() { fmt.Println("hi") }
EOF
  cat > backend/main_test.go <<'EOF'
package main

import "testing"

func TestMath(t *testing.T) {
  if 1+1 != 2 {
    t.Fatal("math broken")
  }
}
EOF
  git add .gitignore backend
  GIT_AUTHOR_DATE="2026-01-02T03:04:05" GIT_COMMITTER_DATE="2026-01-02T03:04:05" \
    git commit -q -m "Add backend skeleton and gitignore"

  # Commit 2 (C2): frontend skeleton only (must be EXCLUDED by backend split)
  cat > frontend/package.json <<'EOF'
{ "name": "frontend-fixture", "version": "0.0.0", "private": true }
EOF
  cat > frontend/README.md <<'EOF'
# frontend fixture
EOF
  git add frontend
  GIT_AUTHOR_DATE="2026-01-02T04:04:05" GIT_COMMITTER_DATE="2026-01-02T04:04:05" \
    git commit -q -m "Add frontend skeleton"

  # Commit 3 (C3): mixed commit (touches backend/ AND frontend/)
  cat > backend/handler.go <<'EOF'
package main

func handle() {}
EOF
  mkdir -p frontend/src
  cat > frontend/src/index.ts <<'EOF'
console.log("index");
EOF
  git add backend/handler.go frontend/src/index.ts
  GIT_AUTHOR_DATE="2026-01-02T05:04:05" GIT_COMMITTER_DATE="2026-01-02T05:04:05" \
    git commit -q -m "Touch both backend and frontend"

  # Commit 4 (C4): backend only
  cat > backend/util.go <<'EOF'
package main

func util() {}
EOF
  git add backend/util.go
  GIT_AUTHOR_DATE="2026-01-02T06:04:05" GIT_COMMITTER_DATE="2026-01-02T06:04:05" \
    git commit -q -m "Backend util"

  # Commit 5 (C5): frontend only (must be EXCLUDED by backend split)
  cat > frontend/src/app.ts <<'EOF'
console.log("app");
EOF
  git add frontend/src/app.ts
  GIT_AUTHOR_DATE="2026-01-02T07:04:05" GIT_COMMITTER_DATE="2026-01-02T07:04:05" \
    git commit -q -m "Frontend app"

  # Pre-create the gitignored dirs as untracked, to confirm they stay
  # untracked and absent from history after filter-repo.
  mkdir -p frontend/node_modules/.placeholder frontend/.angular/cache
  echo "stub" > frontend/node_modules/.placeholder/dummy
  echo "stub" > frontend/.angular/cache/dummy
}

# ---------------------------------------------------------------------------
# IT-001: split-backend.sh preserves all backend-relevant history (including
# the mixed commit, with only its backend changes), and excludes frontend-
# only commits.
# ---------------------------------------------------------------------------

it_001() {
  local fx
  fx="$(mktemp -d)"
  build_fixture "$fx"
  local out work
  out="$(SOURCE_REPO_URL="$fx" NO_PUSH=1 KEEP_DIR=1 bash "$SPLIT_BACKEND" 2>&1)" || {
    note_fail IT-001 "split-backend.sh exited non-zero: $out"
    rm -rf "$fx"
    return
  }
  work="$(echo "$out" | sed -n 's/^WORK_DIR=//p')"
  if [ -z "$work" ] || [ ! -d "$work" ]; then
    note_fail IT-001 "WORK_DIR not emitted or missing: $out"
    rm -rf "$fx" "$work"
    return
  fi
  local subjects
  subjects="$(git -C "$work" log --format='%s')"
  # Expected: C1 (backend skeleton), C3 (mixed), C4 (backend util). NOT C2/C5.
  local missing="" extra=""
  for s in "Add backend skeleton and gitignore" "Touch both backend and frontend" "Backend util"; do
    if ! echo "$subjects" | grep -qx -- "$s"; then missing="$missing|$s"; fi
  done
  for s in "Add frontend skeleton" "Frontend app"; do
    if echo "$subjects" | grep -qx -- "$s"; then extra="$extra|$s"; fi
  done
  if [ -n "$missing" ] || [ -n "$extra" ]; then
    note_fail IT-001 "subject mismatch missing=<$missing> extra=<$extra> log=<$subjects>"
  else
    note_pass IT-001
  fi
  # Confirm the mixed commit includes the backend change and NOT the frontend file
  if ! git -C "$work" ls-tree -r HEAD --name-only | grep -qx "handler.go"; then
    note_fail IT-001 "mixed commit did not carry backend/handler.go forward"
  fi
  if git -C "$work" ls-tree -r HEAD --name-only | grep -q "^src/index.ts\|frontend/src/index.ts"; then
    note_fail IT-001 "frontend file from the mixed commit leaked into backend repo"
  fi
  # Confirm Dockerfile-less backend fixture still has the expected files at root
  if ! git -C "$work" ls-tree -r HEAD --name-only | grep -qx "go.mod"; then
    note_fail IT-001 "backend/go.mod was not promoted to repo root"
  fi
  # Exercise go vet && go test against the filtered repo (fixture has a real,
  # passing Go module so this is a credible IT-001 harness, not a mock).
  if command -v go >/dev/null 2>&1; then
    if ( cd "$work" && go vet ./... && go test ./... >/dev/null 2>&1 ); then
      note_pass IT-001.go-test
    else
      note_fail IT-001.go-test "go vet/test failed in filtered backend repo"
    fi
  else
    echo "[SKIP] IT-001.go-test: go not on PATH"
  fi
  rm -rf "$fx" "$work"
}

# ---------------------------------------------------------------------------
# IT-002: split-backend.sh refuses a clone with uncommitted local changes.
# ---------------------------------------------------------------------------

it_002() {
  local fx
  fx="$(mktemp -d)"
  build_fixture "$fx"
  # Make a fresh clone (so we own it) and dirty it.
  local dirty
  dirty="$(mktemp -d)"
  git clone -q "$fx" "$dirty"
  echo "stray" >> "$dirty/backend/main.go"
  local out rc
  out="$(SOURCE_DIR="$dirty" NO_PUSH=1 bash "$SPLIT_BACKEND" 2>&1)"; rc=$?
  if [ "$rc" = "0" ]; then
    note_fail IT-002 "script proceeded despite dirty working tree: $out"
  elif ! echo "$out" | grep -qi "dirty"; then
    note_fail IT-002 "non-zero but no dirty message: rc=$rc out=$out"
  else
    note_pass IT-002
  fi
  # Implantation check: filter-repo preserves the original main.go content
  # (it should be unchanged because we aborted before rewriting).
  if ! git -C "$dirty" diff --quiet HEAD -- backend/main.go; then
    note_pass IT-002.work-tree-pres
  else
    note_fail IT-002.work-tree-pres "work tree state unexpected (should still be dirty)"
  fi
  rm -rf "$fx" "$dirty"
}

# ---------------------------------------------------------------------------
# IT-003: running split-backend.sh twice, each against its own fresh clone of
# the same source commit, produces identical filtered history.
# ---------------------------------------------------------------------------

it_003() {
  local fx
  fx="$(mktemp -d)"
  build_fixture "$fx"
  local out1 work1 out2 work2
  out1="$(SOURCE_REPO_URL="$fx" NO_PUSH=1 KEEP_DIR=1 bash "$SPLIT_BACKEND" 2>&1)"
  work1="$(echo "$out1" | sed -n 's/^WORK_DIR=//p')"
  out2="$(SOURCE_REPO_URL="$fx" NO_PUSH=1 KEEP_DIR=1 bash "$SPLIT_BACKEND" 2>&1)"
  work2="$(echo "$out2" | sed -n 's/^WORK_DIR=//p')"
  if [ -z "$work1" ] || [ -z "$work2" ]; then
    note_fail IT-003 "missing WORK_DIRs: out1=<$out1> out2=<$out2>"
    rm -rf "$fx"; return
  fi
  local h1 h2
  h1="$(git -C "$work1" log --format='%H %s')"
  h2="$(git -C "$work2" log --format='%H %s')"
  if [ "$h1" = "$h2" ]; then
    note_pass IT-003
  else
    note_fail IT-003 "histories differ:\n$h1\n--\n$h2"
  fi
  rm -rf "$fx" "$work1" "$work2"
}

# ---------------------------------------------------------------------------
# IT-004: split-frontend.sh preserves only frontend-relevant history, with
# gitignored node_modules/.angular absent. The fixture does not configure a
# working Angular unit-test build; `npx ng test --watch=false` execution is
# deferred to the real-split verification step (task_02/task_03).
# ---------------------------------------------------------------------------

it_004() {
  local fx
  fx="$(mktemp -d)"
  build_fixture "$fx"
  local out work
  out="$(SOURCE_REPO_URL="$fx" NO_PUSH=1 KEEP_DIR=1 bash "$SPLIT_FRONTEND" 2>&1)" || {
    note_fail IT-004 "split-frontend.sh exited non-zero: $out"
    rm -rf "$fx"; return
  }
  work="$(echo "$out" | sed -n 's/^WORK_DIR=//p')"
  if [ -z "$work" ] || [ ! -d "$work" ]; then
    note_fail IT-004 "WORK_DIR not emitted or missing: $out"
    rm -rf "$fx" "$work"; return
  fi
  local subjects
  subjects="$(git -C "$work" log --format='%s')"
  # Expected frontend history: C2 (frontend skeleton), C3 (mixed commit),
  # C5 (frontend app). NOT C1 or C4.
  local missing="" extra=""
  for s in "Add frontend skeleton" "Touch both backend and frontend" "Frontend app"; do
    if ! echo "$subjects" | grep -qx -- "$s"; then missing="$missing|$s"; fi
  done
  for s in "Add backend skeleton and gitignore" "Backend util"; do
    if echo "$subjects" | grep -qx -- "$s"; then extra="$extra|$s"; fi
  done
  if [ -n "$missing" ] || [ -n "$extra" ]; then
    note_fail IT-004 "subject mismatch missing=<$missing> extra=<$extra> log=<$subjects>"
  else
    note_pass IT-004
  fi
  # The mixed commit should carry frontend/src/index.ts forward, NOT backend/handler.go
  if ! git -C "$work" ls-tree -r HEAD --name-only | grep -qx "src/index.ts" && \
     ! git -C "$work" ls-tree -r HEAD --name-only | grep -qx "frontend/src/index.ts"; then
    note_fail IT-004 "frontend/src/index.ts was not carried forward by the mixed commit"
  fi
  if git -C "$work" ls-tree -r HEAD --name-only | grep -q "handler.go"; then
    note_fail IT-004 "backend/handler.go leaked into frontend repo from mixed commit"
  fi
  # IT-004 gitignore check: node_modules and .angular must be absent
  local entries
  entries="$(git -C "$work" ls-tree -r HEAD --name-only)"
  if echo "$entries" | grep -q "node_modules"; then
    note_fail IT-004.gitignore "node_modules present in git tree"
  else
    note_pass IT-004.gitignore
  fi
  if echo "$entries" | grep -q "^\.angular\|/\.angular"; then
    note_fail IT-004.gitignore-angular ".angular present in git tree"
  else
    note_pass IT-004.gitignore-angular
  fi
  # Working-tree absence too (git clone wouldn't bring untracked dirs anyway,
  # but assert explicitly per the contract)
  if [ -d "$work/node_modules" ] || [ -d "$work/.angular" ] || \
     [ -d "$work/frontend/node_modules" ] || [ -d "$work/frontend/.angular" ]; then
    note_fail IT-004.work-tree "gitignored dirs present in working tree"
  else
    note_pass IT-004.work-tree
  fi
  rm -rf "$fx" "$work"
}

# ---------------------------------------------------------------------------

echo "== split-script fixture verification (IT-001..IT-004) =="
it_001
it_002
it_003
it_004
echo ""
echo "split-fixture summary: PASS=$PASS FAIL=$FAIL"
if [ "$FAIL" -ne 0 ]; then
  printf '  - %s\n' "${FAILURES[@]}"
  exit 1
fi
exit 0