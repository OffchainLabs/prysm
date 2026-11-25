# syntax=docker/dockerfile:1
#
# Prysm image for the Kurtosis workflows in .github/workflows.
#
# ethereum-package starts the beacon node with `sh -c "exec /beacon-chain ..."`
# and ignores ENTRYPOINT, so the root image (distroless, /entrypoint only) fails.
#
# Differences from the root Dockerfile:
#   - debian:bookworm-slim instead of distroless.
#   - Binary at /$BIN instead of /entrypoint.
#   - /entrypoint is a symlink to /$BIN.
#   - CA bundle copied from the build stage.
#
# Usage (from the repo root):
#   docker build -f .github/kurtosis.Dockerfile --build-arg BIN=beacon-chain|validator -t <name> .

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

FROM debian:bookworm-slim
ARG BIN=beacon-chain
LABEL org.opencontainers.image.source="https://github.com/OffchainLabs/prysm"
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/${BIN} /${BIN}
RUN ln -s "/${BIN}" /entrypoint
ENTRYPOINT ["/entrypoint"]
