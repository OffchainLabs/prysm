### Added

- Gossip-level validation (slot, bid consistency, builder signature) on `POST /eth/v1/beacon/execution_payload_envelopes` for the default and `broadcast_validation=gossip` levels; failures return 400 and the envelope is not broadcast.
- `broadcast_validation=gossip` on the block publish endpoints now verifies the proposer signature before broadcast (previously a no-op); the default level is unchanged.
