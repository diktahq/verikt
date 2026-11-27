#!/bin/bash
# Tests for scripts/corpus-audit.sh. Self-contained: builds two synthetic
# repositories under a temp dir and audits them. Needs a verikt binary with an
# engine: VERIKT, or one built from this checkout.
# Bash 3.2 compatible (macOS default bash).
#
# Usage: bash scripts/corpus-audit_test.sh
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
AUDIT="$SCRIPT_DIR/corpus-audit.sh"
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd -P)

WORK=$(mktemp -d "${TMPDIR:-/tmp}/verikt-corpus-test.XXXXXX")
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0
pass() { PASS=$((PASS + 1)); echo "ok   - $1"; }
fail() { FAIL=$((FAIL + 1)); echo "FAIL - $1"; }

if [ -z "${VERIKT:-}" ]; then
  (cd "$REPO_ROOT" && go build -o "$WORK/verikt" ./cmd/verikt)
  VERIKT="$WORK/verikt"
fi
export VERIKT

# Repository "clustered": three naked goroutines in one file, so the detector's
# findings are concentrated and must be listed as suspected false positives.
# (Not in Start/Run/Serve: naked_goroutine exempts lifecycle methods.)
CL="$WORK/src/clustered"
mkdir -p "$CL/internal/worker" "$CL/.git"
printf 'module example.com/clustered\n\ngo 1.24\n' > "$CL/go.mod"
cat > "$CL/internal/worker/worker.go" <<'EOF'
package worker

func Process() {
	go func() {}()
	go func() {}()
	go func() {}()
}
EOF
echo "ref: refs/heads/main" > "$CL/.git/HEAD"

# Repository "clean": nothing to report.
CN="$WORK/src/clean"
mkdir -p "$CN/internal/calc"
printf 'module example.com/clean\n\ngo 1.24\n' > "$CN/go.mod"
printf 'package calc\n\n// Add adds.\nfunc Add(a, b int) int { return a + b }\n' > "$CN/internal/calc/calc.go"

snapshot() { (cd "$1" && find . -type f -print0 | sort -z | xargs -0 shasum) ; }
before_cl=$(snapshot "$CL")
before_cn=$(snapshot "$CN")

REPORT="$WORK/out/report.md"
mkdir -p "$WORK/out"
if TMPDIR="$WORK/tmp" bash -c 'mkdir -p "$TMPDIR"; exec "$0" -o "$1" "$2" "$3"' "$AUDIT" "$REPORT" "$CL" "$CN" 2> "$WORK/stderr"; then
  pass "audit runs"
else
  fail "audit runs: $(cat "$WORK/stderr")"
fi

# expect NAME CMD...: pass when CMD succeeds.
expect() { local name=$1; shift; if "$@"; then pass "$name"; else fail "$name"; fi; }
unchanged() { [ "$(snapshot "$CL")" = "$before_cl" ] && [ "$(snapshot "$CN")" = "$before_cn" ]; }
no_config() { [ ! -f "$CL/verikt.yaml" ]; }
in_report() { grep -qF -- "$1" "$REPORT"; }
copies_gone() { [ "$(find "$WORK/tmp" -maxdepth 1 -name 'verikt-corpus.*' | wc -l | tr -d ' ')" -eq 0 ]; }
refuses_inside() {
  if "$AUDIT" -o "$REPO_ROOT/corpus-report.md" "$CN" 2>/dev/null; then
    rm -f "$REPO_ROOT/corpus-report.md"
    return 1
  fi
  [ ! -f "$REPO_ROOT/corpus-report.md" ]
}

# The report is Markdown: the backticks below are literal, not expansions.
# shellcheck disable=SC2016
{
  expect "audited repositories are not written to" unchanged
  expect "the minimal config goes into the copy only" no_config
  expect "each repository is listed with its file count" in_report '| clustered | 1 | ok |'
  expect "a clean repository is listed too" in_report '| clean | 1 | ok |'
  expect "findings are grouped by detector" in_report '| `naked_goroutine` | 3 |'
  expect "a concentrated detector is listed as a suspected false positive" \
    in_report '`naked_goroutine`: 100% of 3 findings in one file, `clustered:internal/worker/worker.go`'
  expect "each finding is reported with its location" in_report '`clustered:internal/worker/worker.go:4`'
  expect "each finding is reported with its source line" in_report '`go func() {}()`'
  expect "the copies are removed after the run" copies_gone
  expect "a report path inside the verikt repository is refused" refuses_inside
}

echo
echo "$PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
