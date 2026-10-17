### Fixed

- Limit how many `execution_payload` gossip messages each peer can send into validation per slot, dropping the excess before gossipsub marks it as seen so another peer's copy is still validated. Ignore a correctly signed envelope whose `parent_beacon_block_root` does not match its block, so it cannot take the builder's seen slot.
