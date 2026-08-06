load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@io_bazel_rules_go//go:def.bzl", "GoLibrary", "GoSource", "go_context", "new_go_info")
load("@io_bazel_rules_go//go/tools/gopackagesdriver:aspect.bzl", "GoPkgInfo", "go_pkg_info_aspect")

_METHODICAL_TOOL = Label("//tools/genception:methodicalgen")
_GENCEPTION_TOOL = Label("//tools/genception/cmd:cmd")
_SSZ_RUNTIME = Label("@com_github_offchainlabs_methodical_ssz//ssz:go_default_library")

def _ssz_methodical_impl(ctx):
    go_ctx = go_context(ctx)
    all_json_files = {}
    stdlib = ""
    inputs = [ctx.file.config_file]
    ssz_lib = ctx.attr.ssz_lib[GoLibrary]
    generated_srcs = getattr(ssz_lib, "srcs", [])
    ssz_sources = new_go_info(
        go_ctx,
        ctx.attr,
        name = ssz_lib.name,
        importpath = ssz_lib.importpath,
        coverage_instrumented = ctx.coverage_instrumented(),
        generated_srcs = generated_srcs,
        pathtype = ssz_lib.pathtype,
        verify_resolver_deps = True,
    )
    inputs += ssz_sources.srcs
    for dep in ctx.attr.deps + [ctx.attr.ssz_lib]:
        pkginfo = dep[OutputGroupInfo]
        if hasattr(pkginfo, "go_generated_srcs"):
            inputs += pkginfo.go_generated_srcs.to_list()

        # collect all the paths to json files dict keys for uniqueness
        json_files = pkginfo.go_pkg_driver_json_file.to_list()
        inputs += json_files
        if len(json_files) > 0:
            for jf in json_files:
                # presumably path is full path from exec root
                all_json_files[jf.path] = ""
        inputs += pkginfo.go_pkg_driver_srcs.to_list()
        inputs += pkginfo.go_pkg_driver_export_file.to_list()

        # we just ned to get the stdlib once
        if stdlib == "":
            std_ds = pkginfo.go_pkg_driver_stdlib_json_file.to_list()
            if len(std_ds) > 0:
                stdlib = std_ds[0].path
                inputs += std_ds

    # concat the stdlib with all the other json file paths and write to disk
    json_out = [stdlib] + all_json_files.keys()
    all_pkg_list = ctx.actions.declare_file("methodical-pkg-list.json")
    ctx.actions.write(all_pkg_list, content = json.encode(json_out))
    out_base = ctx.outputs.out.root.path

    # The //proto:network build setting (mainnet|minimal) -- the same flag the
    # ssz_proto_files select() keys on -- determines which .pb.go sizes the
    # go_proto dep was built with, and therefore which //go:build constraint
    # methodical must stamp so the output matches the mainnet/minimal pair that
    # //build/gen writes. methodical emits no header on its own; both codegen
    # paths drive it through --go-build-constraint.
    go_build_constraint = "!minimal"
    if ctx.attr._network[BuildSettingInfo].value == "minimal":
        go_build_constraint = "minimal"

    args = [
        "gen",
        "--config=" + ctx.file.config_file.path,
        "--output=" + ctx.outputs.out.path,
        "--go-build-constraint=" + go_build_constraint,
    ]
    if ctx.attr.override_package_name != "":
        args.append("--override-package-name=" + ctx.attr.override_package_name)
    if ctx.attr._disable_progressive[BuildSettingInfo].value:
        args.append("--disable-progressive")

    codegen_bins = [ctx.file.genception, ctx.file.methodical_tool]
    ctx.actions.run_shell(
        env = {
            "PACKAGE_JSON_INVENTORY": all_pkg_list.path,
            "PACKAGES_BASE": out_base,
            "GOCACHE": "./.gocache",
            "GOPACKAGESDRIVER": ctx.file.genception.path,
            "GOPACKAGESDRIVER_LOG_PATH": out_base + "/gopackagesdriver.log",
        },
        inputs = [all_pkg_list] + inputs + codegen_bins,
        outputs = [ctx.outputs.out],
        command = """
        {cmd} {args}
        """.format(
            out_base = out_base,
            json_list = all_pkg_list.path,
            cmd = "$(pwd)/" + ctx.file.methodical_tool.path,
            args = " ".join(args),
            out = ctx.outputs.out.path,
        ),
    )

ssz_methodical = rule(
    implementation = _ssz_methodical_impl,
    attrs = {
        "config_file": attr.label(
            doc = "path to yaml config file for methodical --config flag ",
            mandatory = True,
            allow_single_file = True,
        ),
        "out": attr.output(
            doc = "The new Go file to emit the generated mocks into",
        ),
        "override_package_name": attr.string(
            doc = "Override the name of the package the generated file is in (eg 'eth' for proto/prysm/v1alpha1)",
            mandatory = False,
        ),
        # Build-wide progressive-merkleization toggle. Defaults to the
        # //tools:disable_progressive_merkleization bool_flag so a single
        # `--//tools:disable_progressive_merkleization` flips every
        # ssz_methodical target to --disable-progressive with no per-call edits.
        "_disable_progressive": attr.label(
            default = "//tools:disable_progressive_merkleization",
        ),
        # The //proto:network string_flag (mainnet|minimal) that ssz_proto_files
        # selects on. Read here to pick the //go:build constraint methodical
        # stamps, so --//proto:network=minimal yields a `//go:build minimal`
        # file matching //build/gen's minimal twin, with no per-call edits.
        "_network": attr.label(
            default = "//proto:network",
        ),
        "deps": attr.label_list(aspects = [go_pkg_info_aspect]),
        # provides access to the go toolchain via go_context(), enabling package driver indexing.
        "_go_context_data": attr.label(
            default = "@io_bazel_rules_go//:go_context_data",
        ),
        "methodical_tool": attr.label(
            doc = "The methodical tool (binary) to run",
            default = _METHODICAL_TOOL,
            allow_single_file = True,
            executable = True,
            cfg = "exec",
            mandatory = False,
        ),
        "genception": attr.label(
            doc = "gopackagesdriver tool for package discovery inside bazel sandbox",
            default = _GENCEPTION_TOOL,
            allow_single_file = True,
            executable = True,
            cfg = "exec",
            mandatory = False,
        ),
        # injects ssz library into the generation enviroment for indexing by the package driver.
        "ssz_lib": attr.label(providers = [GoLibrary], default = _SSZ_RUNTIME, aspects = [go_pkg_info_aspect]),
    },
    toolchains = ["@io_bazel_rules_go//go:toolchain"],
)

def _ssz_gen_spectest_impl(ctx):
    # gen-spectest does no package loading or code generation: it only
    # interpolates names from the yaml config and copies fixtures out of a
    # consensus-spec-tests release tarball. So, unlike ssz_methodical (the
    # `gen` subcommand), it needs none of the gopackagesdriver / go_pkg_info
    # apparatus -- just the config, the tarball, and the methodical binary.
    out_dir = ctx.actions.declare_directory(ctx.attr.out_dir)
    tool = ctx.file.methodical_tool

    args = [
        "gen-spectest",
        # The tool reads the archive via a file:// URI; resolve it to an
        # absolute path under the exec root at action time.
        "--release-uri=file://$(pwd)/" + ctx.file.release_tarball.path,
        "--config=" + ctx.file.config_file.path,
        "--output=" + out_dir.path,
    ]
    if ctx.attr.package_name != "":
        args.append("--package-name=" + ctx.attr.package_name)

    ctx.actions.run_shell(
        inputs = [ctx.file.config_file, ctx.file.release_tarball],
        tools = [tool],
        outputs = [out_dir],
        command = "$(pwd)/{tool} {args}".format(
            tool = tool.path,
            args = " ".join(args),
        ),
        mnemonic = "SszGenSpectest",
        progress_message = "Generating ssz spectests from %s" % ctx.file.config_file.short_path,
    )
    return [DefaultInfo(files = depset([out_dir]))]

ssz_gen_spectest = rule(
    implementation = _ssz_gen_spectest_impl,
    attrs = {
        "config_file": attr.label(
            doc = "path to the spectest yaml config (same format as the standalone spectest command)",
            mandatory = True,
            allow_single_file = True,
        ),
        "release_tarball": attr.label(
            doc = "consensus-spec-tests release tarball, e.g. @consensus_spec_tests//:mainnet.tar.gz",
            mandatory = True,
            allow_single_file = True,
        ),
        "out_dir": attr.string(
            doc = "name of the generated tree-artifact directory (holds methodical_test.go and testdata/)",
            mandatory = True,
        ),
        "package_name": attr.string(
            doc = "package name for the generated test file (default: 'spectest', set by the tool)",
            mandatory = False,
        ),
        "methodical_tool": attr.label(
            doc = "The methodical tool (binary) to run",
            default = _METHODICAL_TOOL,
            allow_single_file = True,
            executable = True,
            cfg = "exec",
            mandatory = False,
        ),
    },
)
