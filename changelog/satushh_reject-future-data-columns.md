### Fixed

- Reject data column sidecars from future slots in `ReceiveDataColumns`, so they can't move the storage pruning horizon past current data.
