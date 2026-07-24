### Changed

- Allow builders to bid on multiple branches, deduplicate bids per `(slot, parent_block_hash, parent_block_root)` and cap at `MAX_BIDS_PER_BUILDER` per slot.
