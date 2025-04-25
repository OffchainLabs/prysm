// Package beacon defines a gRPC beacon service implementation,
// following the official API standards https://ethereum.github.io/beacon-apis/#/.
// This package includes the beacon and config endpoints.
package beacon

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	blockfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/block"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/voluntaryexits"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

// Server defines a server implementation of the gRPC Beacon Chain service,
// providing RPC endpoints to access data relevant to the Ethereum Beacon Chain.
type Server struct {
	BeaconDB                db.ReadOnlyDatabase
	ChainInfoFetcher        blockchain.ChainInfoFetcher
	GenesisTimeFetcher      blockchain.TimeFetcher
	BlockReceiver           blockchain.BlockReceiver
	BlockNotifier           blockfeed.Notifier
	OperationNotifier       operation.Notifier
	Broadcaster             p2p.Broadcaster
	AttestationCache        *cache.AttestationCache
	AttestationsPool        attestations.Pool
	SlashingsPool           slashings.PoolManager
	VoluntaryExitsPool      voluntaryexits.PoolManager
	StateGenService         stategen.StateManager
	Stater                  lookup.Stater
	Blocker                 lookup.Blocker
	HeadFetcher             blockchain.HeadFetcher
	TimeFetcher             blockchain.TimeFetcher
	OptimisticModeFetcher   blockchain.OptimisticModeFetcher
	V1Alpha1ValidatorServer eth.BeaconNodeValidatorServer
	SyncChecker             sync.Checker
	CanonicalHistory        *stategen.CanonicalHistory
	ExecutionReconstructor  execution.Reconstructor
	FinalizationFetcher     blockchain.FinalizationFetcher
	BLSChangesPool          blstoexec.PoolManager
	ForkchoiceFetcher       blockchain.ForkchoiceFetcher
	CoreService             *core.Service
	AttestationStateFetcher blockchain.AttestationStateFetcher
}
