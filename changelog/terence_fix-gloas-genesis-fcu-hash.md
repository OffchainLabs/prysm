### Fixed
- Use `LatestBlockHash` instead of `LatestExecutionPayloadHeader` for Gloas genesis states in `hashForGenesisBlock`, fixing zero head block hash in forkchoice updates at Gloas genesis.
