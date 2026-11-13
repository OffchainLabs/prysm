### Fixed

- Reject a block batch during initial sync when the first block's parent is a stored block at or below the finalized slot that is not the finalized root. Before, the batch skipped the finalized-descendant check and ran the state transition from that old state.
