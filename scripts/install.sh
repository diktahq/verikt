#!/bin/bash
# Install verikt — https://verikt.dev
#
# Usage:
#   curl -fsSL https://verikt.dev/install.sh | bash
#   curl -fsSL https://verikt.dev/install.sh | VERIKT_VERSION=v0.2.0 bash
#   curl -fsSL https://verikt.dev/install.sh | bash -s -- --setup
#
# Environment:
#   VERIKT_VERSION      Release tag to install (e.g. v0.2.0 or 0.2.0). Default: latest.
#   VERIKT_INSTALL_DIR  Absolute install directory. Default: /usr/local/bin.
#   VERIKT_RELEASE_URL  Release base URL. Default: https://github.com/diktahq/verikt/releases
#   VERIKT_LATEST_URL   API URL used to resolve "latest" to a tag.
#                       Default: https://api.github.com/repos/diktahq/verikt/releases/latest
set -euo pipefail

VERSION_RE='^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'

die() {
  echo "Error: $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Install verikt from GitHub releases.

Options:
  --setup     Run 'verikt setup' after installing (registers verikt with your AI agents)
  -h, --help  Show this help

Environment: VERIKT_VERSION, VERIKT_INSTALL_DIR, VERIKT_RELEASE_URL, VERIKT_LATEST_URL
EOF
}

# normalize_version prints the version as a v-prefixed tag, or fails if it is
# not a valid semver tag.
normalize_version() {
  local v="$1"
  case "$v" in
    v*) ;;
    *) v="v$v" ;;
  esac
  if [[ ! "$v" =~ $VERSION_RE ]]; then
    return 1
  fi
  printf '%s\n' "$v"
}

# resolve_latest prints the tag_name of the latest release.
resolve_latest() {
  local url="$1" body tag
  body=$(curl -fsSL "$url") || die "could not query latest release from $url"
  tag=$(printf '%s\n' "$body" \
    | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -n 1)
  [ -n "$tag" ] || die "could not find tag_name in latest release response"
  printf '%s\n' "$tag"
}

detect_os() {
  local os
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    linux|darwin) printf '%s\n' "$os" ;;
    *) die "unsupported operating system: $os (supported: linux, darwin)" ;;
  esac
}

detect_arch() {
  local arch
  arch=$(uname -m)
  case "$arch" in
    x86_64|amd64) printf 'amd64\n' ;;
    aarch64|arm64) printf 'arm64\n' ;;
    *) die "unsupported architecture: $arch (supported: amd64, arm64)" ;;
  esac
}

sha256_of() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    return 1
  fi
}

main() {
  local run_setup=0 arg
  for arg in "$@"; do
    case "$arg" in
      --setup) run_setup=1 ;;
      -h|--help) usage; exit 0 ;;
      *) usage >&2; die "unknown argument: $arg" ;;
    esac
  done

  local install_dir="${VERIKT_INSTALL_DIR:-/usr/local/bin}"
  if [[ "$install_dir" != /* ]]; then
    die "VERIKT_INSTALL_DIR must be an absolute path, got: $install_dir"
  fi

  local release_url="${VERIKT_RELEASE_URL:-https://github.com/diktahq/verikt/releases}"
  local latest_url="${VERIKT_LATEST_URL:-https://api.github.com/repos/diktahq/verikt/releases/latest}"
  local requested="${VERIKT_VERSION:-latest}"
  local os arch tag

  os=$(detect_os)
  arch=$(detect_arch)

  if [ "$requested" = "latest" ]; then
    local latest
    latest=$(resolve_latest "$latest_url")
    tag=$(normalize_version "$latest") || die "latest release tag is not a valid version: $latest"
  else
    tag=$(normalize_version "$requested") || die "invalid VERIKT_VERSION: $requested (expected e.g. v0.2.0)"
  fi

  # Must match .goreleaser.yaml: {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
  local archive="verikt_${tag#v}_${os}_${arch}.tar.gz"
  local base="$release_url/download/$tag"

  echo "Installing verikt $tag ($os/$arch)..."
  local tmp
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/verikt-install.XXXXXX")
  # shellcheck disable=SC2064 # expand $tmp now; it is local to main.
  trap "rm -rf '$tmp'" EXIT

  curl -fsSL "$base/$archive" -o "$tmp/$archive" || die "download failed: $base/$archive"
  curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || die "download failed: $base/checksums.txt"

  # Verify the checksum before extracting. Match the exact filename field.
  local expected actual
  expected=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/checksums.txt")
  if [ -z "$expected" ]; then
    die "no checksum entry for $archive in checksums.txt. Aborting."
  fi
  actual=$(sha256_of "$tmp/$archive") || die "neither sha256sum nor shasum found; cannot verify download. Aborting."
  if [ "$expected" != "$actual" ]; then
    echo "Error: checksum mismatch for $archive" >&2
    echo "  Expected: $expected" >&2
    echo "  Actual:   $actual" >&2
    echo "The download may be corrupted or tampered with. Aborting." >&2
    exit 1
  fi
  echo "Checksum verified."

  mkdir "$tmp/extract"
  tar xzf "$tmp/$archive" -C "$tmp/extract"
  [ -f "$tmp/extract/verikt" ] || die "archive does not contain a verikt binary"
  chmod 0755 "$tmp/extract/verikt"

  if [ -w "$install_dir" ]; then
    mv "$tmp/extract/verikt" "$install_dir/verikt"
  else
    echo "Installing to $install_dir requires sudo."
    sudo mv "$tmp/extract/verikt" "$install_dir/verikt"
  fi
  echo "verikt $tag installed to $install_dir/verikt"

  echo ""
  if [ "$run_setup" -eq 1 ]; then
    "$install_dir/verikt" setup
  else
    echo "Next: Run 'verikt setup' to register with your AI agents."
  fi
  echo "Then run 'verikt new my-service' to get started. Docs: https://verikt.dev"
}

main "$@"
