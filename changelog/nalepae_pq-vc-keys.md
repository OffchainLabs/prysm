### Added

- In the PQ fork, the validator client derives the post-quantum keys of its validators at startup, and finds them on the beacon node by XMSS public key.
- In the PQ fork, the validator client builds the full VRF trees of its validators from the VRF state the genesis tool publishes, set with the new `--vrf-state-url` flag. This is compatible with Lighthouse's `pq-devnet-0`.
