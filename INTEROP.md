# Prysm Client Interoperability Guide

The recommended way to run Prysm against other Ethereum consensus/execution
clients is [ethereum-package](https://github.com/ethpandaops/ethereum-package),
a [Kurtosis](https://docs.kurtosis.com/) package that spins up a full multi-client
devnet (execution + consensus + validators) with genesis generated for you.

> [!NOTE]
> The deterministic interop key flags (`--interop-num-validators`,
> `--interop-start-index`, `--interop-eth1data-votes`) have been removed. Use
> ethereum-package instead — it provisions validator keys and genesis automatically.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Kurtosis CLI](https://docs.kurtosis.com/install/)

## Running a devnet with Prysm

Write a `network_params.yaml` selecting Prysm as the consensus client:

```yaml
participants:
  - el_type: geth
    cl_type: prysm
  - el_type: nethermind
    cl_type: lighthouse
```

Then launch the enclave:

```sh
kurtosis run github.com/ethpandaops/ethereum-package --args-file ./network_params.yaml
```

Kurtosis prints the RPC/API endpoints for every started service. See the
[ethereum-package configuration reference](https://github.com/ethpandaops/ethereum-package#configuration)
for the full set of options (fork schedule, validator counts, extra flags, etc.).

## Using a local Prysm build

By default `cl_type: prysm` pulls the published Prysm image. To test local
changes, build and load a Docker image into your local daemon with:

```sh
bazel run //cmd/beacon-chain:oci_image_tarball
bazel run //cmd/validator:oci_image_tarball
```

Then point the participant at the built images via `cl_image` and `vc_image` in `network_params.yaml`.

## Generating a genesis state manually

If you only need a `genesis.ssz` (e.g. for a custom harness), `prysmctl` still
generates one from a chain config:

```sh
curl https://raw.githubusercontent.com/ethereum/consensus-specs/refs/heads/dev/configs/minimal.yaml -o /tmp/minimal.yaml

bazel run //cmd/prysmctl --config=minimal -- \
  testnet generate-genesis \
  --genesis-time-delay=120 \
  --num-validators=256 \
  --output-ssz=/tmp/genesis.ssz \
  --chain-config-file=/tmp/minimal.yaml
```
