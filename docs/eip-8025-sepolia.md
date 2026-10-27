# EIP-8025 on Sepolia

Use the `ethpandaops/prysm-beacon-chain:optional-proofs` image, and add these flags to your usual beacon node command.

## Verifier

```
--sepolia
--zkvm
--p2p-host-ip=${P2P_HOST_IP}
--trusted-peer-id=16Uiu2HAmQxANYUvxH9bdN71jbHrGgTKE6ngtwR8R1yPBPmmNEnEc
```

- `--zkvm`: enables proof verification.
- `--p2p-host-ip`: your public IP. The Sepolia prover cannot accept inbound connections, so it must connect to you. Your P2P ports must be open.
- `--trusted-peer-id`: the prover. Never disconnected by your node (not strictly needed).
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

## Prover

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
