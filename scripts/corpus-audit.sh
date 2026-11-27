#!/bin/bash
# Runs verikt's built-in detectors over real repositories before a release
# (ADR-012) and reports every finding with the line that produced it, grouped by
# detector and by concentration.
#
# Usage:
#   scripts/corpus-audit.sh [-o REPORT] [--keep] [--own-config] REPO [REPO...]
#
# Each REPO is copied to a scratch directory and the copy is analysed: the
# original is never written to (ADR-012 rule 2). The copy's verikt.yaml is
# replaced by a minimal one (flat architecture), so every repository is judged by
# the same detectors and its own architecture rules don't mix in; --own-config
# keeps a repository's verikt.yaml instead.
#
# The report quotes source lines from the audited repositories, so it is written
# outside this checkout (default: a temp file) and must never be committed. An -o
# path inside this repository is refused.
#
# VERIKT selects the binary (default: built from this checkout into the scratch
# directory, which embeds whatever engine internal/engineclient/bin holds; run
# `mise run build-engine` first for a fresh one).
#
# Bash 3.2 compatible (macOS default bash). Needs jq and rsync.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd -P)

die() { echo "corpus-audit: $*" >&2; exit 1; }

usage() {
  awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "${BASH_SOURCE[0]}"
  exit "${1:-0}"
}

REPORT=""
KEEP=0
OWN_CONFIG=0
REPOS=()
while [ $# -gt 0 ]; do
  case "$1" in
    -o) [ $# -ge 2 ] || die "-o needs a path"; REPORT=$2; shift 2 ;;
    --keep) KEEP=1; shift ;;
    --own-config) OWN_CONFIG=1; shift ;;
    -h|--help) usage 0 ;;
    --) shift; while [ $# -gt 0 ]; do REPOS+=("$1"); shift; done ;;
    -*) die "unknown option: $1" ;;
    *) REPOS+=("$1"); shift ;;
  esac
done
[ ${#REPOS[@]} -gt 0 ] || usage 2

command -v jq >/dev/null || die "jq is required"
command -v rsync >/dev/null || die "rsync is required"

WORK=$(mktemp -d "${TMPDIR:-/tmp}/verikt-corpus.XXXXXX")
cleanup() { if [ "$KEEP" -eq 0 ]; then rm -rf -- "$WORK"; fi; }
trap cleanup EXIT

# inside_repo PATH: true when PATH (which may not exist yet) is in this checkout.
inside_repo() {
  local dir
  dir=$(cd "$(dirname "$1")" 2>/dev/null && pwd -P) || return 1
  case "$dir/" in "$REPO_ROOT"/*) return 0 ;; esac
  return 1
}

if [ -z "$REPORT" ]; then
  # Its own directory, so the copies in WORK can still be removed.
  REPORT="$(mktemp -d "${TMPDIR:-/tmp}/verikt-corpus-report.XXXXXX")/report.md"
elif inside_repo "$REPORT"; then
  die "refusing to write the report inside the verikt repository ($REPORT): it quotes source from the audited repositories"
fi

if [ -z "${VERIKT:-}" ]; then
  echo "building verikt from $REPO_ROOT ..." >&2
  (cd "$REPO_ROOT" && go build -o "$WORK/verikt" ./cmd/verikt) || die "go build failed"
  VERIKT="$WORK/verikt"
fi

ROWS="$WORK/findings.tsv"
: > "$ROWS"
SUMMARY="$WORK/summary.tsv"
: > "$SUMMARY"

minimal_config() {
  local lang=go
  if [ ! -f "$1/go.mod" ] && [ -f "$1/package.json" ]; then lang=typescript; fi
  printf 'language: %s\narchitecture: flat\n' "$lang" > "$1/verikt.yaml"
}

audit_repo() {
  local src=$1 n=$2 name dest json files
  [ -d "$src" ] || { echo "skipping $src: not a directory" >&2; return 0; }
  name=$(basename "$(cd "$src" && pwd -P)")
  dest="$WORK/repos/$n-$name"
  mkdir -p "$dest"
  # Nested worktrees (.worktrees/, .claude/worktrees/) are other checkouts of the
  # same code; auditing them again would count every finding twice.
  rsync -a --exclude .git --exclude node_modules --exclude .worktrees \
    --exclude .claude/worktrees "$src/" "$dest/"
  if [ "$OWN_CONFIG" -eq 0 ] || [ ! -f "$dest/verikt.yaml" ]; then minimal_config "$dest"; fi

  json="$WORK/$n-$name.json"
  # A run with findings exits 1; only an unreadable result is a failure.
  "$VERIKT" check --path "$dest" --detectors -o json > "$json" 2> "$WORK/$n-$name.err" || true
  if ! jq -e '.result' "$json" >/dev/null 2>&1; then
    printf '%s\t%s\terror\n' "$name" "-" >> "$SUMMARY"
    echo "verikt failed on $name: $(head -c 300 "$WORK/$n-$name.err")" >&2
    return 0
  fi
  files=$(find "$dest" -type f \( -name '*.go' -o -name '*.ts' -o -name '*.tsx' \) ! -name '*_test.go' | wc -l | tr -d ' ')
  printf '%s\t%s\tok\n' "$name" "$files" >> "$SUMMARY"
  jq -r --arg repo "$name" --arg dest "$dest" '
    [.violations[]?, .anti_patterns[]?][]
    | [$repo, (.rule // .name), .severity,
       # read(1) collapses consecutive tabs, so no field may be empty.
       ((.file // "") | if . == "" then "." else . end),
       ((.line // 0) | tostring),
       ((.message // "-") | gsub("[\t\n]"; " ")), $dest] | @tsv' "$json" >> "$ROWS"
}

i=0
for r in "${REPOS[@]}"; do
  i=$((i + 1))
  echo "[$i/${#REPOS[@]}] $r" >&2
  audit_repo "$r" "$i"
done

# source_line DEST FILE LINE: the trimmed line, cut at 160 bytes.
source_line() {
  [ "$3" -gt 0 ] 2>/dev/null || return 0
  [ -f "$1/$2" ] || return 0
  sed -n "${3}p" "$1/$2" | sed 's/^[[:space:]]*//' | cut -c1-160
}

# concentration RULE COL: "share key" of the largest group of RULE's findings by
# column COL (4 = file, 8 = directory), as a percentage.
concentration() {
  awk -F'\t' -v rule="$1" -v col="$2" '
    $2 == rule {
      key = $1 ":" (col == 8 ? (match($4, /.*\//) ? substr($4, 1, RLENGTH) : "./") : $4)
      n[key]++; total++
    }
    END {
      for (k in n) if (n[k] > best) { best = n[k]; top = k }
      if (total) printf "%d %s\n", int(best * 100 / total), top
    }' "$ROWS"
}

{
  echo "# verikt corpus audit"
  echo
  echo "Generated $(date -u +%Y-%m-%dT%H:%M:%SZ) by $("$VERIKT" --version 2>/dev/null | head -1 || echo verikt). This report quotes source from the audited repositories: keep it out of any repository."
  echo
  echo "## Repositories"
  echo
  echo "| Repository | Source files analysed | Result |"
  echo "|---|---|---|"
  awk -F'\t' '{ printf "| %s | %s | %s |\n", $1, $2, $3 }' "$SUMMARY"
  echo
  total=$(wc -l < "$ROWS" | tr -d ' ')
  echo "## Detectors ($total findings)"
  echo
  echo "| Detector | Findings | Errors | Repositories | Largest file share | Largest directory share |"
  echo "|---|---|---|---|---|---|"
  rules=$(cut -f2 "$ROWS" | sort | uniq -c | sort -rn | awk '{print $2}')
  for rule in $rules; do
    count=$(awk -F'\t' -v r="$rule" '$2 == r' "$ROWS" | wc -l | tr -d ' ')
    errors=$(awk -F'\t' -v r="$rule" '$2 == r && $3 == "error"' "$ROWS" | wc -l | tr -d ' ')
    repos=$(awk -F'\t' -v r="$rule" '$2 == r { print $1 }' "$ROWS" | sort -u | wc -l | tr -d ' ')
    fshare=$(concentration "$rule" 4 | cut -d' ' -f1)
    dshare=$(concentration "$rule" 8 | cut -d' ' -f1)
    echo "| \`$rule\` | $count | $errors | $repos | ${fshare}% | ${dshare}% |"
  done
  echo
  echo "## Suspected false positives"
  echo
  echo "INV-005: findings clustered in one file or directory are presumed wrong until shown otherwise, and an error-severity finding must have earned its precision. Listed: detectors with at least 3 findings where one file holds 50% or more, or one directory 70% or more; and every error-severity finding."
  echo
  suspects=0
  for rule in $rules; do
    count=$(awk -F'\t' -v r="$rule" '$2 == r' "$ROWS" | wc -l | tr -d ' ')
    [ "$count" -ge 3 ] || continue
    read -r fshare ftop <<EOF
$(concentration "$rule" 4)
EOF
    read -r dshare dtop <<EOF
$(concentration "$rule" 8)
EOF
    if [ "$fshare" -ge 50 ]; then
      echo "- \`$rule\`: $fshare% of $count findings in one file, \`$ftop\`"; suspects=$((suspects + 1))
    elif [ "$dshare" -ge 70 ]; then
      echo "- \`$rule\`: $dshare% of $count findings in one directory, \`$dtop\`"; suspects=$((suspects + 1))
    fi
  done
  errs=$(awk -F'\t' '$3 == "error"' "$ROWS" | wc -l | tr -d ' ')
  if [ "$errs" -gt 0 ]; then
    echo "- $errs error-severity finding(s): each is listed below with its line; confirm every one is a real defect."
    suspects=$((suspects + 1))
  fi
  [ "$suspects" -gt 0 ] || echo "None by these heuristics. Still read each detector's findings below."
  echo
  echo "## Findings"
  for rule in $rules; do
    echo
    echo "### \`$rule\`"
    echo
    awk -F'\t' -v r="$rule" '$2 == r' "$ROWS" | sort -t"$(printf '\t')" -k1,1 -k4,4 -k5,5n |
      while IFS="$(printf '\t')" read -r repo _ sev file line msg dest; do
        loc="$repo:$file"
        [ "$line" -gt 0 ] && loc="$loc:$line"
        echo "- **$sev** \`$loc\` — $msg"
        src=$(source_line "$dest" "$file" "$line")
        [ -z "$src" ] || echo "  \`$(printf '%s' "$src" | tr '`' "'")\`"
      done
  done
} > "$REPORT"

echo "report: $REPORT" >&2
if [ "$KEEP" -eq 1 ]; then
  echo "copies kept in: $WORK (they hold the audited source; delete them when done)" >&2
fi
