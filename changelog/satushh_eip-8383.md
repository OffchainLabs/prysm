### Changed

- EIP-8383: Reduce the mainnet `MIN_EPOCHS_FOR_BLOCK_REQUESTS` from 33024 epochs (~5 months) to 8192 epochs (~36 days). Backfill, `--beacon-db-pruning` and the advertised earliest available slot now follow the shorter window.
