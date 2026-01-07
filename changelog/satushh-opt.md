### Changed

- Performance improvement in state (MarshalSSZTo): use copy() instead of byte-by-byte loop which isn't required.