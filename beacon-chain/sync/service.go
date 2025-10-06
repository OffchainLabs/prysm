// Package sync includes all chain-synchronization logic for the beacon node,
// including gossip-sub validators for blocks, attestations, and other p2p
// messages, as well as ability to process and respond to block requests
// by peers.
package sync

import (
	"context"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/async"
	"github.com/OffchainLabs/prysm/v6/async/abool"
	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
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
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync/backfill/coverage"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	lruwrpr "github.com/OffchainLabs/prysm/v6/cache/lru"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	leakybucket "github.com/OffchainLabs/prysm/v6/container/leaky-bucket"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	"github.com/OffchainLabs/prysm/v6/runtime"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	gcache "github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/trailofbits/go-mutexasserts"
	"golang.org/x/sync/singleflight"
)

var _ runtime.Service = (*Service)(nil)

const (
	rangeLimit               uint64 = 1024
	seenBlockSize                   = 1000
	seenDataColumnSize              = seenBlockSize * 128 // Each block can have max 128 data columns.
	seenUnaggregatedAttSize         = 20000
	seenAggregatedAttSize           = 16384
	seenSyncMsgSize                 = 1000 // Maximum of 512 sync committee members, 1000 is a safe amount.
	seenSyncContributionSize        = 512  // Maximum of SYNC_COMMITTEE_SIZE as specified by the spec.
	seenExitSize                    = 100
	seenProposerSlashingSize        = 100
	badBlockSize                    = 1000
	syncMetricsInterval             = 10 * time.Second
)

var (
	// Seconds in one epoch.
	pendingBlockExpTime = time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot)) * time.Second
	// time to allow processing early blocks.
	earlyBlockProcessingTolerance = slots.MultiplySlotBy(2)
	// time to allow processing early attestations.
	earlyAttestationProcessingTolerance = params.BeaconConfig().MaximumGossipClockDisparityDuration()
	errWrongMessage                     = errors.New("wrong pubsub message")
	errNilMessage                       = errors.New("nil pubsub message")
)

// Common type for functional p2p validation options.
type validationFn func(ctx context.Context) (pubsub.ValidationResult, error)

// config to hold dependencies for the sync service.
type config struct {
	attestationNotifier     operation.Notifier
	p2p                     p2p.P2P
	beaconDB                db.NoHeadAccessDatabase
	attestationCache        *cache.AttestationCache
	attPool                 attestations.Pool
	exitPool                voluntaryexits.PoolManager
	slashingPool            slashings.PoolManager
	syncCommsPool           synccommittee.Pool
	blsToExecPool           blstoexec.PoolManager
	chain                   blockchainService
	initialSync             Checker
	blockNotifier           blockfeed.Notifier
	operationNotifier       operation.Notifier
	executionReconstructor  execution.Reconstructor
	stateGen                *stategen.State
	slasherAttestationsFeed *event.Feed
	slasherBlockHeadersFeed *event.Feed
	clock                   *startup.Clock
	stateNotifier           statefeed.Notifier
	blobStorage             *filesystem.BlobStorage
	dataColumnStorage       *filesystem.DataColumnStorage
	batchVerifierLimit      int
}

// This defines the interface for interacting with block chain service
type blockchainService interface {
	blockchain.BlockReceiver
	blockchain.BlobReceiver
	blockchain.DataColumnReceiver
	blockchain.HeadFetcher
	blockchain.FinalizationFetcher
	blockchain.ForkFetcher
	blockchain.AttestationReceiver
	blockchain.TimeFetcher
	blockchain.GenesisFetcher
	blockchain.CanonicalFetcher
	blockchain.OptimisticModeFetcher
	blockchain.SlashingReceiver
	blockchain.ForkchoiceFetcher
}

// Service is responsible for handling all run time p2p related operations as the
// main entry point for network messages.
type Service struct {
	cfg                              *config
	ctx                              context.Context
	cancel                           context.CancelFunc
	slotToPendingBlocks              *gcache.Cache
	seenPendingBlocks                map[[32]byte]bool
	blkRootToPendingAtts             map[[32]byte][]any
	subHandler                       *subTopicHandler
	pendingAttsLock                  sync.RWMutex
	pendingQueueLock                 sync.RWMutex
	chainStarted                     *abool.AtomicBool
	validateBlockLock                sync.RWMutex
	rateLimiter                      *limiter
	seenBlockLock                    sync.RWMutex
	seenBlockCache                   *lru.Cache
	seenBlobLock                     sync.RWMutex
	seenBlobCache                    *lru.Cache
	seenDataColumnCache              *slotAwareCache
	seenAggregatedAttestationLock    sync.RWMutex
	seenAggregatedAttestationCache   *lru.Cache
	seenUnAggregatedAttestationLock  sync.RWMutex
	seenUnAggregatedAttestationCache *lru.Cache
	seenExitLock                     sync.RWMutex
	seenExitCache                    *lru.Cache
	seenProposerSlashingLock         sync.RWMutex
	seenProposerSlashingCache        *lru.Cache
	seenAttesterSlashingLock         sync.RWMutex
	seenAttesterSlashingCache        map[uint64]bool
	seenSyncMessageLock              sync.RWMutex
	seenSyncMessageCache             *lru.Cache
	seenSyncContributionLock         sync.RWMutex
	seenSyncContributionCache        *lru.Cache
	badBlockCache                    *lru.Cache
	badBlockLock                     sync.RWMutex
	syncContributionBitsOverlapLock  sync.RWMutex
	syncContributionBitsOverlapCache *lru.Cache
	signatureChan                    chan *signatureVerifier
	kzgChan                          chan *kzgVerifier
	clockWaiter                      startup.ClockWaiter
	initialSyncComplete              chan struct{}
	verifierWaiter                   *verification.InitializerWaiter
	newBlobVerifier                  verification.NewBlobVerifier
	newColumnsVerifier               verification.NewDataColumnsVerifier
	columnSidecarsExecSingleFlight   singleflight.Group
	reconstructionSingleFlight       singleflight.Group
	availableBlocker                 coverage.AvailableBlocker
	reconstructionRandGen            *rand.Rand
	trackedValidatorsCache           *cache.TrackedValidatorsCache
	ctxMap                           ContextByteVersions
	slasherEnabled                   bool
	lcStore                          *lightClient.Store
	dataColumnLogCh                  chan dataColumnLogEntry
	digestActions                    perDigestSet
	subscriptionSpawner              func(func()) // see Service.spawn for details
}

// NewService initializes new regular sync service.
func NewService(ctx context.Context, opts ...Option) *Service {
	ctx, cancel := context.WithCancel(ctx)
	r := &Service{
		ctx:                   ctx,
		cancel:                cancel,
		chainStarted:          abool.New(),
		cfg:                   &config{clock: startup.NewClock(time.Unix(0, 0), [32]byte{})},
		slotToPendingBlocks:   gcache.New(pendingBlockExpTime /* exp time */, 0 /* disable janitor */),
		seenPendingBlocks:     make(map[[32]byte]bool),
		blkRootToPendingAtts:  make(map[[32]byte][]any),
		dataColumnLogCh:       make(chan dataColumnLogEntry, 1000),
		reconstructionRandGen: rand.NewGenerator(),
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil
		}
	}
	// Initialize signature channel with configured limit
	r.signatureChan = make(chan *signatureVerifier, r.cfg.batchVerifierLimit)
	// Initialize KZG channel with fixed buffer size of 100.
	// This buffer size is designed to handle burst traffic of data column gossip messages:
	// - Data columns arrive less frequently than attestations (default batchVerifierLimit=1000)
	r.kzgChan = make(chan *kzgVerifier, 100)
	// Correctly remove it from our seen pending block map.
	// The eviction method always assumes that the mutex is held.
	r.slotToPendingBlocks.OnEvicted(func(s string, i interface{}) {
		if !mutexasserts.RWMutexLocked(&r.pendingQueueLock) {
			log.Errorf("Mutex is not locked during cache eviction of values")
			// Continue on to allow elements to be properly removed.
		}
		blks, ok := i.([]interfaces.ReadOnlySignedBeaconBlock)
		if !ok {
			log.Errorf("Invalid type retrieved from the cache: %T", i)
			return
		}

		for _, b := range blks {
			root, err := b.Block().HashTreeRoot()
			if err != nil {
				log.WithError(err).Error("Could not calculate htr of block")
				continue
			}
			delete(r.seenPendingBlocks, root)
		}
	})
	r.subHandler = newSubTopicHandler()
	r.rateLimiter = newRateLimiter(r.cfg.p2p)
	r.initCaches()

	return r
}

func newBlobVerifierFromInitializer(ini *verification.Initializer) verification.NewBlobVerifier {
	return func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
		return ini.NewBlobVerifier(b, reqs)
	}
}

func newDataColumnsVerifierFromInitializer(ini *verification.Initializer) verification.NewDataColumnsVerifier {
	return func(roDataColumns []blocks.RODataColumn, reqs []verification.Requirement) verification.DataColumnsVerifier {
		return ini.NewDataColumnsVerifier(roDataColumns, reqs)
	}
}

// Start the regular sync service.
func (s *Service) Start() {
	v, err := s.verifierWaiter.WaitForInitializer(s.ctx)
	if err != nil {
		log.WithError(err).Error("Could not get verification initializer")
		return
	}
	s.newBlobVerifier = newBlobVerifierFromInitializer(v)
	s.newColumnsVerifier = newDataColumnsVerifierFromInitializer(v)

	go s.verifierRoutine()
	go s.kzgVerifierRoutine()
	go s.startDiscoveryAndSubscriptions()
	go s.processDataColumnLogs()

	s.cfg.p2p.AddConnectionHandler(s.reValidatePeer, s.sendGoodbye)
	s.cfg.p2p.AddDisconnectionHandler(func(_ context.Context, _ peer.ID) error {
		// no-op
		return nil
	})
	s.cfg.p2p.AddPingMethod(s.sendPingRequest)

	s.processPendingBlocksQueue()
	s.maintainPeerStatuses()

	if params.FuluEnabled() {
		s.maintainCustodyInfo()
	}

	s.resyncIfBehind()

	// Update sync metrics.
	async.RunEvery(s.ctx, syncMetricsInterval, s.updateMetrics)

	// Prune data column cache periodically on finalization.
	async.RunEvery(s.ctx, 30*time.Second, s.pruneDataColumnCache)
}

// Stop the regular sync service.
func (s *Service) Stop() error {
	defer func() {
		s.cancel()

		if s.rateLimiter != nil {
			s.rateLimiter.free()
		}
	}()

	// Create context with timeout to prevent hanging
	goodbyeCtx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	// Use WaitGroup to ensure all goodbye messages complete
	var wg sync.WaitGroup
	for _, peerID := range s.cfg.p2p.Peers().Connected() {
		if s.cfg.p2p.Host().Network().Connectedness(peerID) == network.Connected {
			wg.Add(1)
			go func(pid peer.ID) {
				defer wg.Done()
				if err := s.sendGoodByeAndDisconnect(goodbyeCtx, p2ptypes.GoodbyeCodeClientShutdown, pid); err != nil {
					log.WithError(err).WithField("peerID", pid).Error("Failed to send goodbye message")
				}
			}(peerID)
		}
	}
	wg.Wait()
	log.Debug("All goodbye messages sent successfully")

	// Now safe to remove handlers / unsubscribe.
	for _, p := range s.cfg.p2p.Host().Mux().Protocols() {
		s.cfg.p2p.Host().RemoveStreamHandler(p)
	}
	for _, t := range s.cfg.p2p.PubSub().GetTopics() {
		s.unSubscribeFromTopic(t)
	}
	return nil
}

// Status of the currently running regular sync service.
func (s *Service) Status() error {
	// If our head slot is on a previous epoch and our peers are reporting their head block are
	// in the most recent epoch, then we might be out of sync.
	if headEpoch := slots.ToEpoch(s.cfg.chain.HeadSlot()); headEpoch+1 < slots.ToEpoch(s.cfg.clock.CurrentSlot()) &&
		headEpoch+1 < s.cfg.p2p.Peers().HighestEpoch() {
		return errors.New("out of sync")
	}
	return nil
}

// This initializes the caches to update seen beacon objects coming in from the wire
// and prevent DoS.
func (s *Service) initCaches() {
	s.seenBlockCache = lruwrpr.New(seenBlockSize)
	s.seenBlobCache = lruwrpr.New(seenBlockSize * params.BeaconConfig().DeprecatedMaxBlobsPerBlockElectra)
	s.seenDataColumnCache = newSlotAwareCache(seenDataColumnSize)
	s.seenAggregatedAttestationCache = lruwrpr.New(seenAggregatedAttSize)
	s.seenUnAggregatedAttestationCache = lruwrpr.New(seenUnaggregatedAttSize)
	s.seenSyncMessageCache = lruwrpr.New(seenSyncMsgSize)
	s.seenSyncContributionCache = lruwrpr.New(seenSyncContributionSize)
	s.syncContributionBitsOverlapCache = lruwrpr.New(seenSyncContributionSize)
	s.seenExitCache = lruwrpr.New(seenExitSize)
	s.seenAttesterSlashingCache = make(map[uint64]bool)
	s.seenProposerSlashingCache = lruwrpr.New(seenProposerSlashingSize)
	s.badBlockCache = lruwrpr.New(badBlockSize)
}

func (s *Service) waitForChainStart() {
	clock, err := s.clockWaiter.WaitForClock(s.ctx)
	if err != nil {
		log.WithError(err).Error("Sync service failed to receive genesis data")
		return
	}
	s.cfg.clock = clock
	startTime := clock.GenesisTime()
	log.WithField("startTime", startTime).Debug("Received state initialized event")

	ctxMap, err := ContextByteVersionsForValRoot(clock.GenesisValidatorsRoot())
	if err != nil {
		log.
			WithError(err).
			WithField("genesisValidatorRoot", clock.GenesisValidatorsRoot()).
			Error("Sync service failed to initialize context version map")
		return
	}
	s.ctxMap = ctxMap

	// We need to register RPC handlers ASAP so that we can handle incoming status message
	// requests from peers.
	nse := params.GetNetworkScheduleEntry(clock.CurrentEpoch())
	if err := s.registerRPCHandlers(nse); err != nil {
		// If we fail here, we won't be able to peer with anyone because we can't handle their status messages.
		log.WithError(err).Error("Failed to register RPC handlers")
		// TODO: need ability to bubble the error up to the top of the node init tree and exit safely.
		return
	}

	// Wait for chainstart in separate routine.
	if startTime.After(prysmTime.Now()) {
		time.Sleep(prysmTime.Until(startTime))
	}
	log.WithField("startTime", startTime).Debug("Chain started in sync service")
	s.markForChainStart()
}

func (s *Service) startDiscoveryAndSubscriptions() {
	// Wait for the chain to start.
	s.waitForChainStart()

	if s.ctx.Err() != nil {
		log.Debug("Context closed, exiting StartDiscoveryAndSubscription")
		return
	}

	// Start the fork watcher.
	go s.p2pHandlerControlLoop()
}

func (s *Service) writeErrorResponseToStream(responseCode byte, reason string, stream libp2pcore.Stream) {
	writeErrorResponseToStream(responseCode, reason, stream, s.cfg.p2p)
}

func (s *Service) setRateCollector(topic string, c *leakybucket.Collector) {
	s.rateLimiter.limiterMap[topic] = c
}

// marks the chain as having started.
func (s *Service) markForChainStart() {
	s.chainStarted.Set()
}

// pruneDataColumnCache removes entries from the data column cache that are older than the finalized slot.
func (s *Service) pruneDataColumnCache() {
	finalizedCheckpoint := s.cfg.chain.FinalizedCheckpt()
	finalizedSlot, err := slots.EpochStart(finalizedCheckpoint.Epoch)
	if err != nil {
		log.WithError(err).Error("Could not calculate finalized slot for cache pruning")
		return
	}

	pruned := s.seenDataColumnCache.pruneSlotsBefore(finalizedSlot)
	if pruned > 0 {
		log.WithFields(logrus.Fields{
			"finalizedSlot": finalizedSlot,
			"prunedEntries": pruned,
		}).Debug("Pruned data column cache entries before finalized slot")
	}
}

func (s *Service) chainIsStarted() bool {
	return s.chainStarted.IsSet()
}

func (s *Service) waitForInitialSync(ctx context.Context) error {
	select {
	case <-s.initialSyncComplete:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Checker defines a struct which can verify whether a node is currently
// synchronizing a chain with the rest of peers in the network.
type Checker interface {
	Initialized() bool
	Syncing() bool
	Synced() bool
	Status() error
	Resync() error
}
