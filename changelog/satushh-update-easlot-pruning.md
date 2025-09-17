### Added

- Update the earliest available slot after pruning operations in both the beacon chain database pruner and data column filesystem storage. This ensures the P2P layer accurately knows which historical data is available after pruning, preventing nodes from advertising or attempting to serve data that has been pruned. 