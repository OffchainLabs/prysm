#!/bin/bash

# -----------------------------------------------------------------------------
# This script builds and uploads docker images to the registries using
# docker buildx for multi-arch support.
# -----------------------------------------------------------------------------

# Validate that the tag argument exists.
if [ "$1" = "" ]
then
  echo "Usage: $0 <tag>"
  exit
fi
TAG=$1

# Build and push beacon-chain
echo "Building and pushing beacon-chain image..."
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --tag gcr.io/prysmaticlabs/prysm/beacon-chain:${TAG} \
  --file cmd/beacon-chain/Dockerfile \
  --push .

# Build and push validator
echo "Building and pushing validator image..."
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --tag gcr.io/prysmaticlabs/prysm/validator:${TAG} \
  --file cmd/validator/Dockerfile \
  --push .

# Build and push prysmctl
echo "Building and pushing prysmctl image..."
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --tag gcr.io/prysmaticlabs/prysm/prysmctl:${TAG} \
  --file cmd/prysmctl/Dockerfile \
  --push .
