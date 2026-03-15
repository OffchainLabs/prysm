#!/bin/bash
set -euo pipefail

# Downloads consensus spec test vectors and config data needed for tests.
# Usage: ./hack/download-spec-tests.sh [version]
#
# Environment variables:
#   CONSENSUS_SPEC_TESTS_VERSION - Override the pinned spec tests version
#   GITHUB_TOKEN - Required for nightly builds

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Pinned versions from the former WORKSPACE file.
DEFAULT_SPEC_TESTS_VERSION="v1.7.0-alpha.2"
CONSENSUS_SPEC_VERSION="${CONSENSUS_SPEC_TESTS_VERSION:-${1:-$DEFAULT_SPEC_TESTS_VERSION}}"

HOLESKY_COMMIT="8aec65f11f0c986d6b76b2eb902420635eb9b815"
HOODI_COMMIT="b6ee51b2045a5e7fe3efac52534f75b080b049c6"
SEPOLIA_COMMIT="f9158732adb1a2a6440613ad2232eb50e7384c4f"
MAINNET_COMMIT="980aee8893a2291d473c38f63797d5bc370fa381"

echo "==> Consensus spec tests version: ${CONSENSUS_SPEC_VERSION}"

# --- Helper ---
download_and_extract() {
    local url="$1"
    local dest="$2"
    local strip="${3:-0}"

    if [ -d "$dest" ] && [ "$(ls -A "$dest" 2>/dev/null)" ]; then
        echo "    Already exists: $dest (skipping)"
        return
    fi

    mkdir -p "$dest"
    local tmp
    tmp=$(mktemp)
    echo "    Downloading: $url"
    curl -sSfL -o "$tmp" "$url"
    tar -xzf "$tmp" --strip-components="$strip" -C "$dest"
    rm -f "$tmp"
}

# --- 1. Spec test vectors ---
# Each archive contains a top-level tests/ directory with the flavor subdirectory
# (e.g. tests/general/...). We extract all three to the repo root so they merge
# into a single tests/ tree.
echo "==> Downloading spec test vectors into tests/"
RELEASE_BASE="https://github.com/ethereum/consensus-specs/releases/download/${CONSENSUS_SPEC_VERSION}"

for flavor in general minimal mainnet; do
    FLAVOR_DIR="$REPO_ROOT/tests/${flavor}"
    if [ -d "$FLAVOR_DIR" ] && [ "$(ls -A "$FLAVOR_DIR" 2>/dev/null)" ]; then
        echo "  -> ${flavor} (already exists, skipping)"
        continue
    fi
    echo "  -> ${flavor}"
    tmp=$(mktemp)
    echo "    Downloading: ${RELEASE_BASE}/${flavor}.tar.gz"
    curl -sSfL -o "$tmp" "${RELEASE_BASE}/${flavor}.tar.gz"
    tar -xzf "$tmp" -C "$REPO_ROOT"
    rm -f "$tmp"
done

# --- 2. Consensus specs (configs + presets) ---
echo "==> Downloading consensus specs into external/consensus_spec/"
CONSENSUS_SPEC_STRIP_PREFIX="consensus-specs-${CONSENSUS_SPEC_VERSION#v}"
download_and_extract \
    "https://github.com/ethereum/consensus-specs/archive/refs/tags/${CONSENSUS_SPEC_VERSION}.tar.gz" \
    "$REPO_ROOT/external/consensus_spec" \
    1

# --- 3. Testnet configs ---
echo "==> Downloading testnet configs into external/..."

echo "  -> holesky"
download_and_extract \
    "https://github.com/eth-clients/holesky/archive/${HOLESKY_COMMIT}.tar.gz" \
    "$REPO_ROOT/external/holesky_testnet" \
    1

echo "  -> hoodi"
download_and_extract \
    "https://github.com/eth-clients/hoodi/archive/${HOODI_COMMIT}.tar.gz" \
    "$REPO_ROOT/external/hoodi_testnet" \
    1

echo "  -> sepolia"
download_and_extract \
    "https://github.com/eth-clients/sepolia/archive/${SEPOLIA_COMMIT}.tar.gz" \
    "$REPO_ROOT/external/sepolia_testnet" \
    1

echo "  -> mainnet"
download_and_extract \
    "https://github.com/eth-clients/mainnet/archive/${MAINNET_COMMIT}.tar.gz" \
    "$REPO_ROOT/external/mainnet" \
    1

echo "==> Done. Test data is ready."
