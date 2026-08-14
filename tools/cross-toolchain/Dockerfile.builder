# syntax=docker/dockerfile:1
#
# Cross-toolchain builder image
# -----------------------------
#
# This docker file builds a cross-toolchain builder image that can be used to build binaries for multiple architectures.

FROM golang:1.26-bookworm@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651 AS builder

ENV PRYSM_ZIG_CACHE=/opt/prysm-zig OSXCROSS_PREFIX=/usr/osxcross DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates xz-utils \
    && rm -rf /var/lib/apt/lists/*

COPY tools/cross-toolchain/ /opt/cross-toolchain/

RUN /opt/cross-toolchain/install-zig.sh >/dev/null \
    && /opt/cross-toolchain/install-mingw.sh >/dev/null \
    && /opt/cross-toolchain/install-osxcross.sh >/dev/null \
    && rm -rf /var/lib/apt/lists/*
