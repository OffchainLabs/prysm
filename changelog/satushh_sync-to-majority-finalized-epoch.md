### Fixed

- Sync to the finalized epoch most peers agree on during initial sync, so one peer claiming a higher finalized epoch can no longer stall it.
- Count votes for finalized epoch 0 in `BestFinalized`, where one peer with a higher epoch could outvote a majority at epoch 0.
