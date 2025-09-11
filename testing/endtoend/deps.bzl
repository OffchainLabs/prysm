load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")  # gazelle:keep

lighthouse_version = "v7.0.0-beta.0"
lighthouse_archive_name = "lighthouse-%s-x86_64-unknown-linux-gnu.tar.gz" % lighthouse_version

def e2e_deps():
    http_archive(
        name = "web3signer",
        urls = ["https://github.com/Consensys/web3signer/releases/download/25.9.0/web3signer-25.9.0.tar.gz"],
        sha256 = "4bc95a86e232050ff071279043e1d04616572d551f6f72aee31108f96dc77bd8",
        build_file = "@prysm//testing/endtoend:web3signer.BUILD",
        strip_prefix = "web3signer-25.9.0",
    )

    http_archive(
        name = "lighthouse",
        integrity = "sha256-qMPifuh7u0epItu8DzZ8YdZ2fVZNW7WKnbmmAgjh/us=",
        build_file = "@prysm//testing/endtoend:lighthouse.BUILD",
        url = ("https://github.com/sigp/lighthouse/releases/download/%s/" + lighthouse_archive_name) % lighthouse_version,
    )
