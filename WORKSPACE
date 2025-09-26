workspace(name = "prysm")

load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "rules_pkg",
    sha256 = "8c20f74bca25d2d442b327ae26768c02cf3c99e93fad0381f32be9aab1967675",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_pkg/releases/download/0.8.1/rules_pkg-0.8.1.tar.gz",
        "https://github.com/bazelbuild/rules_pkg/releases/download/0.8.1/rules_pkg-0.8.1.tar.gz",
    ],
)

load("@rules_pkg//:deps.bzl", "rules_pkg_dependencies")

rules_pkg_dependencies()

http_archive(
    name = "toolchains_protoc",
    sha256 = "abb1540f8a9e045422730670ebb2f25b41fa56ca5a7cf795175a110a0a68f4ad",
    strip_prefix = "toolchains_protoc-0.3.6",
    url = "https://github.com/aspect-build/toolchains_protoc/releases/download/v0.3.6/toolchains_protoc-v0.3.6.tar.gz",
)

load("@toolchains_protoc//protoc:repositories.bzl", "rules_protoc_dependencies")

rules_protoc_dependencies()

load("@rules_proto//proto:repositories.bzl", "rules_proto_dependencies")

rules_proto_dependencies()

load("@bazel_features//:deps.bzl", "bazel_features_deps")

bazel_features_deps()

load("@toolchains_protoc//protoc:toolchain.bzl", "protoc_toolchains")

protoc_toolchains(
    name = "protoc_toolchains",
    version = "v25.3",
)

HERMETIC_CC_TOOLCHAIN_VERSION = "v3.0.1"

http_archive(
    name = "hermetic_cc_toolchain",
    sha256 = "3bc6ec127622fdceb4129cb06b6f7ab098c4d539124dde96a6318e7c32a53f7a",
    urls = [
        "https://mirror.bazel.build/github.com/uber/hermetic_cc_toolchain/releases/download/{0}/hermetic_cc_toolchain-{0}.tar.gz".format(HERMETIC_CC_TOOLCHAIN_VERSION),
        "https://github.com/uber/hermetic_cc_toolchain/releases/download/{0}/hermetic_cc_toolchain-{0}.tar.gz".format(HERMETIC_CC_TOOLCHAIN_VERSION),
    ],
)

load("@hermetic_cc_toolchain//toolchain:defs.bzl", zig_toolchains = "toolchains")

zig_toolchains()

# Register zig sdk toolchains with support for Ubuntu 20.04 (Focal Fossa) which has an EOL date of April, 2025.
# For ubuntu glibc support, see https://launchpad.net/ubuntu/+source/glibc
register_toolchains(
    "@zig_sdk//toolchain:linux_amd64_gnu.2.31",
    "@zig_sdk//toolchain:linux_arm64_gnu.2.31",
    # Hermetic cc toolchain is not yet supported on darwin. Sysroot needs to be provided.
    # See https://github.com/uber/hermetic_cc_toolchain#osx-sysroot
    #    "@zig_sdk//toolchain:darwin_amd64",
    #    "@zig_sdk//toolchain:darwin_arm64",
    # Windows builds are not supported yet.
    #    "@zig_sdk//toolchain:windows_amd64",
)

load("@prysm//tools/cross-toolchain:darwin_cc_hack.bzl", "configure_nonhermetic_darwin")

configure_nonhermetic_darwin()

load("@prysm//tools/cross-toolchain:prysm_toolchains.bzl", "configure_prysm_toolchains")

configure_prysm_toolchains()

http_archive(
    name = "bazel_skylib",
    sha256 = "1c531376ac7e5a180e0237938a2536de0c54d93f5c278634818e0efc952dd56c",
    urls = [
        "https://github.com/bazelbuild/bazel-skylib/releases/download/1.0.3/bazel-skylib-1.0.3.tar.gz",
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-skylib/releases/download/1.0.3/bazel-skylib-1.0.3.tar.gz",
    ],
)

load("@bazel_skylib//:workspace.bzl", "bazel_skylib_workspace")

bazel_skylib_workspace()

http_archive(
    name = "bazel_gazelle",
    integrity = "sha256-MpOL2hbmcABjA1R5Bj2dJMYO2o15/Uc5Vj9Q0zHLMgk=",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.35.0/bazel-gazelle-v0.35.0.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.35.0/bazel-gazelle-v0.35.0.tar.gz",
    ],
)

http_archive(
    name = "com_github_atlassian_bazel_tools",
    sha256 = "60821f298a7399450b51b9020394904bbad477c18718d2ad6c789f231e5b8b45",
    strip_prefix = "bazel-tools-a2138311856f55add11cd7009a5abc8d4fd6f163",
    urls = ["https://github.com/atlassian/bazel-tools/archive/a2138311856f55add11cd7009a5abc8d4fd6f163.tar.gz"],
)

http_archive(
    name = "rules_distroless",
    sha256 = "e64f06e452cd153aeab81f752ccf4642955b3af319e64f7bc7a7c9252f76b10e",
    strip_prefix = "rules_distroless-f5e678217b57ce3ad2f1c0204bd4e9d416255773",
    url = "https://github.com/GoogleContainerTools/rules_distroless/archive/f5e678217b57ce3ad2f1c0204bd4e9d416255773.tar.gz",
)

load("@rules_distroless//distroless:dependencies.bzl", "rules_distroless_dependencies")

rules_distroless_dependencies()

http_archive(
    name = "distroless",
    integrity = "sha256-Cf00kUp1NyXA3LzbdyYy4Kda27wbkB8+A9MliTxq4jE=",
    strip_prefix = "distroless-9dc924b9fe812eec2fa0061824dcad39eb09d0d6",
    url = "https://github.com/GoogleContainerTools/distroless/archive/9dc924b9fe812eec2fa0061824dcad39eb09d0d6.tar.gz",  # 2024-01-24
)

http_archive(
    name = "aspect_bazel_lib",
    sha256 = "a272d79bb0ac6b6965aa199b1f84333413452e87f043b53eca7f347a23a478e8",
    strip_prefix = "bazel-lib-2.9.3",
    url = "https://github.com/bazel-contrib/bazel-lib/releases/download/v2.9.3/bazel-lib-v2.9.3.tar.gz",
)

load("@aspect_bazel_lib//lib:repositories.bzl", "aspect_bazel_lib_dependencies", "aspect_bazel_lib_register_toolchains")

aspect_bazel_lib_dependencies()

aspect_bazel_lib_register_toolchains()

http_archive(
    name = "rules_oci",
    sha256 = "4a276e9566c03491649eef63f27c2816cc222f41ccdebd97d2c5159e84917c3b",
    strip_prefix = "rules_oci-1.7.4",
    url = "https://github.com/bazel-contrib/rules_oci/releases/download/v1.7.4/rules_oci-v1.7.4.tar.gz",
)

load("@rules_oci//oci:dependencies.bzl", "rules_oci_dependencies")

rules_oci_dependencies()

load("@rules_oci//oci:repositories.bzl", "LATEST_CRANE_VERSION", "oci_register_toolchains")

oci_register_toolchains(
    name = "oci",
    crane_version = LATEST_CRANE_VERSION,
)

http_archive(
    name = "io_bazel_rules_go",
    patch_args = ["-p1"],
    patches = [
        # Expose internals of go_test for custom build transitions.
        "//third_party:io_bazel_rules_go_test.patch",
    ],
    sha256 = "a729c8ed2447c90fe140077689079ca0acfb7580ec41637f312d650ce9d93d96",
    urls = [
        "https://mirror.bazel.build/github.com/bazel-contrib/rules_go/releases/download/v0.57.0/rules_go-v0.57.0.zip",
        "https://github.com/bazel-contrib/rules_go/releases/download/v0.57.0/rules_go-v0.57.0.zip",
    ],
)

# Override default import in rules_go with special patch until
# https://github.com/gogo/protobuf/pull/582 is merged.
git_repository(
    name = "com_github_gogo_protobuf",
    commit = "b03c65ea87cdc3521ede29f62fe3ce239267c1bc",
    patch_args = ["-p1"],
    patches = [
        "@io_bazel_rules_go//third_party:com_github_gogo_protobuf-gazelle.patch",
        "//third_party:com_github_gogo_protobuf-equal.patch",
    ],
    remote = "https://github.com/gogo/protobuf",
    shallow_since = "1610265707 +0000",
    # gazelle args: -go_prefix github.com/gogo/protobuf -proto legacy
)

load("@rules_oci//oci:pull.bzl", "oci_pull")

# A multi-arch base image
oci_pull(
    name = "linux_debian11_multiarch_base",  # Debian bullseye
    digest = "sha256:55a5e011b2c4246b4c51e01fcc2b452d151e03df052e357465f0392fcd59fddf",
    image = "gcr.io/prysmaticlabs/distroless/cc-debian11",
    platforms = [
        "linux/amd64",
        "linux/arm64/v8",
    ],
    reproducible = True,
)

load("@prysm//tools:image_deps.bzl", "prysm_image_deps")

prysm_image_deps()

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains(
    go_version = "1.25.1",
    nogo = "@//:nogo",
)

load("//:distroless_deps.bzl", "distroless_deps")

distroless_deps()

http_archive(
    name = "io_kubernetes_build",
    sha256 = "b84fbd1173acee9d02a7d3698ad269fdf4f7aa081e9cecd40e012ad0ad8cfa2a",
    strip_prefix = "repo-infra-6537f2101fb432b679f3d103ee729dd8ac5d30a0",
    url = "https://github.com/kubernetes/repo-infra/archive/6537f2101fb432b679f3d103ee729dd8ac5d30a0.tar.gz",
)

http_archive(
    name = "eip3076_spec_tests",
    build_file_content = """
filegroup(
    name = "test_data",
    srcs = glob([
        "**/*.json",
    ]),
    visibility = ["//visibility:public"],
)
    """,
    sha256 = "516d551cfb3e50e4ac2f42db0992f4ceb573a7cb1616d727a725c8161485329f",
    url = "https://github.com/eth-clients/slashing-protection-interchange-tests/archive/refs/tags/v5.3.0.tar.gz",
)

http_archive(
    name = "eip4881_spec_tests",
    build_file_content = """
filegroup(
    name = "test_data",
    srcs = glob([
        "**/*.yaml",
    ]),
    visibility = ["//visibility:public"],
)
    """,
    sha256 = "89cb659498c0d196fc9f957f8b849b2e1a5c041c3b2b3ae5432ac5c26944297e",
    url = "https://github.com/ethereum/EIPs/archive/5480440fe51742ed23342b68cf106cefd427e39d.tar.gz",
)

consensus_spec_version = "v1.6.0-beta.0"

load("@prysm//tools:download_spectests.bzl", "consensus_spec_tests")

consensus_spec_tests(
    name = "consensus_spec_tests",
    flavors = {
        "general": "sha256-rT3jQp2+ZaDiO66gIQggetzqr+kGeexaLqEhbx4HDMY=",
        "minimal": "sha256-wowwwyvd0KJLsE+oDOtPkrhZyJndJpJ0lbXYsLH6XBw=",
        "mainnet": "sha256-4ZLrLNeO7NihZ4TuWH5V5fUhvW9Y3mAPBQDCqrfShps=",
    },
    version = consensus_spec_version,
)

http_archive(
    name = "consensus_spec",
    build_file_content = """
filegroup(
    name = "spec_data",
    srcs = glob([
        "**/*.yaml",
    ]),
    visibility = ["//visibility:public"],
)
    """,
    integrity = "sha256-sBe3Rx8zGq9IrvfgIhZQpYidGjy3mE1SiCb6/+pjLdY=",
    strip_prefix = "consensus-specs-" + consensus_spec_version[1:],
    url = "https://github.com/ethereum/consensus-specs/archive/refs/tags/%s.tar.gz" % consensus_spec_version,
)

bls_test_version = "v0.1.1"

http_archive(
    name = "bls_spec_tests",
    build_file_content = """
filegroup(
    name = "test_data",
    srcs = glob([
        "**/*.yaml",
    ]),
    visibility = ["//visibility:public"],
)
    """,
    sha256 = "93c7d006e7c5b882cbd11dc9ec6c5d0e07f4a8c6b27a32f964eb17cf2db9763a",
    url = "https://github.com/ethereum/bls12-381-tests/releases/download/%s/bls_tests_yaml.tar.gz" % bls_test_version,
)

http_archive(
    name = "holesky_testnet",
    build_file_content = """
filegroup(
    name = "configs",
    srcs = [
        "metadata/config.yaml",
    ],
    visibility = ["//visibility:public"],
)
""",
    integrity = "sha256-htyxg8Ln2o8eCiifFN7/hcHGZg8Ir9CPzCEx+FUnnCs=",
    strip_prefix = "holesky-8aec65f11f0c986d6b76b2eb902420635eb9b815",
    url = "https://github.com/eth-clients/holesky/archive/8aec65f11f0c986d6b76b2eb902420635eb9b815.tar.gz",
)

http_archive(
    name = "mainnet",
    build_file_content = """
filegroup(
    name = "configs",
    srcs = [
        "metadata/config.yaml",
    ],
    visibility = ["//visibility:public"],
)
""",
    integrity = "sha256-NZr/gsQK9rBHRnznlPBiNzJpK8MPMrfUa3f+QYqn1+g=",
    strip_prefix = "mainnet-978f1794eada6f85bee76e4d2d5959a5fb8e0cc5",
    url = "https://github.com/eth-clients/mainnet/archive/978f1794eada6f85bee76e4d2d5959a5fb8e0cc5.tar.gz",
)

http_archive(
    name = "sepolia_testnet",
    build_file_content = """
filegroup(
    name = "configs",
    srcs = [
        "metadata/config.yaml",
    ],
    visibility = ["//visibility:public"],
)
""",
    integrity = "sha256-+UZgfvBcea0K0sbvAJZOz5ZNmxdWZYbohP38heUuc6w=",
    strip_prefix = "sepolia-f9158732adb1a2a6440613ad2232eb50e7384c4f",
    url = "https://github.com/eth-clients/sepolia/archive/f9158732adb1a2a6440613ad2232eb50e7384c4f.tar.gz",
)

http_archive(
    name = "hoodi_testnet",
    build_file_content = """
filegroup(
    name = "configs",
    srcs = [
        "metadata/config.yaml",
    ],
    visibility = ["//visibility:public"],
)
""",
    integrity = "sha256-G+4c9c/vci1OyPrQJnQCI+ZCv/E0cWN4hrHDY3i7ns0=",
    strip_prefix = "hoodi-b6ee51b2045a5e7fe3efac52534f75b080b049c6",
    url = "https://github.com/eth-clients/hoodi/archive/b6ee51b2045a5e7fe3efac52534f75b080b049c6.tar.gz",
)

http_archive(
    name = "com_google_protobuf",
    sha256 = "7c3ebd7aaedd86fa5dc479a0fda803f602caaf78d8aff7ce83b89e1b8ae7442a",
    strip_prefix = "protobuf-28.3",
    urls = [
        "https://github.com/protocolbuffers/protobuf/archive/v28.3.tar.gz",
    ],
)

# External dependencies
http_archive(
    name = "googleapis",
    sha256 = "9d1a930e767c93c825398b8f8692eca3fe353b9aaadedfbcf1fca2282c85df88",
    strip_prefix = "googleapis-64926d52febbf298cb82a8f472ade4a3969ba922",
    urls = [
        "https://github.com/googleapis/googleapis/archive/64926d52febbf298cb82a8f472ade4a3969ba922.zip",
    ],
)

load("@googleapis//:repository_rules.bzl", "switched_rules_by_language")

switched_rules_by_language(
    name = "com_google_googleapis_imports",
    go = True,
)

load("//:deps.bzl", "prysm_deps")

# gazelle:repository_macro deps.bzl%prysm_deps
prysm_deps()

load("@prysm//third_party/herumi:herumi.bzl", "bls_dependencies")

bls_dependencies()

load("@prysm//testing/endtoend:deps.bzl", "e2e_deps")

e2e_deps()

load("@com_github_atlassian_bazel_tools//gometalinter:deps.bzl", "gometalinter_dependencies")

gometalinter_dependencies()

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")

gazelle_dependencies(go_sdk = "go_sdk")

load("@com_google_protobuf//:protobuf_deps.bzl", "protobuf_deps")

protobuf_deps()

# Do NOT add new go dependencies here! Refer to DEPENDENCIES.md!
