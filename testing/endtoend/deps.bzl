load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")  # gazelle:keep

lighthouse_version = "v4.6.0-rc.0"
lighthouse_archive_name = "lighthouse-%s-x86_64-unknown-linux-gnu-portable.tar.gz" % lighthouse_version

def e2e_deps():
    http_archive(
        name = "web3signer",
        urls = ["https://artifacts.consensys.net/public/web3signer/raw/names/web3signer.tar.gz/versions/24.12.0/web3signer-24.12.0.tar.gz"],
        sha256 = "5d2eff119e065a50bd2bd727e098963d0e61a3f6525bdc12b11515d3677a84d1",
        build_file = "@prysm//testing/endtoend:web3signer.BUILD",
        strip_prefix = "web3signer-24.12.0",
    )

    http_archive(
        name = "lighthouse",
        integrity = "sha256-9jmQN1AJUyogscUYibFchZMUXH0ZRKofW4oPhAFVRAE=",
        build_file = "@prysm//testing/endtoend:lighthouse.BUILD",
        url = ("https://github.com/sigp/lighthouse/releases/download/%s/" + lighthouse_archive_name) % lighthouse_version,
    )
