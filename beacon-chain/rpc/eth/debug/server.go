// Package debug defines a gRPC beacon service implementation,
// following the official API standards https://ethereum.github.io/beacon-apis/#/.
// This package includes the beacon and config endpoints.
package debug

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
)

// Server defines a server implementation of the gRPC Beacon Chain service,
// providing RPC endpoints to access data relevant to the Ethereum Beacon Chain.
type Server struct {
	BeaconDB              db.ReadOnlyDatabase
	HeadFetcher           blockchain.HeadFetcher
	Stater                lookup.Stater
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	ForkFetcher           blockchain.ForkFetcher
	ForkchoiceFetcher     blockchain.ForkchoiceFetcher
	FinalizationFetcher   blockchain.FinalizationFetcher
	ChainInfoFetcher      blockchain.ChainInfoFetcher
	GenesisTimeFetcher    blockchain.TimeFetcher
	Blocker               lookup.Blocker
}
