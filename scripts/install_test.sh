#!/bin/bash
# Tests for scripts/install.sh. Self-contained: builds fake release fixtures
# under a temp dir and points the installer at them via file:// URLs.
# Bash 3.2 compatible (macOS default bash).
#
# Usage: bash scripts/install_test.sh
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
INSTALLER="$SCRIPT_DIR/install.sh"
# Run the installer under the same bash that runs this test, so
# `/bin/bash scripts/install_test.sh` exercises bash 3.2 on macOS.
BASH_BIN="$BASH"

WORK=$(mktemp -d "${TMPDIR:-/tmp}/verikt-install-test.XXXXXX")
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); echo "ok   - $1"; }
fail() { FAIL=$((FAIL + 1)); echo "FAIL - $1"; }

host_os() { uname -s | tr '[:upper:]' '[:lower:]'; }
host_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) uname -m ;;
  esac
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

TAG="v1.2.3"
OS=$(host_os)
ARCH=$(host_arch)
ARCHIVE="verikt_${TAG#v}_${OS}_${ARCH}.tar.gz"
MARKER="$WORK/setup-ran"

# make_release <dir> builds <dir>/download/$TAG/{archive,checksums.txt} and
# <dir>/latest.json. The dummy binary records `setup` invocations in $MARKER.
make_release() {
  local root="$1" stage="$1/stage" rel="$1/download/$TAG"
  mkdir -p "$stage" "$rel"
  cat >"$stage/verikt" <<EOF
#!/bin/sh
if [ "\${1:-}" = "setup" ]; then
  echo ran >"$MARKER"
fi
echo "dummy verikt \$*"
EOF
  chmod 0755 "$stage/verikt"
  tar czf "$rel/$ARCHIVE" -C "$stage" verikt
  # A decoy entry whose name contains the archive name as a substring must
  # not be matched.
  {
    echo "0000000000000000000000000000000000000000000000000000000000000000  ${ARCHIVE}.sbom.json"
    echo "$(sha256 "$rel/$ARCHIVE")  $ARCHIVE"
  } >"$rel/checksums.txt"
  printf '{"url":"x","tag_name":"%s","name":"%s"}\n' "$TAG" "$TAG" >"$root/latest.json"
}

# run_install <fixture> <install_dir> [VAR=value ...] -- [args...]
# Runs the installer with env overrides. Output goes to $WORK/out.log.
run_install() {
  local fixture="$1" dir="$2"
  shift 2
  local envs=()
  while [ $# -gt 0 ] && [ "$1" != "--" ]; do
    envs+=("$1")
    shift
  done
  [ $# -gt 0 ] && shift
  case "$dir" in /*) mkdir -p "$dir" ;; esac
  rm -f "$MARKER"
  env VERIKT_INSTALL_DIR="$dir" \
    VERIKT_RELEASE_URL="file://$fixture" \
    VERIKT_LATEST_URL="file://$fixture/latest.json" \
    ${envs[@]+"${envs[@]}"} \
    "$BASH_BIN" "$INSTALLER" ${@+"$@"} >"$WORK/out.log" 2>&1
}

# --- happy path: latest ---------------------------------------------------
FIX="$WORK/good"
make_release "$FIX"

DIR="$WORK/inst-latest"
if run_install "$FIX" "$DIR" -- && [ -x "$DIR/verikt" ]; then
  pass "latest resolves to $TAG and installs"
else
  fail "latest resolves to $TAG and installs"; cat "$WORK/out.log"
fi
if [ -x "$DIR/verikt" ] && [ ! -e "$MARKER" ]; then
  pass "setup not run without --setup"
else
  fail "setup not run without --setup"
fi
if grep -q "Run 'verikt setup' to register with your AI agents" "$WORK/out.log"; then
  pass "prints setup hint"
else
  fail "prints setup hint"; cat "$WORK/out.log"
fi

# --- happy path: pinned version, with and without leading v ---------------
for v in v1.2.3 1.2.3; do
  DIR="$WORK/inst-pinned-$v"
  if run_install "$FIX" "$DIR" VERIKT_VERSION="$v" -- && [ -x "$DIR/verikt" ]; then
    pass "VERIKT_VERSION=$v installs"
  else
    fail "VERIKT_VERSION=$v installs"; cat "$WORK/out.log"
  fi
done

# --- --setup runs setup ---------------------------------------------------
DIR="$WORK/inst-setup"
if run_install "$FIX" "$DIR" -- --setup && [ -e "$MARKER" ]; then
  pass "--setup runs verikt setup"
else
  fail "--setup runs verikt setup"; cat "$WORK/out.log"
fi

# --- unknown argument -----------------------------------------------------
DIR="$WORK/inst-badarg"
if run_install "$FIX" "$DIR" -- --bogus; then
  fail "unknown argument exits nonzero"
elif grep -q "unknown argument: --bogus" "$WORK/out.log"; then
  pass "unknown argument exits nonzero"
else
  fail "unknown argument rejected for wrong reason"; cat "$WORK/out.log"
fi

# --- missing checksum entry -----------------------------------------------
FIX2="$WORK/nochecksum"
make_release "$FIX2"
grep -v "  $ARCHIVE\$" "$FIX2/download/$TAG/checksums.txt" >"$FIX2/tmp" || true
mv "$FIX2/tmp" "$FIX2/download/$TAG/checksums.txt"
DIR="$WORK/inst-nochecksum"
if run_install "$FIX2" "$DIR" --; then
  fail "missing checksum entry exits nonzero"
elif grep -q "no checksum entry for $ARCHIVE" "$WORK/out.log"; then
  pass "missing checksum entry exits nonzero"
else
  fail "missing checksum entry rejected for wrong reason"; cat "$WORK/out.log"
fi
if [ ! -e "$DIR/verikt" ]; then
  pass "missing checksum entry installs nothing"
else
  fail "missing checksum entry installs nothing"
fi

# --- checksum mismatch ----------------------------------------------------
FIX3="$WORK/mismatch"
make_release "$FIX3"
echo "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff  $ARCHIVE" \
  >"$FIX3/download/$TAG/checksums.txt"
DIR="$WORK/inst-mismatch"
if run_install "$FIX3" "$DIR" --; then
  fail "checksum mismatch exits nonzero"
elif grep -q "checksum mismatch for $ARCHIVE" "$WORK/out.log"; then
  pass "checksum mismatch exits nonzero"
else
  fail "checksum mismatch rejected for wrong reason"; cat "$WORK/out.log"
fi
if [ ! -e "$DIR/verikt" ]; then pass "checksum mismatch installs nothing"; else fail "checksum mismatch installs nothing"; fi

# --- no sha256 tool available ---------------------------------------------
RBIN="$WORK/restricted-bin"
mkdir -p "$RBIN"
for tool in uname tr curl sed head mktemp rm awk mkdir tar gzip chmod mv cat env; do
  p=$(command -v "$tool" 2>/dev/null || true)
  [ -n "$p" ] && ln -s "$p" "$RBIN/$tool"
done
DIR="$WORK/inst-nosha"
if run_install "$FIX" "$DIR" PATH="$RBIN" --; then
  fail "no sha256 tool exits nonzero"; cat "$WORK/out.log"
else
  if grep -q "neither sha256sum nor shasum" "$WORK/out.log"; then
    pass "no sha256 tool exits nonzero"
  else
    fail "no sha256 tool exits nonzero (wrong reason)"; cat "$WORK/out.log"
  fi
fi
if [ ! -e "$DIR/verikt" ]; then pass "no sha256 tool installs nothing"; else fail "no sha256 tool installs nothing"; fi

# --- invalid versions -----------------------------------------------------
for v in '1.0;rm -rf' '../x' 'v1.2' 'v1.2.3/../../x' 'latest2' ' v1.2.3'; do
  DIR="$WORK/inst-invalid"
  rm -rf "$DIR"
  if run_install "$FIX" "$DIR" VERIKT_VERSION="$v" --; then
    fail "invalid VERIKT_VERSION '$v' exits nonzero"
  elif grep -q "invalid VERIKT_VERSION" "$WORK/out.log"; then
    pass "invalid VERIKT_VERSION '$v' exits nonzero"
  else
    fail "invalid VERIKT_VERSION '$v' rejected for wrong reason"; cat "$WORK/out.log"
  fi
done

# --- invalid latest tag ---------------------------------------------------
FIX4="$WORK/badlatest"
make_release "$FIX4"
printf '{"tag_name":"v1.0;rm -rf /"}\n' >"$FIX4/latest.json"
DIR="$WORK/inst-badlatest"
if run_install "$FIX4" "$DIR" --; then
  fail "invalid latest tag exits nonzero"
elif grep -q "latest release tag is not a valid version" "$WORK/out.log"; then
  pass "invalid latest tag exits nonzero"
else
  fail "invalid latest tag rejected for wrong reason"; cat "$WORK/out.log"
fi

# --- missing release asset (curl -f must not save an error body) ----------
FIX5="$WORK/noarchive"
make_release "$FIX5"
rm "$FIX5/download/$TAG/$ARCHIVE"
DIR="$WORK/inst-noarchive"
if run_install "$FIX5" "$DIR" --; then
  fail "missing archive exits nonzero"
elif grep -q "download failed: .*$ARCHIVE" "$WORK/out.log"; then
  pass "missing archive exits nonzero"
else
  fail "missing archive rejected for wrong reason"; cat "$WORK/out.log"
fi
if [ ! -e "$DIR/verikt" ]; then pass "missing archive installs nothing"; else fail "missing archive installs nothing"; fi

# --- relative install dir -------------------------------------------------
if run_install "$FIX" "relative/dir" --; then
  fail "relative VERIKT_INSTALL_DIR exits nonzero"
elif grep -q "must be an absolute path" "$WORK/out.log"; then
  pass "relative VERIKT_INSTALL_DIR exits nonzero"
else
  fail "relative VERIKT_INSTALL_DIR rejected for wrong reason"; cat "$WORK/out.log"
fi

echo ""
echo "$PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
