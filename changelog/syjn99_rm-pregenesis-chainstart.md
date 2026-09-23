### Removed

- Removed the dead eth1-driven pre-genesis chain-start path from the execution service (`ProcessChainStart`, `ChainStartFetcher`, `statefeed.ChainStarted`, `IsValidGenesisState`) and reserved its `ETH1ChainData`/`ChainStartData` proto fields; the genesis state is always loaded before services start since #15470.
