#!/bin/bash
# Install archway — https://archway.dcsg.me
set -euo pipefail

VERSION="${ARCHWAY_VERSION:-latest}"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Map architecture names
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac

# Download from GitHub releases
RELEASE_URL="https://github.com/dcsg/archway/releases"
if [ "$VERSION" = "latest" ]; then
  DOWNLOAD_URL="$RELEASE_URL/latest/download/archway_${OS}_${ARCH}.tar.gz"
else
  DOWNLOAD_URL="$RELEASE_URL/download/v${VERSION}/archway_${OS}_${ARCH}.tar.gz"
fi

INSTALL_DIR="${ARCHWAY_INSTALL_DIR:-/usr/local/bin}"

echo "Installing archway..."
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -sSL "$DOWNLOAD_URL" -o "$TMP/archway.tar.gz"
tar xzf "$TMP/archway.tar.gz" -C "$TMP"

# Install binary
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP/archway" "$INSTALL_DIR/archway"
else
  sudo mv "$TMP/archway" "$INSTALL_DIR/archway"
fi

echo "archway installed to $INSTALL_DIR/archway"

# Auto-setup: register with AI agents
archway setup

echo ""
echo "Done! Run 'archway new my-service' to get started."
echo "Docs: https://archway.dcsg.me"
