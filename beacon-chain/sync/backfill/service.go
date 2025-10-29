package backfill

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/proto/dbval"
	"github.com/OffchainLabs/prysm/v6/runtime"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Service struct {
	ctx             context.Context
	enabled         bool // service is disabled by default
	clock           *startup.Clock
	store           *Store
	ms              minimumSlotter
	cw              startup.ClockWaiter
	verifierWaiter  InitializerWaiter
	newBlobVerifier verification.NewBlobVerifier
	nWorkers        int
	batchSeq        *batchSequencer
	batchSize       uint64
	pool            batchWorkerPool
	verifier        *verifier
	ctxMap          sync.ContextByteVersions
	p2p             p2p.P2P
	pa              PeerAssigner
	batchImporter   batchImporter
	blobStore       *filesystem.BlobStorage
	initSyncWaiter  func() error
	complete        chan struct{}
}

var _ runtime.Service = (*Service)(nil)

// PeerAssigner describes a type that provides an Assign method, which can assign the best peer
// to service an RPC blockRequest. The Assign method takes a map of peers that should be excluded,
// allowing the caller to avoid making multiple concurrent requests to the same peer.
type PeerAssigner interface {
	Assign(busy map[peer.ID]bool, n int) ([]peer.ID, error)
}

type minimumSlotter func(primitives.Slot) primitives.Slot
type batchImporter func(ctx context.Context, current primitives.Slot, b batch, su *Store) (*dbval.BackfillStatus, error)

func defaultBatchImporter(ctx context.Context, current primitives.Slot, b batch, su *Store) (*dbval.BackfillStatus, error) {
	status := su.status()
	if err := b.ensureParent(bytesutil.ToBytes32(status.LowParentRoot)); err != nil {
		return status, err
	}
	// Import blocks to db and update db state to reflect the newly imported blocks.
	// Other parts of the beacon node may use the same StatusUpdater instance
	// via the coverage.AvailableBlocker interface to safely determine if a given slot has been backfilled.
	return su.fillBack(ctx, current, b.results, b.availabilityStore())
}

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

// WithMinimumSlot allows the user to specify a different backfill minimum slot than the spec default of current - MIN_EPOCHS_FOR_BLOCK_REQUESTS.
// If this value is greater than current - MIN_EPOCHS_FOR_BLOCK_REQUESTS, it will be ignored with a warning log.
func WithMinimumSlot(s primitives.Slot) ServiceOption {
	ms := func(current primitives.Slot) primitives.Slot {
		specMin := minimumBackfillSlot(current)
		if s < specMin {
			return s
		}
		log.WithField("userSlot", s).WithField("specMinSlot", specMin).
			Warn("Ignoring user-specified slot > MIN_EPOCHS_FOR_BLOCK_REQUESTS.")
		return specMin
	}
	return func(s *Service) error {
		s.ms = ms
		return nil
	}
}

// NewService initializes the backfill Service. Like all implementations of the Service interface,
// the service won't begin its runloop until Start() is called.
func NewService(ctx context.Context, su *Store, bStore *filesystem.BlobStorage, cw startup.ClockWaiter, p p2p.P2P, pa PeerAssigner, opts ...ServiceOption) (*Service, error) {
	s := &Service{
		ctx:           ctx,
		store:         su,
		blobStore:     bStore,
		cw:            cw,
		ms:            minimumBackfillSlot,
		p2p:           p,
		pa:            pa,
		batchImporter: defaultBatchImporter,
		complete:      make(chan struct{}),
	}
	for _, o := range opts {
		if err := o(s); err != nil {
			return nil, err
		}
	}
	s.pool = newP2PBatchWorkerPool(p, s.nWorkers)

	return s, nil
}

func (s *Service) initVerifier(ctx context.Context) (*verifier, sync.ContextByteVersions, error) {
	cps, err := s.store.originState(ctx)
	if err != nil {
		return nil, nil, err
	}
	keys, err := cps.PublicKeys()
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to retrieve public keys for all validators in the origin state")
	}
	vr := cps.GenesisValidatorsRoot()
	ctxMap, err := sync.ContextByteVersionsForValRoot(bytesutil.ToBytes32(vr))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to initialize context version map using genesis validator root %#x", vr)
	}
	v, err := newBackfillVerifier(vr, keys)
	return v, ctxMap, err
}

func (s *Service) updateComplete() bool {
	b, err := s.pool.complete()
	if err != nil {
		if errors.Is(err, errEndSequence) {
			log.WithField("backfillSlot", b.begin).Info("Backfill is complete")
			return true
		}
		log.WithError(err).Error("Backfill service received unhandled error from worker pool")
		return true
	}
	s.batchSeq.update(b)
	return false
}

func (s *Service) importBatches(ctx context.Context) {
	importable := s.batchSeq.importable()
	imported := 0
	defer func() {
		if imported == 0 {
			return
		}
		backfillBatchesImported.Add(float64(imported))
	}()
	current := s.clock.CurrentSlot()
	for i := range importable {
		ib := importable[i]
		if len(ib.results) == 0 {
			log.WithFields(ib.logFields()).Error("Batch with no results, skipping importer")
		}
		_, err := s.batchImporter(ctx, current, ib, s.store)
		if err != nil {
			log.WithError(err).WithFields(ib.logFields()).Debug("Backfill batch failed to import")
			s.downscorePeer(ib.blockPid, "backfillBatchImportError")
			s.batchSeq.update(ib.withState(batchErrRetryable))
			// If a batch fails, the subsequent batches are no longer considered importable.
			break
		}
		s.batchSeq.update(ib.withState(batchImportComplete))
		imported += 1
		// Calling update with state=batchImportComplete will advance the batch list.
	}

	nt := s.batchSeq.numTodo()
	log.WithField("imported", imported).WithField("importable", len(importable)).
		WithField("batchesRemaining", nt).
		Info("Backfill batches processed")

	backfillRemainingBatches.Set(float64(nt))
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
			log.Debug("Backfill batches waiting for descendent batch to complete")
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
		log.Info("Backfill service not enabled")
		s.markComplete()
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	defer func() {
		log.Info("Backfill service is shutting down")
		cancel()
	}()
	clock, err := s.cw.WaitForClock(ctx)
	if err != nil {
		log.WithError(err).Error("Backfill service failed to start while waiting for genesis data")
		return
	}
	s.clock = clock
	v, err := s.verifierWaiter.WaitForInitializer(ctx)
	s.newBlobVerifier = newBlobVerifierFromInitializer(v)

	if err != nil {
		log.WithError(err).Error("Could not initialize blob verifier in backfill service")
		return
	}

	if s.store.isGenesisSync() {
		log.Info("Backfill short-circuit; node synced from genesis")
		s.markComplete()
		return
	}
	status := s.store.status()
	// Exit early if there aren't going to be any batches to backfill.
	if primitives.Slot(status.LowSlot) <= s.ms(s.clock.CurrentSlot()) {
		log.WithField("minimumRequiredSlot", s.ms(s.clock.CurrentSlot())).
			WithField("backfillLowestSlot", status.LowSlot).
			Info("Exiting backfill service; minimum block retention slot > lowest backfilled block")
		s.markComplete()
		return
	}
	s.verifier, s.ctxMap, err = s.initVerifier(ctx)
	if err != nil {
		log.WithError(err).Error("Unable to initialize backfill verifier")
		return
	}

	if s.initSyncWaiter != nil {
		log.Info("Backfill service waiting for initial-sync to reach head before starting")
		if err := s.initSyncWaiter(); err != nil {
			log.WithError(err).Error("Error waiting for init-sync to complete")
			return
		}
	}
	s.pool.spawn(ctx, s.nWorkers, clock, s.pa, s.verifier, s.ctxMap, s.newBlobVerifier, s.blobStore)
	s.batchSeq = newBatchSequencer(s.nWorkers, s.ms(s.clock.CurrentSlot()), primitives.Slot(status.LowSlot), primitives.Slot(s.batchSize))
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
		if err := s.batchSeq.moveMinimum(s.ms(s.clock.CurrentSlot())); err != nil {
			log.WithError(err).Error("Non-recoverable error while adjusting backfill minimum slot")
		}
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

// minimumBackfillSlot determines the lowest slot that backfill needs to download based on looking back
// MIN_EPOCHS_FOR_BLOCK_REQUESTS from the current slot.
func minimumBackfillSlot(current primitives.Slot) primitives.Slot {
	oe := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests)
	if oe > slots.MaxSafeEpoch() {
		oe = slots.MaxSafeEpoch()
	}
	offset := slots.UnsafeEpochStart(oe)
	if offset >= current {
		// Slot 0 is the genesis block, therefore the signature in it is invalid.
		// To prevent us from rejecting a batch, we restrict the minimum backfill batch till only slot 1
		return 1
	}
	return current - offset
}

func newBlobVerifierFromInitializer(ini *verification.Initializer) verification.NewBlobVerifier {
	return func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
		return ini.NewBlobVerifier(b, reqs)
	}
}

func (s *Service) markComplete() {
	close(s.complete)
	log.Info("Backfill service marked as complete")
}

func (s *Service) WaitForCompletion() error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-s.complete:
		return nil
	}
}

func (s *Service) downscorePeer(peerID peer.ID, reason string) {
	newScore := s.p2p.Peers().Scorers().BadResponsesScorer().Increment(peerID)
	log.WithFields(logrus.Fields{"peerID": peerID, "reason": reason, "newScore": newScore}).Debug("Downscore peer")
}
