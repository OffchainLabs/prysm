### Changed

- `--pruner-retention-epochs`: Values lower than `MIN_EPOCHS_FOR_BLOCK_REQUESTS` are not ignored any more. Instead, the node lowers `MIN_EPOCHS_FOR_BLOCK_REQUESTS` accordingly, which allows testing the pruner (including the state-diff pruning) without waiting for months of chain data. Such a node does not serve the block range mandated by the specification any more.
