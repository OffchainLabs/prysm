### Changed

- Update `notifyForkchoiceUpdate` to derive `ForkchoiceState.HeadBlockHash` from `headState.LatestBlockHash()` for Gloas blocks, instead of reading `Body().Execution()` (which is not available on Gloas blocks)
