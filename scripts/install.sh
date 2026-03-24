#!/bin/bash
# Install verikt — https://verikt.dev
set -euo pipefail

VERSION="${VERIKT_VERSION:-latest}"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Validate install dir is absolute path.
INSTALL_DIR="${VERIKT_INSTALL_DIR:-/usr/local/bin}"
if [[ "$INSTALL_DIR" != /* ]]; then
  echo "Error: VERIKT_INSTALL_DIR must be an absolute path, got: $INSTALL_DIR" >&2
  exit 1
fi

# Map architecture names
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac

# Download from GitHub releases
RELEASE_URL="https://github.com/diktahq/verikt/releases"
if [ "$VERSION" = "latest" ]; then
  DOWNLOAD_URL="$RELEASE_URL/latest/download/verikt_${OS}_${ARCH}.tar.gz"
  CHECKSUM_URL="$RELEASE_URL/latest/download/checksums.txt"
else
  DOWNLOAD_URL="$RELEASE_URL/download/v${VERSION}/verikt_${OS}_${ARCH}.tar.gz"
  CHECKSUM_URL="$RELEASE_URL/download/v${VERSION}/checksums.txt"
fi

echo "Installing verikt..."
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

ARCHIVE="verikt_${OS}_${ARCH}.tar.gz"
curl -sSL "$DOWNLOAD_URL" -o "$TMP/$ARCHIVE"
curl -sSL "$CHECKSUM_URL" -o "$TMP/checksums.txt"

# Verify checksum before extracting.
EXPECTED=$(grep "$ARCHIVE" "$TMP/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
  echo "Warning: checksum not found for $ARCHIVE — skipping verification" >&2
else
  if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL=$(sha256sum "$TMP/$ARCHIVE" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    ACTUAL=$(shasum -a 256 "$TMP/$ARCHIVE" | awk '{print $1}')
  else
    echo "Warning: no sha256sum or shasum found — skipping verification" >&2
    ACTUAL="$EXPECTED"
  fi

  if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "Error: checksum mismatch!" >&2
    echo "  Expected: $EXPECTED" >&2
    echo "  Actual:   $ACTUAL" >&2
    echo "The download may be corrupted or tampered with. Aborting." >&2
    exit 1
  fi
  echo "Checksum verified."
fi

tar xzf "$TMP/$ARCHIVE" -C "$TMP"

# Install binary
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP/verikt" "$INSTALL_DIR/verikt"
else
  echo "Installing to $INSTALL_DIR requires sudo."
  sudo mv "$TMP/verikt" "$INSTALL_DIR/verikt"
fi

echo "verikt installed to $INSTALL_DIR/verikt"

# Auto-setup: register with AI agents
verikt setup

echo ""
echo "Done! Run 'verikt new my-service' to get started."
echo "Docs: https://verikt.dev"
