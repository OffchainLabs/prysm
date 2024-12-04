# Prysm Client Interoperability Guide

This README details how to setup Prysm for interop testing for usage with other Ethereum consensus clients.

> [!IMPORTANT]  
> This guide is likely to be outdated. The Prysm team does not have capacity to troubleshoot
> outdated interop guides or instructions. If you experience issues with this guide, please file and
> issue for visibility and propose fixes, if possible.

## Installation & Setup

1. Install [Bazel](https://docs.bazel.build/versions/master/install.html) **(Recommended)**
2. `git clone https://github.com/prysmaticlabs/prysm && cd prysm`
3. `bazel build //cmd/...`

## Starting from Genesis

Prysm can be started from a built-in mainnet genesis state, or started with a provided genesis state by
using the `--genesis-state` flag and providing a path to the genesis.ssz file.

## Generating a Genesis State

To setup the necessary files for these quick starts, Prysm provides a tool to generate a `genesis.ssz` from
a deterministically generated set of validator private keys following the official interop YAML format 
[here](https://github.com/ethereum/eth2.0-pm/blob/master/interop/mocked_start).

You can use `prysmctl` to create a deterministic genesis state for interop.

```sh
# Download (or create) a chain config file.
curl https://raw.githubusercontent.com/ethereum/consensus-specs/refs/heads/dev/configs/minimal.yaml -o /tmp/minimal.yaml

# Run prysmctl to generate genesis with a 2 minute genesis delay and 256 validators. 
bazel run //cmd/prysmctl --config=minimal -- \
  testnet generate-genesis \
  --genesis-time-delay=120 \
  --num-validators=256 \
  --output-ssz=/tmp/genesis.ssz \
  --chain-config-file=/tmp/minimal.yaml
```

The flags are explained below:
- `bazel run //cmd/prysmctl` is the bazel command to compile and run prysmctl.
- `--config=minimal` is a bazel build time configuration flag to compile Prysm with minimal state constants.
- `--` is an argument divider to tell bazel that everything after this divider should be passed as arguments to prysmctl. Without this divider, it isn't clear to bazel if the arguments are meant to be build time arguments or runtime arguments so the operation complains and fails to build without this divider.
- `testnet` is the primary command argument for prysmctl.
- `generate-genesis` is the subcommand to `testnet` in prysmctl.
- `--genesis-time-delay` uint: The number of seconds in the future to define genesis. Example: a value of 60 will set the genesis time to 1 minute in the future. This should be sufficiently large enough to allow for you to start the beacon node before the genesis time. 
- `--num-validators` int: Number of validators to deterministically include in the generated genesis state
- `--output-ssz` string: Output filename of the SSZ marshaling of the generated genesis state
- `--chain-config-file` string: Filepath to a chain config yaml file.

Note: This guide saves items to the `/tmp/` directory which will not persist if your machine is
restarted. Consider tweaking the arguments if persistence is needed.

## Launching a Beacon Node + Validator Client

### Launching from Pure CLI Flags

Open up two terminal windows, run:

```
bazel run //cmd/beacon-chain --config=minimal -- \
  --minimal-config \
  --bootstrap-node= \
  --deposit-contract 0x8A04d14125D0FDCDc742F4A05C051De07232EDa4 \
  --datadir=/tmp/beacon-chain-minimal-devnet \
  --force-clear-db \
  --min-sync-peers=0 \
  --genesis-state=/tmp/genesis.ssz \
  --chain-config-file=/tmp/minimal.yaml
```

This will start the system with 256 validators. The flags used can be explained as such:

- `bazel run //cmd/beacon-chain --config=minimal` builds and runs the beacon node in minimal build configuration.
- `--` is a flag divider to distingish between bazel flags and flags that should be passed to the application. All flags and arguments after this divider are passed to the beacon chain.
- `--minimal-config` tells the beacon node to use minimal network configuration. This is different from the compile time state configuration flag `--config=minimal` and both are required.
- `--bootstrap-node=` disables the default bootstrap nodes. This prevents the client from attempting to peer with mainnet nodes.
- `--datadir=/tmp/beacon-chain-minimal-devnet` sets the data directory in a temporary location. Change this to your preferred destination.
- `--force-clear-db` will delete the beaconchain.db file without confirming with the user. This is helpful for iteratively running local devnets without changing the datadir, but less helpful for one off runs where there was no database in the data directory.
- `--min-sync-peers=0` allows the beacon node to skip initial sync without peers. This is essential because Prysm expects at least a few peers to start start the blockchain.
- `--genesis-state=/tmp/genesis.ssz` defines the path to the generated genesis ssz file. The beacon node will use this as the initial genesis state.
- `--chain-config-file=/tmp/minimal.yaml` defines the path to the yaml file with the chain configuration.

As soon as the beacon node has started, start the validator in the other terminal window. 

```
bazel run //cmd/validator --config=minimal -- --datadir=/tmp/validator --interopt-num-validators=256 --minimal-config --suggested-fee-recipient=0x8A04d14125D0FDCDc742F4A05C051De07232EDa4
```

This will launch and kickstart the system with your 256 validators performing their duties accordingly.
