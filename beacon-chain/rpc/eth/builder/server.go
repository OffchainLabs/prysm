package builder

import (
	"github.com/prysmaticlabs/prysm/v6/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v6/beacon-chain/rpc/lookup"
)

type Server struct {
	FinalizationFetcher   blockchain.FinalizationFetcher
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	Stater                lookup.Stater
}
