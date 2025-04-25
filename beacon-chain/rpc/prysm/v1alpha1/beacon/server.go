// Package beacon defines a gRPC beacon service implementation, providing
// useful endpoints for checking fetching chain-specific data such as
// blocks, committees, validators, assignments, and more.
package beacon

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	blockfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/block"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

// Server defines a server implementation of the gRPC Beacon Chain service,
// providing RPC endpoints to access data relevant to the Ethereum beacon chain.
type Server struct {
	BeaconDB                    db.ReadOnlyDatabase
	Ctx                         context.Context
	ChainStartFetcher           execution.ChainStartFetcher
	HeadFetcher                 blockchain.HeadFetcher
	CanonicalFetcher            blockchain.CanonicalFetcher
	FinalizationFetcher         blockchain.FinalizationFetcher
	DepositFetcher              cache.DepositFetcher
	BlockFetcher                execution.POWBlockFetcher
	GenesisTimeFetcher          blockchain.TimeFetcher
	StateNotifier               statefeed.Notifier
	BlockNotifier               blockfeed.Notifier
	AttestationNotifier         operation.Notifier
	Broadcaster                 p2p.Broadcaster
	AttestationCache            *cache.AttestationCache
	AttestationsPool            attestations.Pool
	SlashingsPool               slashings.PoolManager
	ChainStartChan              chan time.Time
	ReceivedAttestationsBuffer  chan *ethpb.Attestation
	CollectedAttestationsBuffer chan []*ethpb.Attestation
	StateGen                    stategen.StateManager
	SyncChecker                 sync.Checker
	ReplayerBuilder             stategen.ReplayerBuilder
	OptimisticModeFetcher       blockchain.OptimisticModeFetcher
	CoreService                 *core.Service
}
