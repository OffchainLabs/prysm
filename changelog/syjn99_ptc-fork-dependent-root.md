### Fixed

- PTC duties `dependent_root` for the Gloas fork epoch is now the block root at `compute_start_slot_at_epoch(epoch) - 1`, per [beacon-APIs#671](https://github.com/ethereum/beacon-APIs/pull/671). This also fixes a genesis root returned when `GLOAS_FORK_EPOCH` is 1.
- gRPC `GetPTCDuties` rejects the Gloas fork epoch until the fork, the same as the REST endpoint.
