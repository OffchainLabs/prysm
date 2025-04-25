package core

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	opfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/synccommittee"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
)

type Service struct {
	BeaconDB              db.ReadOnlyDatabase
	ChainInfoFetcher      blockchain.ChainInfoFetcher
	HeadFetcher           blockchain.HeadFetcher
	FinalizedFetcher      blockchain.FinalizationFetcher
	GenesisTimeFetcher    blockchain.TimeFetcher
	SyncChecker           sync.Checker
	Broadcaster           p2p.Broadcaster
	SyncCommitteePool     synccommittee.Pool
	OperationNotifier     opfeed.Notifier
	AttestationCache      *cache.AttestationDataCache
	StateGen              stategen.StateManager
	P2P                   p2p.Broadcaster
	ReplayerBuilder       stategen.ReplayerBuilder
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
}
