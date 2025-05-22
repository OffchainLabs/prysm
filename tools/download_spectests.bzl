# bazel build @consensus_spec_tests//:test_data
# bazel build @consensus_spec_tests//:test_data --repo_env=CONSENSUS_SPEC_TESTS_VERSION=nightly

def _get_redirected_url(repository_ctx, url, headers):
    if not repository_ctx.which("curl"):
        fail("curl is required to resolve redirect URLs")

    cmd = [
        "curl",
        "-sL",  # silent + follow redirects
        "-o", "NUL" if repository_ctx.os.name == "windows" else "/dev/null",
        "-w", "%{url_effective}",
        "-H", "Authorization: %s" % headers["Authorization"],
        "-H", "Accept: %s" % headers["Accept"],
        url,
    ]

    result = repository_ctx.execute(cmd, quiet = True)
    if result.return_code != 0:
        fail("curl failed to resolve redirected URL: %s" % result.stderr)
    return result.stdout.strip()

def _impl(repository_ctx):
    version = repository_ctx.getenv("CONSENSUS_SPEC_TESTS_VERSION") or repository_ctx.attr.version
    token = repository_ctx.getenv("GITHUB_TOKEN") or ""

    if version == "nightly":
        print("Downloading nightly tests")
        if not token:
            fail("Error GITHUB_TOKEN is not set")

        headers = {
            "Authorization": "token %s" % token,
            "Accept": "application/vnd.github+json",
        }

        repository_ctx.download(
            "https://api.github.com/repos/%s/actions/workflows/%s/runs?branch=%s&status=success&per_page=1"
                % (repository_ctx.attr.repo, repository_ctx.attr.workflow, repository_ctx.attr.branch),
            headers = headers,
            output = "runs.json"
        )

        run_id = json.decode(repository_ctx.read("runs.json"))["workflow_runs"][0]["id"]
        repository_ctx.delete("runs.json")

        repository_ctx.download(
            "https://api.github.com/repos/%s/actions/runs/%s/artifacts"
                % (repository_ctx.attr.repo, run_id),
            headers = headers,
            output = "artifacts.json"
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
            repository_ctx.download_and_extract(url, integrity = integrity)

    repository_ctx.file("BUILD.bazel", """
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
    environ = ["CONSENSUS_SPEC_TESTS_VERSION", "GITHUB_TOKEN"],
    attrs = {
        "version": attr.string(mandatory = True),
        "flavors": attr.string_dict(mandatory = True),
        "repo": attr.string(default = "ethereum/consensus-specs"),
        "workflow": attr.string(default = "generate_vectors.yml"),
        "branch": attr.string(default = "dev"),
        "release_url_template": attr.string(default = "https://github.com/ethereum/consensus-spec-tests/releases/download/%s"),
    },
)
