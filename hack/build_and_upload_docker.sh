#!/bin/bash

# -----------------------------------------------------------------------------
# Builds and pushes the Prysm container images to the registry.
#
# Replaces the former Bazel rules_oci push (Phase 5/9 of the Bazel->Go-toolchain
# migration) with a single `make build docker=true push=true` — which cross-builds the
# linux/amd64+arm64 portable binaries (via zig) and pushes a multi-arch manifest per image
# with `docker buildx`. As before, beacon-chain is tagged both `<tag>` and `<tag>-portable`,
# and validator + prysmctl are tagged `<tag>` (see build/docker).
# -----------------------------------------------------------------------------

set -e

# Validate that the tag argument exists.
if [ "$1" = "" ]
then
  echo "Usage: $0 <tag>"
  exit 1
fi
TAG=$1

make build docker=true push=true mode=release DOCKER_TAG="$TAG"
