# syntax=docker/dockerfile:1
#
# This Dockerfile builds a distroless image for the Prysm client.

ARG BASE=gcr.io/prysmaticlabs/distroless/cc-debian11@sha256:55a5e011b2c4246b4c51e01fcc2b452d151e03df052e357465f0392fcd59fddf

FROM debian:bullseye-slim AS rootfs
ARG BIN
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates && rm -rf /var/lib/apt/lists/*
COPY tools/docker/debian-pkgs.sh /usr/local/bin/debian-pkgs.sh
RUN /usr/local/bin/debian-pkgs.sh "$TARGETARCH" /rootfs
RUN set -eu; \
    mkdir -p /rootfs/etc /rootfs/bin "/rootfs/app/cmd/$BIN"; \
    printf 'root:x:0:0:root:/root:/bin/bash\nnonroot:x:1001:1001:nonroot:/home/nonroot:/bin/bash\n' > /rootfs/etc/passwd; \
    ln -sf /bin/bash /rootfs/bin/sh; \
    ln -sf "/$BIN" "/rootfs/app/cmd/$BIN/$BIN"; \
    ln -sf "/$BIN" /rootfs/entrypoint

FROM ${BASE}
ARG BIN
ARG TAG
ARG TARGETARCH
LABEL org.opencontainers.image.source="https://github.com/prysmaticlabs/prysm"
COPY --from=rootfs /rootfs/ /
COPY dist/${BIN}-${TAG}-linux-${TARGETARCH} /${BIN}
ENTRYPOINT ["/entrypoint"]
