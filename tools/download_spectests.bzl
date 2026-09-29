# bazel build @consensus_spec_tests//:test_data
# bazel build @consensus_spec_tests//:test_data --repo_env=CONSENSUS_SPEC_TESTS_VERSION=nightly
# bazel build @consensus_spec_tests//:test_data --repo_env=CONSENSUS_SPEC_TESTS_VERSION=nightly-<run_id>
# bazel build @consensus_spec_tests//:test_data --repo_env=CONSENSUS_SPEC_TESTS_DIR=/abs/path/to/tarballs
# bazel build @consensus_spec_tests//:test_data --repo_env=CONSENSUS_SPEC_TESTS_MAINNET=/abs/path/to/mainnet.tar.gz

def _get_redirected_url(repository_ctx, url, headers):
    if not repository_ctx.which("curl"):
        fail("curl is required to resolve redirect URLs")

    cmd = [
        "curl",
        "-sL",  # silent + follow redirects
        "-o",
        "NUL" if repository_ctx.os.name == "windows" else "/dev/null",
        "-w",
        "%{url_effective}",
        "-H",
        "Authorization: %s" % headers["Authorization"],
        "-H",
        "Accept: %s" % headers["Accept"],
        url,
    ]

    result = repository_ctx.execute(cmd, quiet = True)
    if result.return_code != 0:
        fail("curl failed to resolve redirected URL: %s" % result.stderr)
    return result.stdout.strip()

def _local_sources(repository_ctx):
    # Collect per-flavor spec test data to use from disk instead of downloading.
    # A source can be either a tarball (.tar.gz) or an already-unpacked directory
    # tree. Two opt-in mechanisms, which may be combined:
    #
    #   CONSENSUS_SPEC_TESTS_DIR=/abs/dir
    #       For each flavor (general/mainnet/minimal) look, in order, for:
    #         <dir>/<flavor>.tar.gz   - a release-style tarball
    #         <dir>/tests/<flavor>    - unpacked tree with the upstream tests/ prefix
    #         <dir>/<flavor>          - unpacked tree with the prefix stripped
    #       Only the flavors found are used; missing ones are skipped, so you can
    #       run a single preset by providing just e.g. mainnet.
    #
    #   CONSENSUS_SPEC_TESTS_<FLAVOR>=/abs/path
    #       Point a specific flavor at an arbitrarily-named tarball or at an
    #       unpacked flavor tree directly, e.g.
    #       CONSENSUS_SPEC_TESTS_MAINNET=/tmp/my-mainnet.tar.gz or
    #       CONSENSUS_SPEC_TESTS_MAINNET=/data/specs/mainnet. Overrides the
    #       directory entry for that flavor.
    #
    # Returns a dict of flavor -> struct(path, is_dir).
    sources = {}
    requested = False

    local_dir = repository_ctx.getenv("CONSENSUS_SPEC_TESTS_DIR") or ""
    if local_dir:
        requested = True
        if not local_dir.startswith("/"):
            fail("CONSENSUS_SPEC_TESTS_DIR must be an absolute path, got: %s" % local_dir)
        for flavor in repository_ctx.attr.flavors:
            tarball = repository_ctx.path("%s/%s.tar.gz" % (local_dir, flavor))
            nested = repository_ctx.path("%s/tests/%s" % (local_dir, flavor))
            flat = repository_ctx.path("%s/%s" % (local_dir, flavor))
            if tarball.exists:
                sources[flavor] = struct(path = tarball, is_dir = False)
            elif nested.exists and nested.is_dir:
                sources[flavor] = struct(path = nested, is_dir = True)
            elif flat.exists and flat.is_dir:
                sources[flavor] = struct(path = flat, is_dir = True)

    for flavor in repository_ctx.attr.flavors:
        env = "CONSENSUS_SPEC_TESTS_%s" % flavor.upper()
        override = repository_ctx.getenv(env) or ""
        if override:
            requested = True
            if not override.startswith("/"):
                fail("%s must be an absolute path, got: %s" % (env, override))
            src = repository_ctx.path(override)
            if not src.exists:
                fail("%s=%s does not exist" % (env, override))
            sources[flavor] = struct(path = src, is_dir = src.is_dir)

    if requested and not sources:
        fail("Local spec tests requested via CONSENSUS_SPEC_TESTS_DIR/CONSENSUS_SPEC_TESTS_<FLAVOR> " +
             "but no tarballs or unpacked flavor directories (general/mainnet/minimal) were found")

    return sources

def _install_local(repository_ctx, sources):
    # Normalize every local source to the tests/<flavor> layout that the
    # generated BUILD.bazel globs expect (the same layout the upstream release
    # tarballs extract to). Flavors not in `sources` are simply absent; their
    # filegroups resolve to empty globs, which is fine when running one preset.
    print("Using local spec tests:", ", ".join(sorted(sources.keys())))
    for flavor in sources:
        src = sources[flavor]
        dest = "tests/%s" % flavor
        if src.is_dir:
            # Symlink the unpacked flavor tree into place. glob() follows the
            # directory symlink when enumerating sources.
            repository_ctx.symlink(src.path, dest)
        else:
            # Keep the raw tarball available at <flavor>.tar.gz (consumed by
            # e.g. methodical-ssz gen-spectest), and extract into a staging dir
            # so we can normalize whatever internal prefix it uses.
            archive = "%s.tar.gz" % flavor
            repository_ctx.symlink(src.path, archive)
            stage = "_stage/%s" % flavor
            repository_ctx.extract(archive, output = stage)
            nested = repository_ctx.path("%s/tests/%s" % (stage, flavor))
            flat = repository_ctx.path("%s/%s" % (stage, flavor))
            if nested.exists:
                repository_ctx.symlink(nested, dest)
            elif flat.exists:
                repository_ctx.symlink(flat, dest)
            else:
                fail("Could not find flavor '%s' inside tarball %s" % (flavor, src.path))

def _impl(repository_ctx):
    version = repository_ctx.getenv("CONSENSUS_SPEC_TESTS_VERSION") or repository_ctx.attr.version
    token = repository_ctx.getenv("GITHUB_TOKEN") or ""
    local_sources = _local_sources(repository_ctx)

    if local_sources:
        _install_local(repository_ctx, local_sources)
    elif version == "nightly" or version.startswith("nightly-"):
        print("Downloading nightly tests")
        if not token:
            fail("Error GITHUB_TOKEN is not set")

        headers = {
            "Authorization": "token %s" % token,
            "Accept": "application/vnd.github+json",
        }

        if version.startswith("nightly-"):
            run_id = version.split("nightly-", 1)[1]
            if not run_id:
                fail("Error invalid run id")
        else:
            repository_ctx.download(
                "https://api.github.com/repos/%s/actions/workflows/%s/runs?branch=%s&status=success&per_page=1" %
                (repository_ctx.attr.repo, repository_ctx.attr.workflow, repository_ctx.attr.branch),
                headers = headers,
                output = "runs.json",
            )

            run_id = json.decode(repository_ctx.read("runs.json"))["workflow_runs"][0]["id"]
            repository_ctx.delete("runs.json")

        print("Run id:", run_id)
        repository_ctx.download(
            "https://api.github.com/repos/%s/actions/runs/%s/artifacts" %
            (repository_ctx.attr.repo, run_id),
            headers = headers,
            output = "artifacts.json",
        )

        artifacts = json.decode(repository_ctx.read("artifacts.json"))["artifacts"]
        repository_ctx.delete("artifacts.json")

        for artifact in artifacts:
            name = artifact["name"]
            if name == "consensustestgen.log":
                continue
            url = artifact["archive_download_url"]

            # Ugh this is the worst, bazel doesn't follow redirects...
            resolved_url = _get_redirected_url(repository_ctx, url, headers)
            repository_ctx.download_and_extract(resolved_url)
            tar_gz_file = "%s.tar.gz" % name.split(" ")[0].lower()
            repository_ctx.extract(tar_gz_file)
            repository_ctx.delete(tar_gz_file)
    else:
        for flavor in repository_ctx.attr.flavors:
            integrity = repository_ctx.attr.flavors[flavor]
            url = "%s/%s.tar.gz" % (repository_ctx.attr.release_url_template % version, flavor)

            # Download the raw archive (cached by `integrity` in Bazel's
            # content-addressable repository cache) and extract it in place,
            # keeping the tarball so it can also be consumed un-extracted (e.g.
            # methodical-ssz gen-spectest's --release-uri). One download serves
            # both the extracted fixtures and the raw tarball.
            archive = "%s.tar.gz" % flavor
            repository_ctx.download(url, output = archive, integrity = integrity)
            repository_ctx.extract(archive)

    repository_ctx.file("BUILD.bazel", """
# Raw per-flavor archives (general.tar.gz, mainnet.tar.gz, minimal.tar.gz),
# kept un-extracted alongside the extracted fixture filegroups. Consumed by
# rules that want the tarball directly, e.g. //tools:methodical.bzl's
# ssz_gen_spectest --release-uri.
exports_files(glob(["*.tar.gz"]))

filegroup(
    name = "general_tests",
    srcs = glob(["tests/general/**/*.yaml", "tests/general/**/*.ssz_snappy"]),
    visibility = ["//visibility:public"],
)

filegroup(
    name = "mainnet_tests",
    srcs = glob(["tests/mainnet/**/*.yaml", "tests/mainnet/**/*.ssz_snappy"]),
    visibility = ["//visibility:public"],
)

filegroup(
    name = "minimal_tests",
    srcs = glob(["tests/minimal/**/*.yaml", "tests/minimal/**/*.ssz_snappy"]),
    visibility = ["//visibility:public"],
)

filegroup(
    name = "test_data",
    srcs = [
        ":general_tests",
        ":mainnet_tests",
        ":minimal_tests",
    ],
    visibility = ["//visibility:public"],
)
""")

consensus_spec_tests = repository_rule(
    implementation = _impl,
    environ = [
        "CONSENSUS_SPEC_TESTS_VERSION",
        "GITHUB_TOKEN",
        "CONSENSUS_SPEC_TESTS_DIR",
        "CONSENSUS_SPEC_TESTS_GENERAL",
        "CONSENSUS_SPEC_TESTS_MAINNET",
        "CONSENSUS_SPEC_TESTS_MINIMAL",
    ],
    attrs = {
        "version": attr.string(mandatory = True),
        "flavors": attr.string_dict(mandatory = True),
        "repo": attr.string(default = "ethereum/consensus-specs"),
        "workflow": attr.string(default = "nightly-reftests.yml"),
        "branch": attr.string(default = "master"),
        "release_url_template": attr.string(default = "https://github.com/ethereum/consensus-specs/releases/download/%s"),
    },
)
