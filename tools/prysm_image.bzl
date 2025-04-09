load("@rules_oci//oci:defs.bzl", "oci_image", "oci_image_index", "oci_push", "oci_tarball")
load("@rules_pkg//:pkg.bzl", "pkg_tar")
load("//tools:multi_arch.bzl", "multi_arch")

def prysm_image_upload(
        name,
        binary,
        entrypoint,
        symlinks,
        repository,
        tags):
    pkg_tar(
        name = "binary_tar",
        srcs = [binary],
        symlinks = symlinks,
        tags = tags,
        extension = "tar.gz",
    )

    oci_image(
        name = "oci_image",
        base = "@linux_debian11_multiarch_base",
        entrypoint = entrypoint,
        tars = [
            "//tools:passwd_tar",
        ] + select({
          "@platforms//cpu:x86_64": [
            "@amd64_debian11_bash",
            "@amd64_debian11_libtinfo6",
            "@amd64_debian11_coreutils",
            "@amd64_debian11_libacl1",
            "@amd64_debian11_libattr1",
            "@amd64_debian11_libselinux",
            "@amd64_debian11_libpcre2",
          ],
          "@platforms//cpu:arm64": [
            "@arm64_debian11_bash",
            "@arm64_debian11_libtinfo6",
            "@arm64_debian11_coreutils",
            "@arm64_debian11_libacl1",
            "@arm64_debian11_libattr1",
            "@arm64_debian11_libselinux",
            "@arm64_debian11_libpcre2",
          ],
        }) + [
            ":binary_tar",
        ],
        labels = {
          "org.opencontainers.image.source": "https://github.com/prysmaticlabs/prysm",
        },
        tags = tags,
    )

    multi_arch(
        name = "oci_multiarch",
        image = ":oci_image",
        platforms = [
            "@io_bazel_rules_go//go/toolchain:linux_amd64_cgo",
            "@io_bazel_rules_go//go/toolchain:linux_arm64_cgo",
        ],
        tags = tags,
    )

    oci_image_index(
        name = "oci_image_index",
        images = [
            ":oci_multiarch",
        ],
        tags = tags,
    )

    oci_push(
        name = name,
        image = ":oci_image_index",
        repository = repository,
        tags = tags,
    )

    oci_tarball(
        name = "oci_image_tarball",
        image = ":oci_image",
        repo_tags = [repository+":latest"],
    )
