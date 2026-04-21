### Fixed

- Initialise `ExecutionRequestsRoot` on the Gloas genesis block's execution payload bid and `ParentExecutionRequests` on the block body so `NewGenesisBlockForState` can compute the genesis block root without hitting "bytes array does not have the correct length: expected 32 and 0 found".
