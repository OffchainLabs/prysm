# syntax=docker/dockerfile:1
#
# Faithful reproduction of the Bazel rules_oci image (tools/prysm_image.bzl): the pinned
# prysm distroless cc-debian11 base + a layered Debian-11 userland (bash + coreutils),
# /etc/passwd (root + nonroot), /bin/sh -> /bin/bash, and the backwards-compat symlink.
# One parameterized, multi-stage Dockerfile for all images (BIN build-arg). The binaries are
# pre-built by `make cross-build` (Phase 4) and live in ./dist; this image only assembles them.
#
# Build (driven by `make docker` / `make docker-push`):
#   docker buildx build --build-arg BIN=beacon-chain --build-arg TAG=<tag> \
#     --platform linux/amd64,linux/arm64 -f tools/docker/Dockerfile -t <repo>:<tag> .

ARG BASE=gcr.io/prysmaticlabs/distroless/cc-debian11@sha256:55a5e011b2c4246b4c51e01fcc2b452d151e03df052e357465f0392fcd59fddf

# ---- rootfs: assemble the Debian userland + passwd + symlinks (needs a shell) -----------
FROM debian:bullseye-slim AS rootfs
ARG BIN
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates && rm -rf /var/lib/apt/lists/*
COPY tools/docker/debian-pkgs.sh /usr/local/bin/debian-pkgs.sh
RUN /usr/local/bin/debian-pkgs.sh "$TARGETARCH" /rootfs
# /etc/passwd (root + nonroot), /bin/sh -> /bin/bash, and the symlinks. The compat and
# entrypoint symlinks point at /$BIN, which the final stage adds (dangling here is fine).
RUN set -eu; \
    mkdir -p /rootfs/etc /rootfs/bin "/rootfs/app/cmd/$BIN"; \
    printf 'root:x:0:0:root:/root:/bin/bash\nnonroot:x:1001:1001:nonroot:/home/nonroot:/bin/bash\n' > /rootfs/etc/passwd; \
    ln -sf /bin/bash /rootfs/bin/sh; \
    ln -sf "/$BIN" "/rootfs/app/cmd/$BIN/$BIN"; \
    ln -sf "/$BIN" /rootfs/entrypoint

# ---- final image: distroless base + assembled rootfs + the binary ----------------------
FROM ${BASE}
ARG BIN
ARG TAG
ARG TARGETARCH
LABEL org.opencontainers.image.source="https://github.com/prysmaticlabs/prysm"
COPY --from=rootfs /rootfs/ /
COPY dist/${BIN}-${TAG}-linux-${TARGETARCH} /${BIN}
ENTRYPOINT ["/entrypoint"]
