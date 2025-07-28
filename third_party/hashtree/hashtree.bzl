load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

"""
 OffchainLabs hashtree library for fast merkle tree hashing.
Uses native Go bindings with syso files, no CGO overhead.
"""

def hashtree_dependencies():
    _maybe(
        http_archive,
        name = "offchainlabs_hashtree",
        strip_prefix = "hashtree-main",
        urls = [
            "https://github.com/OffchainLabs/hashtree/archive/main.tar.gz",
        ],
        build_file = "@prysm//third_party/hashtree:hashtree_source.BUILD",
    )

def _maybe(repo_rule, name, **kwargs):
    if name not in native.existing_rules():
        repo_rule(name = name, **kwargs)