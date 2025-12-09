package backfill

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	"github.com/OffchainLabs/prysm/v7/runtime"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Service struct {
	ctx             context.Context
	enabled         bool // service is disabled by default
	clock           *startup.Clock
	store           *Store
	syncNeeds       das.SyncNeeds
	syncNeedsWaiter func() (das.SyncNeeds, error)
	ms              minimumSlotter
	cw              startup.ClockWaiter
	verifierWaiter  InitializerWaiter
	nWorkers        int
	batchSeq        *batchSequencer
	batchSize       uint64
	pool            batchWorkerPool
	p2p             p2p.P2P
	pa              PeerAssigner
	batchImporter   batchImporter
	blobStore       *filesystem.BlobStorage
	dcStore         *filesystem.DataColumnStorage
	initSyncWaiter  func() error
	complete        chan struct{}
	workerCfg       *workerCfg
	fuluStart       primitives.Slot
	denebStart      primitives.Slot
}

var _ runtime.Service = (*Service)(nil)

// PeerAssigner describes a type that provides an Assign method, which can assign the best peer
// to service an RPC blockRequest. The Assign method takes a callback used to filter out peers,
// allowing the caller to avoid making multiple concurrent requests to the same peer.
type PeerAssigner interface {
	Assign(filter peers.AssignmentFilter) ([]peer.ID, error)
}

type minimumSlotter func(primitives.Slot) primitives.Slot
type batchImporter func(ctx context.Context, current primitives.Slot, b batch, su *Store) (*dbval.BackfillStatus, error)

// ServiceOption represents a functional option for the backfill service constructor.
type ServiceOption func(*Service) error

// WithEnableBackfill toggles the entire backfill service on or off, intended to be used by a feature flag.
func WithEnableBackfill(enabled bool) ServiceOption {
	return func(s *Service) error {
		s.enabled = enabled
		return nil
	}
}

// WithWorkerCount sets the number of goroutines in the batch processing pool that can concurrently
// make p2p requests to download data for batches.
func WithWorkerCount(n int) ServiceOption {
	return func(s *Service) error {
		s.nWorkers = n
		return nil
	}
}

// WithBatchSize configures the size of backfill batches, similar to the initial-sync block-batch-limit flag.
// It should usually be left at the default value.
func WithBatchSize(n uint64) ServiceOption {
	return func(s *Service) error {
		s.batchSize = n
		return nil
	}
}

// WithInitSyncWaiter sets a function on the service which will block until init-sync
// completes for the first time, or returns an error if context is canceled.
func WithInitSyncWaiter(w func() error) ServiceOption {
	return func(s *Service) error {
		s.initSyncWaiter = w
		return nil
	}
}

// InitializerWaiter is an interface that is satisfied by verification.InitializerWaiter.
// Using this interface enables node init to satisfy this requirement for the backfill service
// while also allowing backfill to mock it in tests.
type InitializerWaiter interface {
	WaitForInitializer(ctx context.Context) (*verification.Initializer, error)
}

// WithVerifierWaiter sets the verification.InitializerWaiter
// for the backfill Service.
func WithVerifierWaiter(viw InitializerWaiter) ServiceOption {
	return func(s *Service) error {
		s.verifierWaiter = viw
		return nil
	}
}

func WithSyncNeedsWaiter(f func() (das.SyncNeeds, error)) ServiceOption {
	return func(s *Service) error {
		if f != nil {
			s.syncNeedsWaiter = f
		}
		return nil
	}
}

// NewService initializes the backfill Service. Like all implementations of the Service interface,
// the service won't begin its runloop until Start() is called.
func NewService(ctx context.Context, su *Store, bStore *filesystem.BlobStorage, dcStore *filesystem.DataColumnStorage, cw startup.ClockWaiter, p p2p.P2P, pa PeerAssigner, opts ...ServiceOption) (*Service, error) {
	s := &Service{
		ctx:        ctx,
		store:      su,
		blobStore:  bStore,
		dcStore:    dcStore,
		cw:         cw,
		p2p:        p,
		pa:         pa,
		complete:   make(chan struct{}),
		fuluStart:  slots.SafeEpochStartOrMax(params.BeaconConfig().FuluForkEpoch),
		denebStart: slots.SafeEpochStartOrMax(params.BeaconConfig().DenebForkEpoch),
	}

	s.batchImporter = s.defaultBatchImporter
	for _, o := range opts {
		if err := o(s); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *Service) updateComplete() bool {
	b, err := s.pool.complete()
	if err != nil {
		if errors.Is(err, errEndSequence) {
			log.WithField("backfillSlot", b.begin).Info("Backfill is complete")
			return true
		}
		log.WithError(err).Error("Service received unhandled error from worker pool")
		return true
	}
	s.batchSeq.update(b)
	return false
}

func (s *Service) importBatches(ctx context.Context) {
	current := s.clock.CurrentSlot()
	imported := 0
	importable := s.batchSeq.importable()
	for _, ib := range importable {
		if len(ib.blocks) == 0 {
			log.WithFields(ib.logFields()).Error("Batch with no results, skipping importer")
			s.batchSeq.update(ib.withError(errors.New("batch has no blocks")))
			// This batch needs to be retried before we can continue importing subsequent batches.
			break
		}
		_, err := s.batchImporter(ctx, current, ib, s.store)
		if err != nil {
			log.WithError(err).WithFields(ib.logFields()).Debug("Backfill batch failed to import")
			s.batchSeq.update(ib.withError(err))
			// If a batch fails, the subsequent batches are no longer considered importable.
			break
		}
		// Calling update with state=batchImportComplete will advance the batch list.
		s.batchSeq.update(ib.withState(batchImportComplete))
		imported += 1
		log.WithFields(ib.logFields()).WithField("batchesRemaining", s.batchSeq.numTodo()).Debug("Imported batch")
	}

	nt := s.batchSeq.numTodo()
	batchesRemaining.Set(float64(nt))
	if imported > 0 {
		batchesImported.Add(float64(imported))
	}
}

func (s *Service) defaultBatchImporter(ctx context.Context, current primitives.Slot, b batch, su *Store) (*dbval.BackfillStatus, error) {
	status := su.status()
	if err := b.ensureParent(bytesutil.ToBytes32(status.LowParentRoot)); err != nil {
		return status, err
	}
	// Import blocks to db and update db state to reflect the newly imported blocks.
	// Other parts of the beacon node may use the same StatusUpdater instance
	// via the coverage.AvailableBlocker interface to safely determine if a given slot has been backfilled.

	checker := newCheckMultiplexer(s.syncNeeds.Currently(), b)
	return su.fillBack(ctx, current, b.blocks, checker)
}

func (s *Service) scheduleTodos() {
	batches, err := s.batchSeq.sequence()
	if err != nil {
		// This typically means we have several importable batches, but they are stuck behind a batch that needs
		// to complete first so that we can chain parent roots across batches.
		// ie backfilling [[90..100), [80..90), [70..80)], if we complete [70..80) and [80..90) but not [90..100),
		// we can't move forward until [90..100) completes, because we need to confirm 99 connects to 100,
		// and then we'll have the parent_root expected by 90 to ensure it matches the root for 89,
		// at which point we know we can process [80..90).
		if errors.Is(err, errMaxBatches) {
			log.Debug("Waiting for descendent batch to complete")
			return
		}
	}
	for _, b := range batches {
		s.pool.todo(b)
	}
}

// Start begins the runloop of backfill.Service in the current goroutine.
func (s *Service) Start() {
	if !s.enabled {
		log.Info("Service not enabled")
		s.markComplete()
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	defer func() {
		log.Info("Service is shutting down")
		cancel()
	}()

	if s.store.isGenesisSync() {
		log.Info("Node synced from genesis, shutting down backfill")
		s.markComplete()
		return
	}

	clock, err := s.cw.WaitForClock(ctx)
	if err != nil {
		log.WithError(err).Error("Service failed to start while waiting for genesis data")
		return
	}
	s.clock = clock

	if s.syncNeedsWaiter == nil {
		log.Error("Service missing sync needs waiter; cannot start")
		return
	}
	syncNeeds, err := s.syncNeedsWaiter()
	if err != nil {
		log.WithError(err).Error("Service failed to start while waiting for sync needs")
		return
	}
	s.syncNeeds = syncNeeds

	status := s.store.status()
	needs := s.syncNeeds.Currently()
	// Exit early if there aren't going to be any batches to backfill.
	if !needs.Block.At(primitives.Slot(status.LowSlot)) {
		log.WithField("minimumSlot", needs.Block.Begin).
			WithField("backfillLowestSlot", status.LowSlot).
			Info("Exiting backfill service; minimum block retention slot > lowest backfilled block")
		s.markComplete()
		return
	}

	if s.initSyncWaiter != nil {
		log.Info("Service waiting for initial-sync to reach head before starting")
		if err := s.initSyncWaiter(); err != nil {
			log.WithError(err).Error("Error waiting for init-sync to complete")
			return
		}
	}

	if s.workerCfg == nil {
		s.workerCfg = &workerCfg{
			clock:        s.clock,
			blobStore:    s.blobStore,
			colStore:     s.dcStore,
			downscore:    s.downscorePeer,
			currentNeeds: s.syncNeeds.Currently,
		}

		if err = initWorkerCfg(ctx, s.workerCfg, s.verifierWaiter, s.store); err != nil {
			log.WithError(err).Error("Could not initialize blob verifier in backfill service")
			return
		}
	}

	// Allow tests to inject a mock pool.
	if s.pool == nil {
		s.pool = newP2PBatchWorkerPool(s.p2p, s.nWorkers, s.syncNeeds.Currently)
	}
	s.pool.spawn(ctx, s.nWorkers, s.pa, s.workerCfg)
	s.batchSeq = newBatchSequencer(s.nWorkers, primitives.Slot(status.LowSlot), primitives.Slot(s.batchSize), s.syncNeeds.Currently)
	if err = s.initBatches(); err != nil {
		log.WithError(err).Error("Non-recoverable error in backfill service")
		return
	}

	for {
		if ctx.Err() != nil {
			return
		}
		if s.updateComplete() {
			s.markComplete()
			return
		}
		s.importBatches(ctx)
		batchesWaiting.Set(float64(s.batchSeq.countWithState(batchImportable)))
		s.scheduleTodos()
	}
}

func (s *Service) initBatches() error {
	batches, err := s.batchSeq.sequence()
	if err != nil {
		return err
	}
	for _, b := range batches {
		s.pool.todo(b)
	}
	return nil
}

func (*Service) Stop() error {
	return nil
}

func (*Service) Status() error {
	return nil
}

// syncEpochOffset subtracts a number of epochs as slots from the current slot, with underflow checks.
// It returns slot 1 if the result would be 0 or underflow. It doesn't return slot 0 because the
// genesis block needs to be specially synced (it doesn't have a valid signature).
func syncEpochOffset(current primitives.Slot, subtract primitives.Epoch) primitives.Slot {
	minEpoch := min(subtract, slots.MaxSafeEpoch())
	// compute slot offset - offset is a number of slots to go back from current (not an absolute slot).
	offset := slots.UnsafeEpochStart(minEpoch)
	// Undeflow protection: slot 0 is the genesis block, therefore the signature in it is invalid.
	// To prevent us from rejecting a batch, we restrict the minimum backfill batch till only slot 1
	if offset >= current {
		return 1
	}
	return current - offset
}

func newBlobVerifierFromInitializer(ini *verification.Initializer) verification.NewBlobVerifier {
	return func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
		return ini.NewBlobVerifier(b, reqs)
	}
}

func newDataColumnVerifierFromInitializer(ini *verification.Initializer) verification.NewDataColumnsVerifier {
	return func(cols []blocks.RODataColumn, reqs []verification.Requirement) verification.DataColumnsVerifier {
		return ini.NewDataColumnsVerifier(cols, reqs)
	}
}

func (s *Service) markComplete() {
	close(s.complete)
	log.Info("Marked as complete")
}

func (s *Service) WaitForCompletion() error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-s.complete:
		return nil
	}
}

func (s *Service) downscorePeer(peerID peer.ID, reason string, err error) {
	newScore := s.p2p.Peers().Scorers().BadResponsesScorer().Increment(peerID)
	logArgs := log.WithFields(logrus.Fields{"peerID": peerID, "reason": reason, "newScore": newScore})
	if err != nil {
		logArgs = logArgs.WithError(err)
	}
	logArgs.Debug("Downscore peer")
}
