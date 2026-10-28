# EIP-8025 on Sepolia

Use the `ethpandaops/prysm-beacon-chain:optional-proofs` Docker image, and add these flags to your usual beacon node command.

To run a Prysm beacon node and an execution client with Docker Compose, follow [this guide](https://prysm-doc.vercel.app/docs/setup/run-with-docker-compose/).

## Running a verifier node

```
--sepolia
--zkvm
--p2p-host-ip=${P2P_HOST_IP}
--trusted-peer-id=16Uiu2HAmQxANYUvxH9bdN71jbHrGgTKE6ngtwR8R1yPBPmmNEnEc
--min-execution-proofs=2
```

### Mandatory

- `--sepolia`: runs on Sepolia.
- `--zkvm`: enables proof verification.

### Recommended: help the prover connect to you

Not strictly needed, but the single Sepolia prover currently running cannot accept inbound connections: you only get its proofs if it connects to you, or through other peers relaying them. At the moment, very few peers are subscribed to execution proofs, so relaying is unlikely.

- `--p2p-host-ip`: your public IP, so the prover can dial you. Your P2P ports must be open.
- `--trusted-peer-id`: the peer ID of the prover, `16Uiu2HAmQxANYUvxH9bdN71jbHrGgTKE6ngtwR8R1yPBPmmNEnEc`. Your node never disconnects it, and accepts it even if your peer slots are full.

### Tuning

- `--min-execution-proofs` (default `2`): number of distinct proof types needed, on top of the EL validation, for a payload to no longer be optimistic. `0` follows the spec (proofs never required).

Expected logs (`DEBUG` lines need `--verbosity=debug`):

```
[2026-10-27 17:51:48.29]  INFO blockchain: Synced optimistic execution payload envelope blockHash=0x63c4e6e1f305 blockRoot=0x23ae9451ce15 gasUtilized=26.69% parentHash=0x4372fa87ada9 sinceSlotStart=296.744685ms slot=11449159
[2026-10-27 17:51:53.18] DEBUG sync: Received and verified execution proof blockRoot=0x23ae9451ce15 proofSize=268 kB proofType=reth-openvm prover=2088 sinceSlotStart=5.19s slot=11449159
[2026-10-27 17:51:53.75] DEBUG sync: Received and verified execution proof blockRoot=0x23ae9451ce15 proofSize=268 kB proofType=ethrex-openvm prover=2088 sinceSlotStart=5.758s slot=11449159
[2026-10-27 17:51:53.75]  INFO blockchain: Payload validated by the EL and execution proofs blockRoot=0x23ae9451ce15 proofTypeCount=2 sinceSlotStart=5.758625406s slot=11449159
```

1. Payload validated by the EL, still optimistic (no proof yet).
2. First proof received and verified.
3. Second proof, of another type, received and verified.
4. `--min-execution-proofs` reached: payload no longer optimistic.

Expected metrics, from the "Execution proofs" tab of [this Grafana dashboard](https://github.com/nalepae/infra/blob/master/beacon-node-dashboard.json):

![Execution proofs dashboard](https://github.com/user-attachments/assets/abe2a2b6-a2bc-44f1-a175-4711d5810e32)

## Running a prover node

```
--sepolia
--zkvm
--p2p-host-ip=${P2P_HOST_IP}
--execution-endpoint=http://<ZKBOOST_HOST>:3000
```

- `--execution-endpoint`: points to [zkboost](https://github.com/eth-act/zkboost) instead of the EL.

zkboost config (`zkboost --config config.toml`):

```toml
port = 3000                                         # Engine API, used by the beacon node
el_engine_endpoint = "http://<EL_HOST>:8551"        # the EL
cl_beacon_endpoint = "http://<BEACON_HOST>:3500"    # the beacon node REST API, receives the proofs

# Validator signing the proofs.
validator_keystore_path = "/validator-keys/keys/<pubkey>/voting-keystore.json"
validator_keystore_password_path = "/validator-keys/secrets/<pubkey>"

# One entry per proof type.
[[zkvm]]
kind = "cluster"
proof_type = "reth-openvm"
endpoint = "http://<OPENVM_CLUSTER_HOST>:3000"
elf_url = "<RETH_OPENVM_ELF_URL>"

[[zkvm]]
kind = "cluster"
proof_type = "ethrex-openvm"
endpoint = "http://<OPENVM_CLUSTER_HOST>:3000"
elf_url = "<ETHREX_OPENVM_ELF_URL>"
```

See the [zkboost configuration](https://github.com/eth-act/zkboost#configuration) for the other zkVM backends.

Working if you see:

```
INFO prover: Broadcast execution proof blockRoot=0xedb3d11063f5 proofSize=262 kB proofType=ethrex-sp1 prover=243 slot=43
```

## Hack / build from source

The code lives on the [`optional-proofs`](https://github.com/OffchainLabs/prysm/tree/optional-proofs) branch ([PR #17690](https://github.com/OffchainLabs/prysm/pull/17690)). To build it, follow the [developer guide](https://prysm-doc.vercel.app/docs/developers/).
