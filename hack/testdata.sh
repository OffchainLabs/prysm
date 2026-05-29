#!/usr/bin/env bash
#
# Download the external test data that Bazel used to fetch as http_archive
# repositories (see WORKSPACE), into a local cache laid out to match the runfile
# paths the tests expect. Replaces Bazel's external-repo mechanism.
#
#   make testdata            # or: hack/testdata.sh
#   PRYSM_TESTDATA=/path make testdata
#
# Idempotent: each archive is verified against the same sha256 Bazel pins and
# skipped on subsequent runs (markers under <dest>/.markers).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
DEST="${PRYSM_TESTDATA:-third_party/testdata}"
MARKERS="$DEST/.markers"
mkdir -p "$DEST" "$MARKERS"

# Pinned versions (keep in sync with WORKSPACE).
CONSENSUS_SPEC_VERSION="v1.7.0-alpha.8"
BLS_VERSION="v0.1.1"

# sha256 (hex) or integrity (sha256-<base64>) — both accepted, matching WORKSPACE.
SHA_SPECTESTS_GENERAL="sha256-szDpBVO2Ebi8/bwbiWFpW6H4c5gxnpU3hAUS31AF02E="
SHA_SPECTESTS_MINIMAL="sha256-SBEdtQ+HwaxFCuPwzcvkJazRuur6LlMol3egANCwH4Y="
SHA_SPECTESTS_MAINNET="sha256-alrKgbLxWFRNb8/jLInQ0eJru5ScAWnxM0rEOzdm/YE="
SHA_CONSENSUS_SPEC="sha256-x0OkYCK+MJfPoEAnEmpftgl60ervC4W3zCg0KA9XiXU="
SHA_BLS="93c7d006e7c5b882cbd11dc9ec6c5d0e07f4a8c6b27a32f964eb17cf2db9763a"
SHA_EIP4881="89cb659498c0d196fc9f957f8b849b2e1a5c041c3b2b3ae5432ac5c26944297e"
SHA_EIP3076="516d551cfb3e50e4ac2f42db0992f4ceb573a7cb1616d727a725c8161485329f"
SHA_MAINNET="sha256-+mqMXyboedVw8Yp0v+U9GDz98QoC1SZET8mjaKPX+AI="
SHA_HOLESKY="sha256-htyxg8Ln2o8eCiifFN7/hcHGZg8Ir9CPzCEx+FUnnCs="
SHA_SEPOLIA="sha256-+UZgfvBcea0K0sbvAJZOz5ZNmxdWZYbohP38heUuc6w="
SHA_HOODI="sha256-G+4c9c/vci1OyPrQJnQCI+ZCv/E0cWN4hrHDY3i7ns0="

b64_to_hex() { base64 -d 2>/dev/null <<<"$1" | od -An -tx1 | tr -d ' \n' || base64 -D <<<"$1" | od -An -tx1 | tr -d ' \n'; }

verify() { # file want
  local got want="$2"
  got="$(shasum -a 256 "$1" | awk '{print $1}')"

  case "$want" in sha256-*) want="$(b64_to_hex "${want#sha256-}")" ;; esac
  if [[ "$got" != "$want" ]]; then
    echo "  hash mismatch: got $got want $want" >&2
    return 1
  fi
}

# fetch NAME URL HASH DEST_SUBDIR STRIP [INCLUDE_GLOB]
fetch() {
  local name="$1" url="$2" hash="$3" dest="$4" strip="$5" include="${6:-}"
  local marker="$MARKERS/$name"

  # Destination dir for this archive (dest "." extracts into the root, e.g. tests/
  # and the bls categories). Resolve an absolute path for display, even if it does
  # not exist yet (the script runs from the repo root, so $PWD is that root).
  local target="$DEST"
  [[ "$dest" != "." ]] && target="$DEST/$dest"
  local disp="$target"
  case "$disp" in /*) ;; *) disp="$PWD/$disp" ;; esac

  if [[ -f "$marker" && "$(cat "$marker")" == "$hash" ]]; then
    echo "✓ $name (cached) -> $disp"
    return
  fi

  echo "↓ $name -> $disp"
  local tmp
  tmp="$(mktemp)"

  curl -fL --retry 3 -s "$url" -o "$tmp"
  verify "$tmp" "$hash"

  [[ "$dest" != "." ]] && rm -rf "$target" # don't wipe the shared root for dest "."
  mkdir -p "$target"
  if [[ -n "$include" ]]; then
    tar -xzf "$tmp" -C "$target" --strip-components="$strip" --include "$include" 2>/dev/null \
      || tar -xzf "$tmp" -C "$target" --strip-components="$strip" --wildcards "$include"
  else
    tar -xzf "$tmp" -C "$target" --strip-components="$strip"
  fi
  rm -f "$tmp"
  echo "$hash" >"$marker"
}

ETH_CLIENTS="https://github.com/eth-clients"
SPEC_REL="https://github.com/ethereum/consensus-specs/releases/download/${CONSENSUS_SPEC_VERSION}"

# consensus-spec-tests flavors -> <dest>/tests/<flavor>/...
fetch consensus_spec_tests_general "${SPEC_REL}/general.tar.gz" "$SHA_SPECTESTS_GENERAL" "." 0
fetch consensus_spec_tests_minimal "${SPEC_REL}/minimal.tar.gz" "$SHA_SPECTESTS_MINIMAL" "." 0
fetch consensus_spec_tests_mainnet "${SPEC_REL}/mainnet.tar.gz" "$SHA_SPECTESTS_MAINNET" "." 0

# consensus-specs source (configs/, presets/) -> external/consensus_spec/
fetch consensus_spec \
  "https://github.com/ethereum/consensus-specs/archive/refs/tags/${CONSENSUS_SPEC_VERSION}.tar.gz" \
  "$SHA_CONSENSUS_SPEC" "external/consensus_spec" 1

# Network config repos -> external/<name>/metadata/config.yaml
fetch mainnet "${ETH_CLIENTS}/mainnet/archive/980aee8893a2291d473c38f63797d5bc370fa381.tar.gz" "$SHA_MAINNET" "external/mainnet" 1
fetch holesky_testnet "${ETH_CLIENTS}/holesky/archive/8aec65f11f0c986d6b76b2eb902420635eb9b815.tar.gz" "$SHA_HOLESKY" "external/holesky_testnet" 1
fetch sepolia_testnet "${ETH_CLIENTS}/sepolia/archive/f9158732adb1a2a6440613ad2232eb50e7384c4f.tar.gz" "$SHA_SEPOLIA" "external/sepolia_testnet" 1
fetch hoodi_testnet "${ETH_CLIENTS}/hoodi/archive/b6ee51b2045a5e7fe3efac52534f75b080b049c6.tar.gz" "$SHA_HOODI" "external/hoodi_testnet" 1

# BLS vectors -> <dest>/<category>/...  (Runfile("aggregate"), etc.)
fetch bls_spec_tests \
  "https://github.com/ethereum/bls12-381-tests/releases/download/${BLS_VERSION}/bls_tests_yaml.tar.gz" \
  "$SHA_BLS" "." 0

# EIP-3076 slashing-protection interchange tests -> external/eip3076_spec_tests/ (has generated/)
fetch eip3076_spec_tests \
  "${ETH_CLIENTS}/slashing-protection-interchange-tests/archive/refs/tags/v5.3.0.tar.gz" \
  "$SHA_EIP3076" "external/eip3076_spec_tests" 1

# EIP-4881 deposit-snapshot vectors (just the eip-4881 assets from the EIPs repo)
fetch eip4881_spec_tests \
  "https://github.com/ethereum/EIPs/archive/5480440fe51742ed23342b68cf106cefd427e39d.tar.gz" \
  "$SHA_EIP4881" "external/eip4881_spec_tests" 1 '*/assets/eip-4881/*'
