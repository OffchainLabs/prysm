package rewards

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/lookup"
)

type Server struct {
	Blocker               lookup.Blocker
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	FinalizationFetcher   blockchain.FinalizationFetcher
	TimeFetcher           blockchain.TimeFetcher
	Stater                lookup.Stater
	HeadFetcher           blockchain.HeadFetcher
	BlockRewardFetcher    BlockRewardsFetcher
}
