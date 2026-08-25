#!/bin/bash
set -eo pipefail

# Repins the lighthouse release used by the multiclient e2e tests, by rewriting
# `lighthouse_version` and the `lighthouse_integrity` map in
# testing/endtoend/deps.bzl.
#
# Usage:
#   hack/update-lighthouse.sh           # latest lighthouse release
#   hack/update-lighthouse.sh v8.2.0    # a specific release
#
# For each target-triple the e2e harness supports, the matching release asset is
# downloaded and its Bazel "integrity" string (sha256-<base64>) is written back.
# Requires curl and openssl.

# Keep in sync with LighthouseTriple in build/externaldata/externaldata.go.
triples=(
    x86_64-unknown-linux-gnu
    aarch64-unknown-linux-gnu
    aarch64-apple-darwin
)

root="$(cd "$(dirname "$0")/.." && pwd)"
deps="$root/testing/endtoend/deps.bzl"

version="${1:-}"
if [ -z "$version" ]; then
    version=$(curl -fsSL https://api.github.com/repos/sigp/lighthouse/releases/latest |
        perl -nle 'print $1 and exit if /"tag_name":\s*"([^"]+)"/')
    if [ -z "$version" ]; then
        echo "❌ could not resolve the latest lighthouse release, pass a version explicitly" >&2
        exit 1
    fi
    echo "Latest lighthouse release: $version"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

entries=""
for triple in "${triples[@]}"; do
    asset="lighthouse-$version-$triple.tar.gz"
    url="https://github.com/sigp/lighthouse/releases/download/$version/$asset"

    echo "-> $url"
    if ! curl -fsSL -o "$tmp/$asset" "$url"; then
        echo "❌ download failed: $url" >&2
        exit 1
    fi

    integrity="sha256-$(openssl dgst -sha256 -binary "$tmp/$asset" | openssl base64 -A)"
    entries+="    \"$triple\": \"$integrity\","$'\n'
done

VERSION="$version" perl -pi -e 's/^lighthouse_version = "[^"]*"$/lighthouse_version = "$ENV{VERSION}"/' "$deps"
ENTRIES="$entries" perl -0pi -e 's/lighthouse_integrity = \{.*?\n\}/lighthouse_integrity = {\n$ENV{ENTRIES}}/s' "$deps"

echo
echo "✅ pinned lighthouse $version in testing/endtoend/deps.bzl:"
git -C "$root" --no-pager diff -- testing/endtoend/deps.bzl
echo
echo "Now run the multiclient e2e tests: make e2e multiclient"
