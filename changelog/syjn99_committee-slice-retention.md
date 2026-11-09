### Fixed

- `ComputeCommittee` now copies the committee out of the shuffled validator list instead of returning a sub-slice, which kept the whole per-validator array (~9 MB on Hoodi, ~19 MB on mainnet) alive for every committee a caller held.
