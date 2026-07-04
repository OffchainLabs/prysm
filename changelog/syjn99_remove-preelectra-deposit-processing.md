### Removed

- Removed the legacy pre-Electra (pre-EIP-6110) eth1 deposit-log processing pipeline: the execution service's deposit-contract log follower, chainstart-from-deposits genesis generation, the EIP-4881 deposit snapshot cache/trie (`beacon-chain/cache/depositsnapshot`), and the block-proposer eth1data majority voting + deposit packing. Post-Electra, eth1 data is frozen to the value in state and deposits flow inline as execution-layer requests, so proposers now set the frozen `Eth1Data` and empty legacy deposits directly.
- Removed the now-unused `--eth1-header-req-limit` and `--interop-eth1data-votes` beacon-chain flags along with their wiring.
