#!/usr/bin/env bash
# debian-pkgs.sh <amd64|arm64> <rootfs-dir>
#
# Download, checksum-verify, and extract Debian-11 userland packages
set -euo pipefail

arch="${1:?usage: debian-pkgs.sh <amd64|arm64> <rootfs-dir>}"
rootfs="${2:?usage: debian-pkgs.sh <amd64|arm64> <rootfs-dir>}"

SNAP="https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main"
MIRROR="https://prysmaticlabs.com/uploads"

# Each entry: "<sha256>  <pool/sub/path/file.deb>" (mirror URL uses the basename).
case "$arch" in
  amd64) pkgs=(
    "f702ef058e762d7208a9c83f6f6bbf02645533bfd615c54e8cdcce842cd57377  b/bash/bash_5.1-2+deb11u1_amd64.deb"
    "96ed58b8fd656521e08549c763cd18da6cff1b7801a3a22f29678701a95d7e7b  n/ncurses/libtinfo6_6.2+20201114-2+deb11u2_amd64.deb"
    "3558a412ab51eee4b60641327cb145bb91415f127769823b68f9335585b308d4  c/coreutils/coreutils_8.32-4+b1_amd64.deb"
    "339f5ede10500c16dd7192d73169c31c4b27ab12130347275f23044ec8c7d897  libs/libselinux/libselinux1_3.1-3_amd64.deb"
    "ee192c8d22624eb9d0a2ae95056bad7fb371e5abc17e23e16b1de3ddb17a1064  p/pcre2/libpcre2-8-0_10.36-2+deb11u1_amd64.deb"
    "af3c3562eb2802481a2b9558df1b389f3c6d9b1bf3b4219e000e05131372ebaf  a/attr/libattr1_2.4.48-6_amd64.deb"
    "aa18d721be8aea50fbdb32cd9a319cb18a3f111ea6ad17399aa4ba9324c8e26a  a/acl/libacl1_2.2.53-10_amd64.deb"
  ) ;;
  arm64) pkgs=(
    "d7c7af5d86f43a885069408a89788f67f248e8124c682bb73936f33874e0611b  b/bash/bash_5.1-2+deb11u1_arm64.deb"
    "58027c991756930a2abb2f87a829393d3fdbfb76f4eca9795ef38ea2b0510e27  n/ncurses/libtinfo6_6.2+20201114-2+deb11u2_arm64.deb"
    "6210c84d6ff84b867dc430f661f22f536e1704c27bdb79de38e26f75b853d9c0  c/coreutils/coreutils_8.32-4_arm64.deb"
    "da98279a47dabaa46a83514142f5c691c6a71fa7e582661a3a3db6887ad3e9d1  libs/libselinux/libselinux1_3.1-3_arm64.deb"
    "27a4362a4793cb67a8ae571bd8c3f7e8654dc2e56d99088391b87af1793cca9c  p/pcre2/libpcre2-8-0_10.36-2+deb11u1_arm64.deb"
    "cb9b59be719a6fdbaabaa60e22aa6158b2de7a68c88ccd7c3fb7f41a25fb43d0  a/attr/libattr1_2.4.48-6_arm64.deb"
    "f164c48192cb47746101de6c59afa3f97777c8fc821e5a30bb890df1f4cb4cfd  a/acl/libacl1_2.2.53-10_arm64.deb"
  ) ;;
  *) echo "debian-pkgs: unsupported arch '$arch' (want amd64|arm64)" >&2; exit 1 ;;
esac

mkdir -p "$rootfs"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

for entry in "${pkgs[@]}"; do
  sha="${entry%% *}"; path="${entry##* }"; file="$(basename "$path")"
  out="$tmp/$file"; ok=0

  for url in "$SNAP/$path" "$MIRROR/$file"; do
    if curl -fsSL "$url" -o "$out" 2>/dev/null && echo "$sha  $out" | sha256sum -c - >/dev/null 2>&1; then
      ok=1; break
    fi
  done
  
  if [ "$ok" -ne 1 ]; then echo "debian-pkgs: failed to fetch/verify $file" >&2; exit 1; fi
  dpkg-deb -x "$out" "$rootfs"
  echo "debian-pkgs: extracted $file" >&2
done
