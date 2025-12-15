load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")  # gazelle:keep

lighthouse_version = "v7.0.0-beta.0"
lighthouse_archive_name = "lighthouse-%s-x86_64-unknown-linux-gnu.tar.gz" % lighthouse_version

def e2e_deps():
    http_archive(
        name = "web3signer",
        urls = ["https://github.com/Consensys/web3signer/releases/download/25.9.1/web3signer-25.9.1.tar.gz"],
        sha256 = "d84498abbe46fcf10ca44f930eafcd80d7339cbf3f7f7f42a77eb1763ab209cf",
        build_file = "@prysm//testing/endtoend:web3signer.BUILD",
        strip_prefix = "web3signer-25.9.1",
    )

    http_archive(
        name = "lighthouse",
        integrity = "sha256-qMPifuh7u0epItu8DzZ8YdZ2fVZNW7WKnbmmAgjh/us=",
        build_file = "@prysm//testing/endtoend:lighthouse.BUILD",
        url = ("https://github.com/sigp/lighthouse/releases/download/%s/" + lighthouse_archive_name) % lighthouse_version,
    )
