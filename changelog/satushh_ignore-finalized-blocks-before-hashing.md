### Fixed

- Check whether a gossiped block is from a finalized slot or builds on a canonical parent from before the justified checkpoint before hashing it, so these cheap ignore checks no longer wait behind the block hash.
