package node

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
)

type Server struct {
	SyncChecker               sync.Checker
	OptimisticModeFetcher     blockchain.OptimisticModeFetcher
	BeaconDB                  db.ReadOnlyDatabase
	PeersFetcher              p2p.PeersProvider
	PeerManager               p2p.PeerManager
	MetadataProvider          p2p.MetadataProvider
	GenesisTimeFetcher        blockchain.TimeFetcher
	HeadFetcher               blockchain.HeadFetcher
	ExecutionChainInfoFetcher execution.ChainInfoFetcher
}
