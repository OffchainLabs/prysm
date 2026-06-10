# syntax=docker/dockerfile:1
#
# Self-contained image build: compiles the binary in-container AND assembles the faithful
# runtime image, so `docker build . -t <name>` works with no prerequisites (no `make cross`,
# no pre-built dist/). Defaults to beacon-chain; pick another binary with:
#   docker build --build-arg BIN=validator -t <name> .
#
# This is the one-command dev convenience. For release / multi-arch images use
# `make docker` (local) or `make docker-push`, which embed the exact zig-cross binaries and
# build all images at once. To keep the glibc-2.31 floor (the distroless cc-debian11 runtime
# base is glibc 2.31), the build stage uses the SAME pinned zig toolchain as `make cross`
# rather than the builder image's native gcc — so the binary matches what we ship.

ARG BASE=gcr.io/prysmaticlabs/distroless/cc-debian11@sha256:55a5e011b2c4246b4c51e01fcc2b452d151e03df052e357465f0392fcd59fddf

# ---- compile the binary with zig cc (portable blst, glibc 2.31 floor) -------------------
FROM golang:1.25-bookworm AS build
ARG BIN=beacon-chain
ARG TAG=dev
ARG TARGETARCH
# xz-utils: install-zig.sh extracts a .tar.xz (not in the stock golang image). curl/ca-certs
# for the download.
RUN apt-get update \
 && apt-get install -y --no-install-recommends xz-utils curl ca-certificates \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
# Copy the whole tree before building: go.mod has local `replace` directives pointing at
# ./third_party/*, so a go.mod-only `go mod download` layer can't resolve them. Module and
# build caches are persisted via the BuildKit cache mounts below instead.
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache \
    set -eu; \
    zig="$(./tools/cross-toolchain/install-zig.sh)"; \
    case "$TARGETARCH" in \
      amd64) triple="x86_64-linux-gnu.2.31"; extra="" ;; \
      arm64) triple="aarch64-linux-gnu.2.31"; extra="-ftree-vectorize -funsafe-math-optimizations -fomit-frame-pointer" ;; \
      *) echo "unsupported TARGETARCH '$TARGETARCH'" >&2; exit 1 ;; \
    esac; \
    CGO_ENABLED=1 CC="$zig cc -target $triple" CXX="$zig c++ -target $triple" \
      CGO_CFLAGS="-D__BLST_PORTABLE__ $extra" \
      go build -trimpath -ldflags "-X github.com/OffchainLabs/prysm/v7/runtime/version.gitTag=$TAG -s -w" \
      -o "/out/$BIN" "./cmd/$BIN"

# ---- rootfs: the Debian userland + passwd + symlinks (mirrors tools/docker/Dockerfile) --
FROM debian:bullseye-slim AS rootfs
ARG BIN=beacon-chain
ARG TARGETARCH
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY tools/docker/debian-pkgs.sh /usr/local/bin/debian-pkgs.sh
RUN /usr/local/bin/debian-pkgs.sh "$TARGETARCH" /rootfs
RUN set -eu; \
    mkdir -p /rootfs/etc /rootfs/bin "/rootfs/app/cmd/$BIN"; \
    printf 'root:x:0:0:root:/root:/bin/bash\nnonroot:x:1001:1001:nonroot:/home/nonroot:/bin/bash\n' > /rootfs/etc/passwd; \
    ln -sf /bin/bash /rootfs/bin/sh; \
    ln -sf "/$BIN" "/rootfs/app/cmd/$BIN/$BIN"; \
    ln -sf "/$BIN" /rootfs/entrypoint

# ---- final image ------------------------------------------------------------------------
FROM ${BASE}
ARG BIN=beacon-chain
LABEL org.opencontainers.image.source="https://github.com/prysmaticlabs/prysm"
COPY --from=rootfs /rootfs/ /
COPY --from=build /out/${BIN} /${BIN}
ENTRYPOINT ["/entrypoint"]
