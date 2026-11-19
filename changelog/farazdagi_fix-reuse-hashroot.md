### Fixed

- Eliminated redundant HashTreeRoot computations in `SaveBlock` and `SaveBlocks` (by reusing cached roots from `ROBlock` wrapper).
