load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")  # gazelle:keep

lighthouse_version = "v8.2.0"

# sha256 (as Bazel "integrity" strings) of each lighthouse release asset the e2e harness
# supports, keyed by the release target-triple suffix. Bazel builds on linux/amd64 and uses
# the x86_64-unknown-linux-gnu entry; the Make e2e harness (build/externaldata) reads this
# map to fetch the binary matching the host platform. Keep in sync with lighthouse_version.
#
# To bump lighthouse, let `hack/update-lighthouse.sh [version]` rewrite both
# lighthouse_version and this map (defaults to the latest upstream release).
lighthouse_integrity = {
    "x86_64-unknown-linux-gnu": "sha256-w3IY/v+yoiVYtfDFGPzxS+i8PVUTMWLH014+UZNFFS0=",
    "aarch64-unknown-linux-gnu": "sha256-5ArnsUOk17YedRx8BWQZ2v4uOAcRkxkzmzRW1GYa2Hs=",
    "aarch64-apple-darwin": "sha256-LVwNzgKtjR4IdiCQJlVQ3+LLf7OmaB7XE/FCmbR+0BM=",
}

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
        integrity = lighthouse_integrity["x86_64-unknown-linux-gnu"],
        build_file = "@prysm//testing/endtoend:lighthouse.BUILD",
        url = ("https://github.com/sigp/lighthouse/releases/download/%s/" + lighthouse_archive_name) % lighthouse_version,
    )
