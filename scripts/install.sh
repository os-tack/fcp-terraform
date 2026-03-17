#!/bin/bash
set -euo pipefail

# fcp-terraform installer — downloads the latest release binary for the current platform.

REPO="os-tack/fcp-terraform"
BINARY="fcp-terraform"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux|darwin) ;;
  mingw*|msys*|cygwin*) OS="windows" ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect arch
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Get latest version tag
VERSION=$(curl -sI "https://github.com/${REPO}/releases/latest" | grep -i '^location:' | sed 's/.*tag\///' | tr -d '\r\n')
if [ -z "$VERSION" ]; then
  echo "Failed to determine latest version." >&2
  exit 1
fi
VERSION_NUM="${VERSION#v}"

# Determine install directory
INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

# Download and extract
EXT="tar.gz"
[ "$OS" = "windows" ] && EXT="zip"

URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}_${VERSION_NUM}_${OS}_${ARCH}.${EXT}"
echo "Downloading ${BINARY} ${VERSION} for ${OS}/${ARCH}..."

if [ "$EXT" = "tar.gz" ]; then
  curl -LsSf "$URL" | tar xz -C "$INSTALL_DIR" "$BINARY"
else
  TMP=$(mktemp -d)
  curl -LsSf "$URL" -o "${TMP}/${BINARY}.zip"
  unzip -qo "${TMP}/${BINARY}.zip" "$BINARY.exe" -d "$INSTALL_DIR"
  rm -rf "$TMP"
fi

echo "Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"

# Check PATH
if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
  echo ""
  echo "NOTE: ${INSTALL_DIR} is not on your PATH. Add it:"
  echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
fi
