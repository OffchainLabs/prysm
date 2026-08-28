# Spec Tests

Spec testing vectors: https://github.com/ethereum/consensus-spec-tests

To run all spectests:

```bash
bazel test //... --test_tag_filters=spectest
```

## Adding new tests

New tests must adhere to the following filename convention:

```
{mainnet/minimal/general}/$fork__$package__$test_test.go
```

An example test is the phase0 epoch processing test for effective balance updates. This test has a spectest path of `{mainnet, minimal}/phase0/epoch_processing/effective_balance_updates/pyspec_tests`.
There are tests for mainnet and minimal config, so for each config we will add a file by the name of `phase0__epoch_processing__effective_balance_updates_test.go` since the fork is `phase0`, the package is `epoch_processing`, and the test is `effective_balance_updates`.

## Running nightly spectests

Since [PR 15312](https://github.com/OffchainLabs/prysm/pull/15312), Prysm has support to download "nightly" spectests from github via a starlark rule configuration by environment variable.
Set `--repo_env=CONSENSUS_SPEC_TESTS_VERSION=nightly` or `--repo_env=CONSENSUS_SPEC_TESTS_VERSION=nightly-<run_id>` when running spectest to download the "nightly" spectests.
Note: A GITHUB_TOKEN environment variable is required to be set. The github token does not need to be associated with your main account; it can be from a "burner account". And the token does not need to be a fine-grained token; it can be a classic token.

```
bazel test //... --test_tag_filters=spectest --repo_env=CONSENSUS_SPEC_TESTS_VERSION=nightly
```

```
bazel test //... --test_tag_filters=spectest --repo_env=CONSENSUS_SPEC_TESTS_VERSION=nightly-21422848633
```

## Using local spectest data

To run spectests against data you already have on disk (e.g. for offline work,
or to test an unreleased spec build) instead of downloading it, use either of the
following `--repo_env` flags. A source can be a tarball **or an already-unpacked
directory tree**. Both flags take precedence over `CONSENSUS_SPEC_TESTS_VERSION`
and require no network access. All paths must be **absolute**.

The "flavors" are the three upstream consensus-specs presets: `mainnet` and `minimal`
(preset-specific configs) and `general` (preset-independent tests: SSZ generic, BLS,
KZG). You usually only need one at a time.

**A directory.** Point `CONSENSUS_SPEC_TESTS_DIR` at a directory and, for each
flavor, the rule uses the first of these it finds, skipping flavors that are absent:

- `<dir>/<flavor>.tar.gz` — a release-style tarball
- `<dir>/tests/<flavor>/` — an unpacked tree keeping the upstream `tests/` prefix
- `<dir>/<flavor>/` — an unpacked tree with the prefix stripped

So both a directory of tarballs and a directory of already-extracted tests work:

```
bazel test //... --test_tag_filters=spectest --repo_env=CONSENSUS_SPEC_TESTS_DIR=/abs/path/to/specs
```

**A single flavor with any name.** Point a specific flavor at an arbitrarily-named
tarball or directly at an unpacked flavor tree with `CONSENSUS_SPEC_TESTS_<FLAVOR>`
(`GENERAL`, `MAINNET`, or `MINIMAL`):

```
bazel test //testing/spectest/mainnet:go_default_test --test_tag_filters=spectest \
  --repo_env=CONSENSUS_SPEC_TESTS_MAINNET=/abs/path/to/my-mainnet.tar.gz
```

The two mechanisms can be combined; a per-flavor override wins over the directory
entry for that flavor. When you supply only some flavors, scope the test target to
the matching preset (e.g. `//testing/spectest/mainnet/...`) rather than `//...`, since
the other flavors' filegroups will be empty.

Notes:
- Unpacked directories are symlinked in, so no large copy is made. The raw
  `@consensus_spec_tests//:<flavor>.tar.gz` target (used by methodical-ssz
  `gen-spectest`) only exists when you supply an actual tarball, not an unpacked dir.
- Bazel keys the repository rule on the value of the env var, not the contents of the
  files. If you change data at the same path, run `bazel sync --configure` (or change
  the path) to force a re-fetch.
