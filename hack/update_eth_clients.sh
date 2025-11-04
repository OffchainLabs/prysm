#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 2 ]; then
  echo "Usage: $0 <network> <commit_sha> [workspace_file]" >&2
  echo "  network: mainnet | holesky | sepolia | hoodi" >&2
  exit 1
fi

NETWORK="$1"
SHA="$2"
WORKSPACE="${3:-WORKSPACE}"

if [ ! -f "$WORKSPACE" ]; then
  echo "Workspace file not found: $WORKSPACE" >&2
  exit 1
fi

case "$NETWORK" in
  mainnet)
    ARCHIVE_NAME="mainnet"
    REPO_NAME="mainnet"
    ;;
  holesky)
    ARCHIVE_NAME="holesky_testnet"
    REPO_NAME="holesky"
    ;;
  sepolia)
    ARCHIVE_NAME="sepolia_testnet"
    REPO_NAME="sepolia"
    ;;
  hoodi)
    ARCHIVE_NAME="hoodi_testnet"
    REPO_NAME="hoodi"
    ;;
  *)
    echo "Unknown network: $NETWORK (expected mainnet|holesky|sepolia|hoodi)" >&2
    exit 1
    ;;
esac

URL="https://github.com/eth-clients/${REPO_NAME}/archive/${SHA}.tar.gz"
STRIP_PREFIX="${REPO_NAME}-${SHA}"

echo "Workspace     : $WORKSPACE"
echo "Network       : $NETWORK"
echo "Archive name  : $ARCHIVE_NAME"
echo "Repo name     : $REPO_NAME"
echo "Commit SHA    : $SHA"
echo "URL           : $URL"
echo "strip_prefix  : $STRIP_PREFIX"
echo
echo "Downloading tarball to compute integrity..." >&2

TMP_TGZ="$(mktemp)"
curl -Ls "$URL" -o "$TMP_TGZ"

INTEGRITY="$(openssl dgst -sha256 -binary "$TMP_TGZ" \
  | openssl base64 -A \
  | sed 's/^/sha256-/')"

rm -f "$TMP_TGZ"

echo "Computed integrity: $INTEGRITY"
echo "Updating $WORKSPACE ..." >&2

export ARCHIVE_NAME URL STRIP_PREFIX INTEGRITY

# Update integrity
perl -0pi -e '
  my $name = $ENV{ARCHIVE_NAME};
  my $val  = $ENV{INTEGRITY};
  s/(http_archive\(\s*name\s*=\s*"$name"[\s\S]*?integrity\s*=\s*")([^"]*)(")/$1$val$3/
' "$WORKSPACE"

# Update strip_prefix
perl -0pi -e '
  my $name = $ENV{ARCHIVE_NAME};
  my $val  = $ENV{STRIP_PREFIX};
  s/(http_archive\(\s*name\s*=\s*"$name"[\s\S]*?strip_prefix\s*=\s*")([^"]*)(")/$1$val$3/
' "$WORKSPACE"

# Update url
perl -0pi -e '
  my $name = $ENV{ARCHIVE_NAME};
  my $val  = $ENV{URL};
  s/(http_archive\(\s*name\s*=\s*"$name"[\s\S]*?url\s*=\s*")([^"]*)(")/$1$val$3/
' "$WORKSPACE"

echo
echo "Done. New values for $ARCHIVE_NAME:"
echo "  url          = \"$URL\""
echo "  strip_prefix = \"$STRIP_PREFIX\""
echo "  integrity    = \"$INTEGRITY\""
