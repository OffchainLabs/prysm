package lightclient

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
)

type Server struct {
	Blocker          lookup.Blocker
	Stater           lookup.Stater
	HeadFetcher      blockchain.HeadFetcher
	ChainInfoFetcher blockchain.ChainInfoFetcher
	BeaconDB         db.HeadAccessDatabase
}
