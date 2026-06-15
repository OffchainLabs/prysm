# syntax=docker/dockerfile:1
#
# Cross-toolchain builder image

FROM golang:1.26-bookworm@sha256:5f68ec6805843bd3981a951ffada82a26a0bd2631045c8f7dba483fa868f5ec5 AS builder

ENV PRYSM_ZIG_CACHE=/opt/prysm-zig OSXCROSS_PREFIX=/usr/osxcross DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates xz-utils \
    && rm -rf /var/lib/apt/lists/*

COPY tools/cross-toolchain/ /opt/cross-toolchain/

RUN /opt/cross-toolchain/install-zig.sh >/dev/null \
    && /opt/cross-toolchain/install-mingw.sh >/dev/null \
    && /opt/cross-toolchain/install-osxcross.sh >/dev/null \
    && rm -rf /var/lib/apt/lists/*
