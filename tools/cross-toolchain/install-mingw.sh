#!/usr/bin/env bash
# install-mingw.sh — ensure the mingw-w64 C/C++ cross-compiler is available, for
# cross-compiling the windows/amd64 release binaries (Phase 4). Linux build host only.
#
# Idempotent: a no-op if already present. Installs via the host package manager otherwise
# (uses sudo when not root). Logs to stderr; prints the gcc path on stdout.
#
# Note: both the C *and* C++ compilers are required — crypto/bls/herumi links herumi's
# prebuilt C++ static lib, so libstdc++ (from g++-mingw-w64) must be present, and we pin the
# POSIX threads variant (the one that ships a linkable libstdc++).
set -euo pipefail

log() { echo "install-mingw: $*" >&2; }
GCC="x86_64-w64-mingw32-gcc"

if command -v "$GCC" >/dev/null 2>&1 && command -v x86_64-w64-mingw32-g++ >/dev/null 2>&1; then
  command -v "$GCC"; exit 0
fi

if [ "$(uname -s)" != "Linux" ]; then
  log "make cross runs on a Linux host only; cannot provision mingw-w64 on $(uname -s)"; exit 1
fi

SUDO=""
if [ "$(id -u)" -ne 0 ] && command -v sudo >/dev/null 2>&1; then SUDO="sudo"; fi

if command -v apt-get >/dev/null 2>&1; then
  log "installing gcc/g++-mingw-w64-x86-64 via apt-get"
  $SUDO apt-get update -qq >&2
  $SUDO apt-get install -y -qq gcc-mingw-w64-x86-64 g++-mingw-w64-x86-64 >&2
  # Select the POSIX threads variant (ships a linkable libstdc++ for herumi).
  $SUDO update-alternatives --set x86_64-w64-mingw32-gcc /usr/bin/x86_64-w64-mingw32-gcc-posix >/dev/null 2>&1 || true
  $SUDO update-alternatives --set x86_64-w64-mingw32-g++ /usr/bin/x86_64-w64-mingw32-g++-posix >/dev/null 2>&1 || true
elif command -v dnf >/dev/null 2>&1; then
  log "installing mingw64-gcc/gcc-c++ via dnf"
  $SUDO dnf install -y mingw64-gcc mingw64-gcc-c++ >&2
elif command -v pacman >/dev/null 2>&1; then
  log "installing mingw-w64-gcc via pacman"
  $SUDO pacman -S --noconfirm mingw-w64-gcc >&2
else
  log "no supported package manager (apt-get/dnf/pacman); install mingw-w64 (gcc+g++) manually"; exit 1
fi

if ! command -v "$GCC" >/dev/null 2>&1 || ! command -v x86_64-w64-mingw32-g++ >/dev/null 2>&1; then
  log "mingw-w64 gcc/g++ still not on PATH after install"; exit 1
fi
command -v "$GCC"
