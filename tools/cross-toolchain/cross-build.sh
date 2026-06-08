#!/usr/bin/env bash
# cross-build.sh — cross-compile the distributed Prysm binaries for every run-target from a
# Linux x86_64 host (Phase 4 of the Bazel->Go-toolchain migration). Driven by `make cross`,
# which exports the configuration below; it also runs standalone with sensible defaults.
#
# Each C toolchain is auto-provisioned (idempotently) by a sibling script:
#   linux   -> install-zig.sh        (hermetic zig cc; triple pins glibc 2.31)
#   darwin  -> install-osxcross.sh   (osxcross o64/oa64-clang; needs osxcross on PATH for ld64)
#   windows -> install-mingw.sh      (mingw-w64; herumi's prebuilt lib needs libstdc++)
# blst defaults to portable (-D__BLST_PORTABLE__); the amd64 beacon-chain also gets a
# -modern- (ADX) artifact. Output names match what prysm.sh / prysm.bat fetch.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"

# --- config (exported by the Makefile; defaults keep the script runnable on its own) -----
GO="${GO:-go}"
DIST="${DIST:-dist}"
TAG="${GIT_TAG:-$(git describe --tags --abbrev=0 2>/dev/null || echo Unknown)}"
BINARIES="${CROSS_BINARIES:-beacon-chain validator client-stats prysmctl}"
TARGETS="${CROSS_TARGETS:-linux/amd64/x86_64-linux-gnu.2.31 linux/arm64/aarch64-linux-gnu.2.31 darwin/amd64/x86_64-macos darwin/arm64/aarch64-macos windows/amd64/x86_64-windows-gnu}"
ARM64_CFLAGS="${CGO_CFLAGS_LINUX_ARM64:-}"
BLST_PORTABLE="${BLST_PORTABLE:--D__BLST_PORTABLE__}"
LDFLAGS="${LDFLAGS:-}"
TAGFLAG="${TAGFLAG:-}"
PGO_BEACON_CHAIN="${PGO_beacon_chain:-}"

# --- host guard: Linux x86_64 only -------------------------------------------------------
if [ "$(uname -s)" != "Linux" ]; then
  echo "❌ make cross runs on a Linux x86_64 host only (it provisions zig + mingw-w64 + osxcross to build all run-targets). Current host: $(uname -s)/$(uname -m)." >&2
  exit 1
fi

zig="$("$here/install-zig.sh")"
mkdir -p "$DIST"

# --- total artifact count for the [n/m] progress counter ---------------------------------
nbins=$(set -- $BINARIES; echo $#)
has_bc=0
for b in $BINARIES; do if [ "$b" = "beacon-chain" ]; then has_bc=1; fi; done
m=0
for t in $TARGETS; do
  a="${t#*/}"; a="${a%%/*}"
  m=$((m + nbins))
  if [ "$a" = "amd64" ] && [ "$has_bc" -eq 1 ]; then m=$((m + 1)); fi
done

n=0
# build <outfile> <pkg> <cgo_cflags> <pgo_flag> <label>
build() {
  n=$((n + 1))
  echo "[$n/$m] → $5"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=1 PATH="$pathpfx$PATH" CC="$cc" CXX="$cxx" \
    CGO_CFLAGS="$3" \
    "$GO" build $TAGFLAG -trimpath $4 -ldflags "$LDFLAGS -s -w" -o "$1" "$2"
}

for tgt in $TARGETS; do
  os="${tgt%%/*}"; rest="${tgt#*/}"; arch="${rest%%/*}"; triple="${rest##*/}"
  if [ "$os" = "windows" ]; then ext=".exe"; else ext=""; fi
  extra=""; pathpfx=""
  case "$os" in
    linux)
      cc="$zig cc -target $triple"; cxx="$zig c++ -target $triple"
      if [ "$arch" = "arm64" ]; then extra="$ARM64_CFLAGS"; fi
      ;;
    darwin)
      obin="$("$here/install-osxcross.sh")"; pathpfx="$obin:"
      if [ "$arch" = "amd64" ]; then cc="$obin/o64-clang"; cxx="$obin/o64-clang++";
      else cc="$obin/oa64-clang"; cxx="$obin/oa64-clang++"; fi
      ;;
    windows)
      "$here/install-mingw.sh" >/dev/null
      cc="x86_64-w64-mingw32-gcc"; cxx="x86_64-w64-mingw32-g++"
      ;;
    *) echo "cross-build: unknown target OS '$os'" >&2; exit 1 ;;
  esac

  for bin in $BINARIES; do
    if [ "$bin" = "beacon-chain" ]; then pgo="$PGO_BEACON_CHAIN"; else pgo=""; fi
    build "$DIST/$bin-$TAG-$os-$arch$ext" "./cmd/$bin" "$BLST_PORTABLE $extra" "$pgo" \
      "$os/$arch  $bin  (portable)"
    if [ "$bin" = "beacon-chain" ] && [ "$arch" = "amd64" ]; then
      build "$DIST/beacon-chain-$TAG-modern-$os-$arch$ext" "./cmd/beacon-chain" "$extra" "$pgo" \
        "$os/$arch  beacon-chain  (modern/ADX)"
    fi
  done
done

echo "✅ cross: built $n/$m artifact(s) → $DIST/"
