### Changed

- Performance improvement in state (MarshalSSZ): use copy() instead of byte-by-byte loop in BlockRoots, RandaoMixes, and HistoricalRoots.