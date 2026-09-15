### Fixed

- Verify a bid's gas limit against the payload identified by `bid.parent_block_hash` instead of the parent block's own payload, so bids that build on the parent's parent payload (empty parent) are no longer rejected with "bid gas limit is incompatible with parent and target".
