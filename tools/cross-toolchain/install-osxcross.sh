#!/usr/bin/env bash
# install-osxcross.sh — ensure the osxcross Linux->macOS cross toolchain
# (o64-clang / oa64-clang) is available for cross-compiling the darwin release binaries
# (Phase 4). Idempotent: a no-op if already installed. Logs to stderr; prints the osxcross
# bin dir on stdout.
#
# This is the high-level provisioner. The heavy lifting (download MacOSX12.3 SDK, build
# osxcross) is delegated to the unchanged Bazel-era scripts in this directory:
#   install_osxcross.sh  (underscore) + link_osxcross.sh
# which the Bazel RBE worker image also used. osxcross builds only on Linux.
set -euo pipefail

log() { echo "install-osxcross: $*" >&2; }
PREFIX="${OSXCROSS_PREFIX:-/usr/osxcross}"
here="$(cd "$(dirname "$0")" && pwd)"

SUDO=""
if [ "$(id -u)" -ne 0 ] && command -v sudo >/dev/null 2>&1; then SUDO="sudo"; fi

# crypto/bls/herumi links herumi's prebuilt darwin lib, whose cgo LDFLAGS hardcode
# `-lstdc++`. The macOS SDK only ships libc++ (no libstdc++), so add a libstdc++ -> libc++
# symlink: it lets the linker resolve -lstdc++, while the real C++ symbols come from libc++
# (linked implicitly via clang's -stdlib=libc++). Idempotent; runs on every invocation so an
# already-installed osxcross also gets the shim.
ensure_stdcxx_shim() {
  for sdklib in "$PREFIX"/SDK/MacOSX*.sdk/usr/lib; do
    [ -e "$sdklib/libc++.tbd" ] || continue
    if [ ! -e "$sdklib/libstdc++.tbd" ]; then
      log "adding libstdc++ -> libc++ compat shim in $sdklib"
      $SUDO ln -sf libc++.tbd "$sdklib/libstdc++.tbd"
    fi
  done
}

if [ -x "$PREFIX/bin/o64-clang" ] && [ -x "$PREFIX/bin/oa64-clang" ]; then
  ensure_stdcxx_shim
  echo "$PREFIX/bin"; exit 0
fi

if [ "$(uname -s)" != "Linux" ]; then
  log "osxcross builds only on Linux; cannot provision here"; exit 1
fi

if ! command -v apt-get >/dev/null 2>&1; then
  log "automatic osxcross provisioning currently supports apt-based hosts only"
  log "install the build deps + run install_osxcross.sh + link_osxcross.sh manually"; exit 1
fi

log "installing osxcross build dependencies via apt-get"
$SUDO apt-get update -qq >&2
$SUDO apt-get install -y -qq \
  clang cmake patch libssl-dev libxml2-dev zlib1g-dev liblzma-dev libbz2-dev \
  uuid-dev libtool make python3 xz-utils curl >&2

log "building osxcross (downloads MacOSX12.3 SDK; this is slow the first time)"
$SUDO "$here/install_osxcross.sh" >&2
$SUDO "$here/link_osxcross.sh" >&2

ensure_stdcxx_shim

if [ ! -x "$PREFIX/bin/o64-clang" ] || [ ! -x "$PREFIX/bin/oa64-clang" ]; then
  log "osxcross wrappers not found under $PREFIX/bin after build"; exit 1
fi
log "osxcross ready at $PREFIX/bin"
echo "$PREFIX/bin"
