package beacon

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	beacondb "github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
)

type Server struct {
	SyncChecker           sync.Checker
	HeadFetcher           blockchain.HeadFetcher
	TimeFetcher           blockchain.TimeFetcher
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	CanonicalHistory      *stategen.CanonicalHistory
	BeaconDB              beacondb.ReadOnlyDatabase
	Stater                lookup.Stater
	Blocker               lookup.Blocker
	ChainInfoFetcher      blockchain.ChainInfoFetcher
	FinalizationFetcher   blockchain.FinalizationFetcher
	CoreService           *core.Service
	Broadcaster           p2p.Broadcaster
	BlobReceiver          blockchain.BlobReceiver
}
