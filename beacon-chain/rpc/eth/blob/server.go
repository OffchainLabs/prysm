package blob

import (
	"github.com/prysmaticlabs/prysm/v6/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v6/beacon-chain/rpc/lookup"
)

type Server struct {
	Blocker               lookup.Blocker
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	FinalizationFetcher   blockchain.FinalizationFetcher
	TimeFetcher           blockchain.TimeFetcher
}
