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
Set `--repo_env=CONSENSUS_SPEC_TESTS_VERSION=nightly` when running spectest to download the "nightly" spectests. 
Note: A GITHUB_TOKEN environment variable is required to be set. The github token must be a [fine grained token](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens#creating-a-fine-grained-personal-access-token).

```
bazel test //... --test_tag_filters=spectest --repo_env=CONSENSUS_SPEC_TESTS_VERSION=nightly
```
