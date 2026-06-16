# Spec Tests

Spec testing vectors: https://github.com/ethereum/consensus-spec-tests

To run all spectests (test vectors are fetched on demand by `build/externaldata`; run
`make testdata` first to pre-fetch). The minimal-config suite needs the `minimal` build tag:

```bash
go test ./testing/spectest/general/... ./testing/spectest/mainnet/...
go test -tags=minimal ./testing/spectest/minimal/...
```

They also run as part of `make test` (mainnet pass + minimal pass).

## Adding new tests

New tests must adhere to the following filename convention:

```
{mainnet/minimal/general}/$fork__$package__$test_test.go
```

An example test is the phase0 epoch processing test for effective balance updates. This test has a spectest path of `{mainnet, minimal}/phase0/epoch_processing/effective_balance_updates/pyspec_tests`.
There are tests for mainnet and minimal config, so for each config we will add a file by the name of `phase0__epoch_processing__effective_balance_updates_test.go` since the fork is `phase0`, the package is `epoch_processing`, and the test is `effective_balance_updates`.

## Spec-test version

The consensus-spec-tests version is pinned in `build/externaldata/externaldata.go`
(`consensusSpecVersion`) and fetched + sha256-verified on demand. To test against a
different release, change that pin and the matching `.ethspecify.yml` version.

(The Bazel-era "nightly" download via `--repo_env=CONSENSUS_SPEC_TESTS_VERSION` is not
ported to the Go toolchain.)
