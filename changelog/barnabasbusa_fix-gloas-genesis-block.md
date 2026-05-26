### Fixed

- Fixed Gloas genesis block reconstruction so the body's `signed_execution_payload_bid.message` mirrors `state.latest_execution_payload_bid`, matching the body root committed to in `state.latest_block_header`. Without this, Prysm built a divergent genesis block (different `block_root` / `body_root` from other clients) and the validator client failed to propose at slot 1 with "parent root … does not match the latest block header signing root in state". Also initializes `parent_execution_requests` so the body's SSZ hash tree root can be computed.
