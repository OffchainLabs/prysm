### Changed

- Add tracing spans to epoch processing (`core.state.ProcessEpoch`, `fulu.ProcessEpoch`), committee shuffling (`helpers.ShuffledIndices`), the committee cache (`committeeCache.AddCommitteeShuffledList`) and `blockChain.updateCachesPostBlockProcessing`.
- Avoid one interface boxing per validator in `validatorsReadOnlyVal` and `ValidatorsReadOnlySeq`, and per public key in `AggregateKeyFromIndices` and `ApplyToEveryValidator`, to reduce allocations.
