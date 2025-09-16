package blockchain

import (
	"time"

	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice"
	lightclient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/voluntaryexits"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

type Option func(s *Service) error

// WithMaxGoroutines to control resource use of the blockchain service.
func WithMaxGoroutines(x int) Option {
	return func(s *Service) error {
		s.cfg.MaxRoutines = x
		return nil
	}
}

// WithLCStore for light client store access.
func WithLCStore() Option {
	return func(s *Service) error {
		s.lcStore = lightclient.NewLightClientStore(s.cfg.P2P, s.cfg.StateNotifier.StateFeed(), s.cfg.BeaconDB)
		return nil
	}
}

// WithWeakSubjectivityCheckpoint for checkpoint sync.
func WithWeakSubjectivityCheckpoint(c *ethpb.Checkpoint) Option {
	return func(s *Service) error {
		s.cfg.WeakSubjectivityCheckpt = c
		return nil
	}
}

// WithDatabase for head access.
func WithDatabase(beaconDB db.HeadAccessDatabase) Option {
	return func(s *Service) error {
		s.cfg.BeaconDB = beaconDB
		return nil
	}
}

// WithChainStartFetcher to retrieve information about genesis.
func WithChainStartFetcher(f execution.ChainStartFetcher) Option {
	return func(s *Service) error {
		s.cfg.ChainStartFetcher = f
		return nil
	}
}

// WithExecutionEngineCaller to call execution engine.
func WithExecutionEngineCaller(c execution.EngineCaller) Option {
	return func(s *Service) error {
		s.cfg.ExecutionEngineCaller = c
		return nil
	}
}

// WithDepositCache for deposit lifecycle after chain inclusion.
func WithDepositCache(c cache.DepositCache) Option {
	return func(s *Service) error {
		s.cfg.DepositCache = c
		return nil
	}
}

// WithPayloadIDCache for payload ID cache.
func WithPayloadIDCache(c *cache.PayloadIDCache) Option {
	return func(s *Service) error {
		s.cfg.PayloadIDCache = c
		return nil
	}
}

// WithTrackedValidatorsCache for tracked validators cache.
func WithTrackedValidatorsCache(c *cache.TrackedValidatorsCache) Option {
	return func(s *Service) error {
		s.cfg.TrackedValidatorsCache = c
		return nil
	}
}

// WithAttestationCache for attestation lifecycle after chain inclusion.
func WithAttestationCache(c *cache.AttestationCache) Option {
	return func(s *Service) error {
		s.cfg.AttestationCache = c
		return nil
	}
}

// WithAttestationPool for attestation lifecycle after chain inclusion.
func WithAttestationPool(p attestations.Pool) Option {
	return func(s *Service) error {
		s.cfg.AttPool = p
		return nil
	}
}

// WithExitPool for exits lifecycle after chain inclusion.
func WithExitPool(p voluntaryexits.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.ExitPool = p
		return nil
	}
}

// WithSlashingPool for slashings lifecycle after chain inclusion.
func WithSlashingPool(p slashings.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.SlashingPool = p
		return nil
	}
}

// WithBLSToExecPool to keep track of BLS to Execution address changes.
func WithBLSToExecPool(p blstoexec.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.BLSToExecPool = p
		return nil
	}
}

// WithP2PBroadcaster to broadcast messages after appropriate processing.
func WithP2PBroadcaster(p p2p.Accessor) Option {
	return func(s *Service) error {
		s.cfg.P2P = p
		return nil
	}
}

// WithStateNotifier to notify an event feed of state processing.
func WithStateNotifier(n statefeed.Notifier) Option {
	return func(s *Service) error {
		s.cfg.StateNotifier = n
		return nil
	}
}

// WithForkChoiceStore to update an optimized fork-choice representation.
func WithForkChoiceStore(f forkchoice.ForkChoicer) Option {
	return func(s *Service) error {
		s.cfg.ForkChoiceStore = f
		return nil
	}
}

// WithAttestationService for dealing with attestation lifecycles.
func WithAttestationService(srv *attestations.Service) Option {
	return func(s *Service) error {
		s.cfg.AttService = srv
		return nil
	}
}

// WithStateGen for managing state regeneration and replay.
func WithStateGen(g *stategen.State) Option {
	return func(s *Service) error {
		s.cfg.StateGen = g
		return nil
	}
}

// WithSlasherAttestationsFeed to forward attestations into slasher if enabled.
func WithSlasherAttestationsFeed(f *event.Feed) Option {
	return func(s *Service) error {
		s.cfg.SlasherAttestationsFeed = f
		return nil
	}
}

// WithFinalizedStateAtStartUp to store finalized state at start up.
func WithFinalizedStateAtStartUp(st state.BeaconState) Option {
	return func(s *Service) error {
		s.cfg.FinalizedStateAtStartUp = st
		return nil
	}
}

// WithClockSynchronizer sets the ClockSetter/ClockWaiter values to be used by services that need to block until
// the genesis timestamp is known (ClockWaiter) or which determine the genesis timestamp (ClockSetter).
func WithClockSynchronizer(gs *startup.ClockSynchronizer) Option {
	return func(s *Service) error {
		s.clockSetter = gs
		s.clockWaiter = gs
		return nil
	}
}

// WithSyncComplete sets a channel that is used to notify blockchain service that the node has synced to head.
func WithSyncComplete(c chan struct{}) Option {
	return func(s *Service) error {
		s.syncComplete = c
		return nil
	}
}

// WithBlobStorage sets the blob storage backend for the blockchain service.
func WithBlobStorage(b *filesystem.BlobStorage) Option {
	return func(s *Service) error {
		s.blobStorage = b
		return nil
	}
}

// WithDataColumnStorage sets the data column storage backend for the blockchain service.
func WithDataColumnStorage(b *filesystem.DataColumnStorage) Option {
	return func(s *Service) error {
		s.dataColumnStorage = b
		return nil
	}
}

// WithSyncChecker sets the sync checker for the blockchain service.
func WithSyncChecker(checker Checker) Option {
	return func(s *Service) error {
		s.cfg.SyncChecker = checker
		return nil
	}
}

// WithSlasherEnabled sets whether the slasher is enabled or not.
func WithSlasherEnabled(enabled bool) Option {
	return func(s *Service) error {
		s.slasherEnabled = enabled
		return nil
	}
}

// WithGenesisTime sets the genesis time for the blockchain service.
func WithGenesisTime(genesisTime time.Time) Option {
	return func(s *Service) error {
		s.genesisTime = genesisTime.Truncate(time.Second) // Genesis time has a precision of 1 second.
		return nil
	}
}

// WithLightClientStore sets the light client store for the blockchain service.
func WithLightClientStore(lcs *lightclient.Store) Option {
	return func(s *Service) error {
		s.lcStore = lcs
		return nil
	}
}

// WithStartWaitingDataColumnSidecars sets a channel that the `areDataColumnsAvailable` function will fill
// in when starting to wait for additional data columns.
func WithStartWaitingDataColumnSidecars(c chan bool) Option {
	return func(s *Service) error {
		s.startWaitingDataColumnSidecars = c
		return nil
	}
}
