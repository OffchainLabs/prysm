### Fixed

- Fix a nil pointer panic when serving a `BeaconBlocksByRange` request that starts at slot 0 after `--beacon-db-pruning` deleted the genesis block.
