#!/usr/bin/env bash
# install-zig.sh — provision a pinned `zig` for CGO cross-compilation (Phase 4 of the
# Bazel→Go-toolchain migration; see BAZEL_MIGRATION.md). Replaces the Zig that
# hermetic_cc_toolchain v3.0.1 bundled under Bazel.
#
# Downloads the pinned zig release for the BUILD HOST (the machine running this), verifies
# its sha256, extracts it into a cache dir, and prints the absolute path to the `zig`
# binary on stdout. Idempotent: a cached, verified zig is reused. All logging goes to
# stderr so `ZIG := $(shell ./tools/cross-toolchain/install-zig.sh)` captures only the path.
#
# zig cross-compiles to every run-target (linux/darwin/windows × amd64/arm64) regardless
# of which host this runs on, so only the host's own tarball is fetched.
set -euo pipefail

# --- Pin -------------------------------------------------------------------------------
# 0.14.1: a stable, widely-used `zig cc` release (proven with Go CGO + blst). Bump the
# version and ALL five shasums together from https://ziglang.org/download/index.json.
ZIG_VERSION="0.14.1"

# sha256 per host tarball (from the signed ziglang.org download index).
sha_for() {
  case "$1" in
    x86_64-linux)   echo "24aeeec8af16c381934a6cd7d95c807a8cb2cf7df9fa40d359aa884195c4716c" ;;
    aarch64-linux)  echo "f7a654acc967864f7a050ddacfaa778c7504a0eca8d2b678839c21eea47c992b" ;;
    x86_64-macos)   echo "b0f8bdfb9035783db58dd6c19d7dea89892acc3814421853e5752fe4573e5f43" ;;
    aarch64-macos)  echo "39f3dc5e79c22088ce878edc821dedb4ca5a1cd9f5ef915e9b3cc3053e8faefa" ;;
    *) return 1 ;;
  esac
}

log() { echo "install-zig: $*" >&2; }

# --- Detect build host ----------------------------------------------------------------
case "$(uname -s)" in
  Linux)  host_os="linux" ;;
  Darwin) host_os="macos" ;;
  *) log "unsupported build OS: $(uname -s) (zig cross-builds run only from linux/macos hosts)"; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64)  host_arch="x86_64" ;;
  arm64|aarch64) host_arch="aarch64" ;;
  *) log "unsupported build arch: $(uname -m)"; exit 1 ;;
esac
host="${host_arch}-${host_os}"

want_sha="$(sha_for "$host" || true)"
if [ -z "$want_sha" ]; then
  log "no pinned zig tarball for build host '$host'"; exit 1
fi

# --- Paths ----------------------------------------------------------------------------
cache_root="${PRYSM_ZIG_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/prysm-zig}"
dest="${cache_root}/${ZIG_VERSION}/zig-${host}-${ZIG_VERSION}"
zig_bin="${dest}/zig"

# Fast path: already installed & runnable.
if [ -x "$zig_bin" ] && "$zig_bin" version >/dev/null 2>&1; then
  echo "$zig_bin"; exit 0
fi

# --- sha256 helper --------------------------------------------------------------------
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}';
  elif command -v shasum   >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}';
  else log "need sha256sum or shasum to verify the download"; return 1; fi
}

# --- Download + verify + extract ------------------------------------------------------
url="https://ziglang.org/download/${ZIG_VERSION}/zig-${host}-${ZIG_VERSION}.tar.xz"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/prysm-zig.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
tarball="${tmp}/zig.tar.xz"

log "downloading zig ${ZIG_VERSION} for ${host}"
curl -fsSL "$url" -o "$tarball"

got_sha="$(sha256_of "$tarball")"
if [ "$got_sha" != "$want_sha" ]; then
  log "checksum mismatch for $url"
  log "  expected $want_sha"
  log "  got      $got_sha"
  exit 1
fi

log "verified; extracting to ${dest}"
mkdir -p "${cache_root}/${ZIG_VERSION}"
# Atomic-ish install: extract to a temp staging dir, then move into place.
tar -xJf "$tarball" -C "$tmp"
rm -rf "$dest"
mv "${tmp}/zig-${host}-${ZIG_VERSION}" "$dest"

if [ ! -x "$zig_bin" ] || ! "$zig_bin" version >/dev/null 2>&1; then
  log "extracted zig is not runnable at ${zig_bin}"; exit 1
fi

echo "$zig_bin"
