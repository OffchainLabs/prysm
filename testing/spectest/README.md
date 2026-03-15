# Spec Tests

Spec testing vectors: https://github.com/ethereum/consensus-spec-tests

To run all spectests:

```bash
go test ./testing/spectest/... -tags spectest
```

## Adding new tests

New tests must adhere to the following filename convention:

```
{mainnet/minimal/general}/$fork__$package__$test_test.go
```

An example test is the phase0 epoch processing test for effective balance updates. This test has a spectest path of `{mainnet, minimal}/phase0/epoch_processing/effective_balance_updates/pyspec_tests`.
There are tests for mainnet and minimal config, so for each config we will add a file by the name of `phase0__epoch_processing__effective_balance_updates_test.go` since the fork is `phase0`, the package is `epoch_processing`, and the test is `effective_balance_updates`.
