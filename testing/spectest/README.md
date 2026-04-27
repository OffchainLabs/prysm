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

## Fork-choice compliance tests

The consensus-specs ["Compliance Tests"](https://github.com/ethereum/consensus-specs/actions/workflows/compliance-tests.yml) workflow generates additional fork-choice scenarios via the runners under [`tests/generators/compliance_runners/fork_choice`](https://github.com/ethereum/consensus-specs/tree/master/tests/generators/compliance_runners/fork_choice). The on-disk layout matches the standard `fork_choice` tests (`<preset>/<fork>/fork_choice_compliance/<suite>/pyspec_tests/<case>/...`) so the same harness in `testing/spectest/shared/common/forkchoice` runs them.

Compliance data is currently shipped only for the `minimal` preset; pre-Fulu forks have no compliance suites.

### Test data resolution

The runner resolves data in this order:

1. `COMPLIANCE_FC_DIR` — absolute path to an unpacked compliance-tests tarball, rooted at the directory that contains `tests/`. Used for local development.
2. Bazel runfiles — the standard `@consensus_spec_tests//:test_data` path, used when the configured `CONSENSUS_SPEC_TESTS_VERSION` ships compliance suites.

If neither path resolves, the per-suite tests log a skip and exit cleanly.

### Running locally

The easiest path is `hack/compliance-fc-report.sh`, which fetches a compliance tarball, runs the suites under bazel sharding, and prints an aggregated per-suite report. It supports several data sources — see `hack/compliance-fc-report.sh --help` for the full list. Highlights:

```bash
# Auto-fetch latest from the consensus-specs Compliance Tests workflow on master.
# This path needs gh and GITHUB_TOKEN — GitHub Actions artifact downloads return
# 403 to anonymous requests, even on public repos.
GITHUB_TOKEN=ghp_... ./hack/compliance-fc-report.sh

# Use a manually-downloaded tarball — no token, no gh required.
./hack/compliance-fc-report.sh --tarball ~/Downloads/small.tar.gz

# Or pull from a public mirror via curl — also no token.
./hack/compliance-fc-report.sh --url https://example.org/small.tar.gz

# Re-use an already-extracted tree.
./hack/compliance-fc-report.sh --dir /var/tmp/compliance_fc_root

# Run only one suite (extra args are forwarded to `bazel test`).
./hack/compliance-fc-report.sh -- --test_filter='..._BlockTree$'
```

Tarballs are cached at `/var/tmp/compliance_fc_cache/<key>/` (the cache key is the run id, the tarball SHA, or the URL SHA depending on source); re-runs against the same source reuse the cache. Override the cache root with `--cache-dir` or `COMPLIANCE_FC_CACHE_DIR`. The `--preset` flag (`minimal`/`mainnet`) selects the bazel target; only `minimal` currently ships compliance data.

To bypass the helper entirely — for example to pin specific bazel flags or work offline — unpack the tarball yourself and set `COMPLIANCE_FC_DIR`:

```bash
mkdir -p /var/tmp/compliance_fc_root
tar -xzf small.tar.gz -C /var/tmp/compliance_fc_root  # leaves /var/tmp/compliance_fc_root/tests/...

bazel test //testing/spectest/minimal:go_default_test --config=minimal \
  --test_filter='TestMinimal_(Fulu|Gloas)_ForkchoiceCompliance_' \
  --test_env=COMPLIANCE_FC_DIR=/var/tmp/compliance_fc_root \
  --test_env=PRYSM_SPECTEST_SKIP_BLS=1 \
  --test_timeout=900 --local_test_jobs=12
```

On macOS, prefer `/var/tmp` over `/tmp` — `.bazelrc` mounts `/var/tmp` into the sandbox via `--sandbox_add_mount_pair=/var/tmp`.

### `PRYSM_SPECTEST_SKIP_BLS`

Compliance test cases ship with `bls_setting: 2` in their `meta.yaml`, meaning signatures are placeholder bytes and verification must be skipped. Prysm's spec runners do not parse `meta.yaml`, so `crypto/bls/signature_batch.go` exposes a process-wide escape hatch: when `PRYSM_SPECTEST_SKIP_BLS=1`, `SignatureBatch.Verify` and `SignatureBatch.VerifyVerbosely` short-circuit to `(true, nil)`. Never set this outside spec tests — it disables every batched signature check.

### Per-suite split and sharding

Each compliance suite (`block_tree_test`, `shuffling_test`, …) is wired as its own top-level `Test*` function, e.g. `TestMinimal_Fulu_ForkchoiceCompliance_BlockTree`. The wiring is one-line wrappers around `forkchoice.RunComplianceSuite(t, preset, fork, suite)`. The split lets bazel test sharding (`shard_count = 12` on `//testing/spectest/minimal:go_default_test`) fan suites out across processes — global state (`helpers.ClearCache()`, `params.OverrideBeaconConfig`) is per-process so shards do not interfere. With `--local_test_jobs=12` the wall-clock cost approaches `max(suite_duration)` rather than the sum.

Adding a new fork or suite means adding the matching one-line wrapper file and listing it in `BUILD.bazel`; the runner picks up directories on disk so no Go changes are needed for new test cases.
