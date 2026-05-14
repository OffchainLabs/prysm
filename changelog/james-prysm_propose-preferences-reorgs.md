### Fixed

- made proposer preferences cache reorg resistent by switching map to map of maps and using slot + proposer index, has validator client retrigger submission on reorg detection.