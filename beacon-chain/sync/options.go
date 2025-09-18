package sync

import (
	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	blockfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/block"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	lightClient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/synccommittee"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/voluntaryexits"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync/backfill/coverage"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
)

type Option func(s *Service) error

func WithAttestationNotifier(notifier operation.Notifier) Option {
	return func(s *Service) error {
		s.cfg.attestationNotifier = notifier
		return nil
	}
}

func WithP2P(p2p p2p.P2P) Option {
	return func(s *Service) error {
		s.cfg.p2p = p2p
		return nil
	}
}

func WithDatabase(db db.NoHeadAccessDatabase) Option {
	return func(s *Service) error {
		s.cfg.beaconDB = db
		return nil
	}
}

func WithAttestationCache(c *cache.AttestationCache) Option {
	return func(s *Service) error {
		s.cfg.attestationCache = c
		return nil
	}
}

func WithAttestationPool(attPool attestations.Pool) Option {
	return func(s *Service) error {
		s.cfg.attPool = attPool
		return nil
	}
}

func WithExitPool(exitPool voluntaryexits.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.exitPool = exitPool
		return nil
	}
}

func WithSlashingPool(slashingPool slashings.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.slashingPool = slashingPool
		return nil
	}
}

func WithSyncCommsPool(syncCommsPool synccommittee.Pool) Option {
	return func(s *Service) error {
		s.cfg.syncCommsPool = syncCommsPool
		return nil
	}
}

func WithBlsToExecPool(blsToExecPool blstoexec.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.blsToExecPool = blsToExecPool
		return nil
	}
}

func WithChainService(chain blockchainService) Option {
	return func(s *Service) error {
		s.cfg.chain = chain
		return nil
	}
}

func WithInitialSync(initialSync Checker) Option {
	return func(s *Service) error {
		s.cfg.initialSync = initialSync
		return nil
	}
}

func WithBlockNotifier(blockNotifier blockfeed.Notifier) Option {
	return func(s *Service) error {
		s.cfg.blockNotifier = blockNotifier
		return nil
	}
}

func WithOperationNotifier(operationNotifier operation.Notifier) Option {
	return func(s *Service) error {
		s.cfg.operationNotifier = operationNotifier
		return nil
	}
}

func WithStateGen(stateGen *stategen.State) Option {
	return func(s *Service) error {
		s.cfg.stateGen = stateGen
		return nil
	}
}

func WithSlasherAttestationsFeed(slasherAttestationsFeed *event.Feed) Option {
	return func(s *Service) error {
		s.cfg.slasherAttestationsFeed = slasherAttestationsFeed
		return nil
	}
}

func WithSlasherBlockHeadersFeed(slasherBlockHeadersFeed *event.Feed) Option {
	return func(s *Service) error {
		s.cfg.slasherBlockHeadersFeed = slasherBlockHeadersFeed
		return nil
	}
}

func WithReconstructor(r execution.Reconstructor) Option {
	return func(s *Service) error {
		s.cfg.executionReconstructor = r
		return nil
	}
}

func WithClockWaiter(cw startup.ClockWaiter) Option {
	return func(s *Service) error {
		s.clockWaiter = cw
		return nil
	}
}

func WithInitialSyncComplete(c chan struct{}) Option {
	return func(s *Service) error {
		s.initialSyncComplete = c
		return nil
	}
}

// WithStateNotifier to notify an event feed of state processing.
func WithStateNotifier(n statefeed.Notifier) Option {
	return func(s *Service) error {
		s.cfg.stateNotifier = n
		return nil
	}
}

// WithBlobStorage gives the sync package direct access to BlobStorage.
func WithBlobStorage(b *filesystem.BlobStorage) Option {
	return func(s *Service) error {
		s.cfg.blobStorage = b
		return nil
	}
}

// WithDataColumnStorage gives the sync package direct access to DataColumnStorage.
func WithDataColumnStorage(b *filesystem.DataColumnStorage) Option {
	return func(s *Service) error {
		s.cfg.dataColumnStorage = b
		return nil
	}
}

// WithVerifierWaiter gives the sync package direct access to the verifier waiter.
func WithVerifierWaiter(v *verification.InitializerWaiter) Option {
	return func(s *Service) error {
		s.verifierWaiter = v
		return nil
	}
}

// WithAvailableBlocker allows the sync package to access the current
// status of backfill.
func WithAvailableBlocker(avb coverage.AvailableBlocker) Option {
	return func(s *Service) error {
		s.availableBlocker = avb
		return nil
	}
}

// WithTrackedValidatorsCache for tracked validators cache.
func WithTrackedValidatorsCache(c *cache.TrackedValidatorsCache) Option {
	return func(s *Service) error {
		s.trackedValidatorsCache = c
		return nil
	}
}

// WithSlasherEnabled configures the sync package to support slashing detection.
func WithSlasherEnabled(enabled bool) Option {
	return func(s *Service) error {
		s.slasherEnabled = enabled
		return nil
	}
}

// WithLightClientStore allows the sync package to access light client data.
func WithLightClientStore(lcs *lightClient.Store) Option {
	return func(s *Service) error {
		s.lcStore = lcs
		return nil
	}
}

// WithBatchVerifierLimit sets the maximum number of signatures to batch verify at once.
func WithBatchVerifierLimit(limit int) Option {
	return func(s *Service) error {
		s.cfg.batchVerifierLimit = limit
		return nil
	}
}

// WithReconstructionRandGen sets the random generator for reconstruction delays.
func WithReconstructionRandGen(rg *rand.Rand) Option {
	return func(s *Service) error {
		s.reconstructionRandGen = rg
		return nil
	}
}
