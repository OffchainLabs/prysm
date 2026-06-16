#!/bin/bash

# -----------------------------------------------------------------------------
# Builds and pushes the Prysm container images to the registry.
#
# Replaces the former Bazel rules_oci push (Phase 5/9 of the Bazel->Go-toolchain
# migration). The Go/Make `build/docker` writes one single-arch image tarball per arch
# (dist/<bin>-<tag>-linux-<arch>.tar) rather than pushing — so this script:
#   1. builds those tarballs with `make dist platform=docker/...`,
#   2. loads each, pushes it under an arch-suffixed tag (<ref>-<arch>), and
#   3. assembles a multi-arch manifest list per image with `docker buildx imagetools`.
# The result matches the old Bazel publish: beacon-chain as <tag> and <tag>-portable,
# validator and prysmctl as <tag> (the tags are baked into the tarballs by build/docker).
#
# Requires a logged-in Docker with buildx (the Release workflow logs in to GCR first).
# -----------------------------------------------------------------------------

set -euo pipefail

# Validate that the tag argument exists.
if [ "${1:-}" = "" ]; then
  echo "Usage: $0 <tag>"
  exit 1
fi
TAG=$1

ARCHES="amd64 arm64"

# 1. Build the per-arch image tarballs into dist/. DOCKER_TAG tags the images :$TAG (and
#    beacon-chain additionally :$TAG-portable); DOCKER_REGISTRY defaults to gcr.io/offchainlabs/prysm.
make dist platform=docker/amd64,docker/arm64 DOCKER_TAG="$TAG"

# 2. Load each arch's tarballs and push an arch-suffixed tag for every image ref they carry.
#    `docker load` prints "Loaded image: <ref>" — we read those so the script stays agnostic
#    of the registry/repo names (build/docker baked them in). Collected base refs (without the
#    -<arch> suffix) drive the manifest assembly in step 3.
refs_file=$(mktemp)
for arch in $ARCHES; do
  for tar in dist/*-linux-"$arch".tar; do
    if [ ! -e "$tar" ]; then
      echo "❌ no docker image tarball for $arch (expected $tar)" >&2
      exit 1
    fi
    docker load -i "$tar" | sed -n 's/^Loaded image: //p' | while read -r ref; do
      echo "→ pushing $ref-$arch"
      docker tag "$ref" "$ref-$arch"
      docker push "$ref-$arch"
      echo "$ref" >> "$refs_file"
    done
  done
done

# 3. For each image, create and push a multi-arch manifest list pointing at its arch tags.
sort -u "$refs_file" | while read -r ref; do
  srcs=""
  for arch in $ARCHES; do
    srcs="$srcs $ref-$arch"
  done
  echo "→ publishing multi-arch manifest $ref ($srcs )"
  docker buildx imagetools create -t "$ref" $srcs
done

rm -f "$refs_file"
echo "✅ pushed multi-arch images for tag $TAG"
