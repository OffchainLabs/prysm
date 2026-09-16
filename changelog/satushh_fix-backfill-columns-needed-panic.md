### Fixed

- Fixed a nil-pointer panic in backfill's `batch.columnsNeeded` for batches with no column work (all blocks pre-Fulu or outside the column retention window): the wrapper called the promoted `(*columnBatch).needed` on the nil embedded `columnBatch` instead of the guarded `(*columnSync).columnsNeeded`.
