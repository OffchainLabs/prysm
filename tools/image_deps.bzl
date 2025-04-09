load("@distroless//private/remote:debian_archive.bzl", "debian_archive")

def prysm_image_deps():
    """
    These dependencies are pulled from https://debian.pkgs.org and support Debian 11.
    """
    debian_archive(
        name = "amd64_debian11_bash",
        package_name = "bash",
        sha256 = "f702ef058e762d7208a9c83f6f6bbf02645533bfd615c54e8cdcce842cd57377",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/b/bash/bash_5.1-2+deb11u1_amd64.deb",
            "https://prysmaticlabs.com/uploads/bash_5.1-2+deb11u1_amd64.deb",
        ],
    )

    debian_archive(
        name = "arm64_debian11_bash",
        package_name = "bash",
        sha256 = "d7c7af5d86f43a885069408a89788f67f248e8124c682bb73936f33874e0611b",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/b/bash/bash_5.1-2+deb11u1_arm64.deb",
            "https://prysmaticlabs.com/uploads/bash_5.1-2+deb11u1_arm64.deb",
        ],
    )

    debian_archive(
        name = "amd64_debian11_libtinfo6",
        package_name = "libtinfo6", # Required by: bash
        sha256 = "96ed58b8fd656521e08549c763cd18da6cff1b7801a3a22f29678701a95d7e7b",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/n/ncurses/libtinfo6_6.2+20201114-2+deb11u2_amd64.deb",
            "https://prysmaticlabs.com/uploads/libtinfo6_6.2+20201114-2+deb11u2_amd64.deb",
        ],
    )

    debian_archive(
        name = "arm64_debian11_libtinfo6",
        package_name = "libtinfo6", # Required by: bash
        sha256 = "58027c991756930a2abb2f87a829393d3fdbfb76f4eca9795ef38ea2b0510e27",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/n/ncurses/libtinfo6_6.2+20201114-2+deb11u1_arm64.deb",
            "https://prysmaticlabs.com/uploads/libtinfo6_6.2+20201114-2+deb11u2_arm64.deb",
        ],
    )

    debian_archive(
        name = "amd64_debian11_coreutils",
        package_name = "coreutils",
        sha256 = "3558a412ab51eee4b60641327cb145bb91415f127769823b68f9335585b308d4",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/c/coreutils/coreutils_8.32-4+b1_amd64.deb",
            "https://prysmaticlabs.com/uploads/coreutils_8.32-4+b1_amd64.deb",
        ],
    )

    debian_archive(
        name = "arm64_debian11_coreutils",
        package_name = "coreutils",
        sha256 = "6210c84d6ff84b867dc430f661f22f536e1704c27bdb79de38e26f75b853d9c0",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/c/coreutils/coreutils_8.32-4_arm64.deb",
            "https://prysmaticlabs.com/uploads/coreutils_8.32-4_arm64.deb",
        ],
    )

    debian_archive(
        name = "amd64_debian11_libselinux",
        package_name = "libselinux", # Required by: coreutils
        sha256 = "339f5ede10500c16dd7192d73169c31c4b27ab12130347275f23044ec8c7d897",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/libs/libselinux/libselinux1_3.1-3_amd64.deb",
            "https://prysmaticlabs.com/uploads/libselinux1_3.1-3_amd64.deb",
        ],
    )

    debian_archive(
        name = "arm64_debian11_libselinux",
        package_name = "libselinux", # Required by: coreutils
        sha256 = "da98279a47dabaa46a83514142f5c691c6a71fa7e582661a3a3db6887ad3e9d1",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/libs/libselinux/libselinux1_3.1-3_arm64.deb",
            "https://prysmaticlabs.com/uploads/libselinux1_3.1-3_arm64.deb",
        ],
    )

    debian_archive(
        name = "amd64_debian11_libpcre2",
        package_name = "libpcre2", # Required by: coreutils
        sha256 = "ee192c8d22624eb9d0a2ae95056bad7fb371e5abc17e23e16b1de3ddb17a1064",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/p/pcre2/libpcre2-8-0_10.36-2+deb11u1_amd64.deb",
            "https://prysmaticlabs.com/uploads/libpcre2-8-0_10.36-2+deb11u1_amd64.deb",
        ],
    )

    debian_archive(
        name = "arm64_debian11_libpcre2",
        package_name = "libpcre2", # Required by: coreutils
        sha256 = "27a4362a4793cb67a8ae571bd8c3f7e8654dc2e56d99088391b87af1793cca9c",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/p/pcre2/libpcre2-8-0_10.36-2+deb11u1_arm64.deb",
            "https://prysmaticlabs.com/uploads/libpcre2-8-0_10.36-2+deb11u1_arm64.deb",
        ],
    )

    debian_archive(
        name = "amd64_debian11_libattr1",
        package_name = "libattr1", # Required by: coreutils
        sha256 = "af3c3562eb2802481a2b9558df1b389f3c6d9b1bf3b4219e000e05131372ebaf",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/a/attr/libattr1_2.4.48-6_amd64.deb",
            "https://prysmaticlabs.com/uploads/libattr1_2.4.48-6_amd64.deb",
        ],
    )

    debian_archive(
        name = "arm64_debian11_libattr1",
        package_name = "libattr1", # Required by: coreutils
        sha256 = "cb9b59be719a6fdbaabaa60e22aa6158b2de7a68c88ccd7c3fb7f41a25fb43d0",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/a/attr/libattr1_2.4.48-6_arm64.deb",
            "https://prysmaticlabs.com/uploads/libattr1_2.4.48-6_arm64.deb",
        ],
    )

    debian_archive(
        name = "amd64_debian11_libacl1",
        package_name = "libacl1", # Required by: coreutils
        sha256 = "aa18d721be8aea50fbdb32cd9a319cb18a3f111ea6ad17399aa4ba9324c8e26a",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/a/acl/libacl1_2.2.53-10_amd64.deb",
            "https://prysmaticlabs.com/uploads/libacl1_2.2.53-10_amd64.deb",
        ],
    )

    debian_archive(
        name = "arm64_debian11_libacl1",
        package_name = "libacl1", # Required by: coreutils
        sha256 = "f164c48192cb47746101de6c59afa3f97777c8fc821e5a30bb890df1f4cb4cfd",
        urls = [
            "https://snapshot.debian.org/archive/debian/20231214T085654Z/pool/main/a/acl/libacl1_2.2.53-10_arm64.deb",
            "https://prysmaticlabs.com/uploads/libacl1_2.2.53-10_arm64.deb",
        ],
    )
