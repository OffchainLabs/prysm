#!/bin/bash
set -euo pipefail

# Downloads external binaries needed for E2E tests.
# Binaries are placed in ./bin/
#
# Supported platforms: linux/amd64 (matching CI). darwin/arm64 builds must be
# compiled from source.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$REPO_ROOT/bin"

LIGHTHOUSE_VERSION="v7.0.0-beta.0"
WEB3SIGNER_VERSION="25.9.1"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

mkdir -p "$BIN_DIR"

# --- Lighthouse ---
echo "==> Lighthouse ${LIGHTHOUSE_VERSION}"
if [ -x "$BIN_DIR/lighthouse" ]; then
    echo "    Already exists: $BIN_DIR/lighthouse (skipping)"
else
    case "${OS}-${ARCH}" in
        linux-x86_64)
            LIGHTHOUSE_ARCHIVE="lighthouse-${LIGHTHOUSE_VERSION}-x86_64-unknown-linux-gnu.tar.gz"
            ;;
        linux-aarch64)
            LIGHTHOUSE_ARCHIVE="lighthouse-${LIGHTHOUSE_VERSION}-aarch64-unknown-linux-gnu.tar.gz"
            ;;
        darwin-arm64)
            LIGHTHOUSE_ARCHIVE="lighthouse-${LIGHTHOUSE_VERSION}-aarch64-apple-darwin.tar.gz"
            ;;
        darwin-x86_64)
            LIGHTHOUSE_ARCHIVE="lighthouse-${LIGHTHOUSE_VERSION}-x86_64-apple-darwin.tar.gz"
            ;;
        *)
            echo "    WARNING: No pre-built Lighthouse binary for ${OS}-${ARCH}. Build from source."
            LIGHTHOUSE_ARCHIVE=""
            ;;
    esac

    if [ -n "${LIGHTHOUSE_ARCHIVE:-}" ]; then
        URL="https://github.com/sigp/lighthouse/releases/download/${LIGHTHOUSE_VERSION}/${LIGHTHOUSE_ARCHIVE}"
        echo "    Downloading: $URL"
        tmp=$(mktemp)
        curl -sSfL -o "$tmp" "$URL"
        tar -xzf "$tmp" -C "$BIN_DIR" lighthouse
        rm -f "$tmp"
        chmod +x "$BIN_DIR/lighthouse"
        echo "    Installed: $BIN_DIR/lighthouse"
    fi
fi

# --- Web3Signer ---
echo "==> Web3Signer ${WEB3SIGNER_VERSION}"
if [ -d "$BIN_DIR/web3signer" ]; then
    echo "    Already exists: $BIN_DIR/web3signer (skipping)"
else
    URL="https://github.com/Consensys/web3signer/releases/download/${WEB3SIGNER_VERSION}/web3signer-${WEB3SIGNER_VERSION}.tar.gz"
    echo "    Downloading: $URL"
    tmp=$(mktemp)
    curl -sSfL -o "$tmp" "$URL"
    tar -xzf "$tmp" -C "$BIN_DIR"
    mv "$BIN_DIR/web3signer-${WEB3SIGNER_VERSION}" "$BIN_DIR/web3signer"
    rm -f "$tmp"
    echo "    Installed: $BIN_DIR/web3signer/"
fi

echo ""
echo "==> Done. E2E binaries are in $BIN_DIR/"
echo ""
echo "NOTE: E2E tests also require prysm binaries. Build them with:"
echo "  go build -o ./bin/beacon-chain ./cmd/beacon-chain"
echo "  go build -o ./bin/validator ./cmd/validator"
