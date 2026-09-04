### Fixed

- Match a batch's parent payload envelope by beacon block root in addition to execution block hash during initial sync, so an ancestor's envelope is not applied as the first block's parent envelope when the parent payload was not revealed.
- Skip the origin data column sidecar prefetch for Gloas checkpoint origins; data availability attaches to the payload envelope, which forward sync imports along with its columns when it exists.
- Persist data column sidecars fetched for already-imported blocks (such as the checkpoint origin) instead of dropping them with the processed portion of a batch.
