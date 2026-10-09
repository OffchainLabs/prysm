# syntax=docker/dockerfile:1
#
# This docker file builds a FAST, non-hermetic image for local iteration (Kurtosis / devnets / ...).
# It is NOT suitable for releases.
# To build a hermetic, reproducible image for releases, use `make dist`.
#
# Usage:
#   docker build [--build-arg BIN=beacon-chain|validator|prysmctl] -t <name> .
#   Default BIN is beacon-chain.

FROM golang:1.26-bookworm AS build
ARG BIN=beacon-chain
ARG TAG=dev

WORKDIR /src

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
  CGO_ENABLED=1 CGO_CFLAGS="-D__BLST_PORTABLE__" \
  go build \
  -ldflags "-X github.com/OffchainLabs/prysm/v7/runtime/version.gitTag=$TAG" \
  -o "/out/$BIN" "./cmd/$BIN"

# The debug variant ships a busybox shell, which Kurtosis' ethereum-package needs to start the beacon node.
FROM gcr.io/distroless/cc-debian12:debug
ARG BIN=beacon-chain
LABEL org.opencontainers.image.source="https://github.com/OffchainLabs/prysm"
# Kurtosis' ethereum-package runs the beacon node as /beacon-chain, like the `make dist` images.
COPY --from=build /out/${BIN} /${BIN}
SHELL ["/busybox/sh", "-c"]
RUN ln -s "/${BIN}" /entrypoint
ENTRYPOINT ["/entrypoint"]
