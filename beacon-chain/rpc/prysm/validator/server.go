package validator

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
)

type Server struct {
	BeaconDB            db.ReadOnlyDatabase
	Stater              lookup.Stater
	CanonicalFetcher    blockchain.CanonicalFetcher
	FinalizationFetcher blockchain.FinalizationFetcher
	ChainInfoFetcher    blockchain.ChainInfoFetcher
	CoreService         *core.Service
}
