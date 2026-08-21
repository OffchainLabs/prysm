package coverage

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	dbiface "github.com/OffchainLabs/prysm/v7/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/runtime"
	"github.com/pkg/errors"
)

var _ runtime.Service = (*Service)(nil)

const (
	defaultSeedPageCandidates = 32
	defaultPageCandidates     = 2048
	defaultPageBytes          = defaultPageCandidates * 32
	defaultPruneBudget        = 4096
	defaultInterPageYield     = 10 * time.Millisecond
)

// Service is the standalone node-owned envelope coverage runtime. Its
// coordinator goroutine is the only writer of coverage and of the
// canonical-revealed serving index; producers notify it through the
// never-closed coalescing Notifier and the state feed, and the by-range RPC
// handler consumes CoherentServeRead snapshots. It runs whenever Gloas is
// scheduled, independent of backfill configuration, checkpoint origin, or
// genesis sync.
type Service struct {
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once

	db            Database
	store         *store
	notifier      *Notifier
	stateNotifier statefeed.Notifier
	clockWaiter   startup.ClockWaiter
	chain         atomic.Value // ChainView
	clock         *startup.Clock

	seedPageCandidates int
	pageCandidates     int
	pageBytes          int
	pruneBudget        int
	interPageYield     time.Duration
	tickInterval       time.Duration

	started        time.Time
	firstPublished bool
	reachedFloor   bool

	statusMu  sync.Mutex
	statusErr error
}

// Option configures the coverage runtime.
type Option func(*Service) error

// WithDatabase sets the runtime's database dependency.
func WithDatabase(d Database) Option {
	return func(s *Service) error {
		s.db = d
		return nil
	}
}

// WithClockWaiter lets the runtime wait for the startup clock, which the
// blockchain service publishes only after origin/forkchoice/head setup.
func WithClockWaiter(cw startup.ClockWaiter) Option {
	return func(s *Service) error {
		s.clockWaiter = cw
		return nil
	}
}

// WithStateNotifier subscribes the runtime to state feed events
// (ExecutionPayloadProcessed as the authoritative live durable-save hook,
// plus head and reorg events).
func WithStateNotifier(n statefeed.Notifier) Option {
	return func(s *Service) error {
		s.stateNotifier = n
		return nil
	}
}

// WithScanBudgets overrides the scan page budgets (tests and tuning).
func WithScanBudgets(seedCandidates, pageCandidates, pageBytes int) Option {
	return func(s *Service) error {
		s.seedPageCandidates = seedCandidates
		s.pageCandidates = pageCandidates
		s.pageBytes = pageBytes
		return nil
	}
}

// WithInterPageYield overrides the pause between committed scan pages.
func WithInterPageYield(d time.Duration) Option {
	return func(s *Service) error {
		s.interPageYield = d
		return nil
	}
}

// WithTickInterval overrides the periodic reconciliation interval used for
// retention floor movement.
func WithTickInterval(d time.Duration) Option {
	return func(s *Service) error {
		s.tickInterval = d
		return nil
	}
}

// New constructs the coverage runtime shell. It is built before the
// blockchain service so the notifier can be injected at the producers'
// construction points; the chain view is late-bound with SetChainView.
func New(ctx context.Context, opts ...Option) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{
		ctx:                ctx,
		cancel:             cancel,
		notifier:           NewNotifier(),
		seedPageCandidates: defaultSeedPageCandidates,
		pageCandidates:     defaultPageCandidates,
		pageBytes:          defaultPageBytes,
		pruneBudget:        defaultPruneBudget,
		interPageYield:     defaultInterPageYield,
	}
	for _, o := range opts {
		if err := o(s); err != nil {
			cancel()
			return nil, err
		}
	}
	if s.db == nil {
		cancel()
		return nil, errors.New("coverage runtime requires a database")
	}
	if s.clockWaiter == nil {
		cancel()
		return nil, errors.New("coverage runtime requires a clock waiter")
	}
	s.store = &store{db: s.db}
	return s, nil
}

// Notifier returns the runtime's never-closed coalescing notifier for
// injection into producer construction options.
func (s *Service) Notifier() *Notifier {
	return s.notifier
}

// SetChainView late-binds the canonical/head view once the blockchain
// service exists.
func (s *Service) SetChainView(cv ChainView) {
	if cv == nil {
		return
	}
	s.chain.Store(&cv)
}

func (s *Service) chainView() ChainView {
	v, ok := s.chain.Load().(*ChainView)
	if !ok || v == nil {
		return nil
	}
	return *v
}

// Start launches the coordinator goroutine. It returns immediately; the
// coordinator waits for the startup clock before its first scan.
func (s *Service) Start() {
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.run()
	})
}

// Stop cancels the runtime lifecycle and joins the coordinator goroutine.
// Producers registered after the runtime stop first (reverse registration
// order), and late notifications into the never-closed notifier are
// harmless no-ops.
func (s *Service) Stop() error {
	s.stopOnce.Do(func() {
		s.cancel()
		s.wg.Wait()
	})
	return nil
}

// Status reports a fatal coordinator initialization error, if any.
func (s *Service) Status() error {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	return s.statusErr
}

func (s *Service) setStatus(err error) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	s.statusErr = err
}

func (s *Service) run() {
	defer s.wg.Done()

	clock, err := s.clockWaiter.WaitForClock(s.ctx)
	if err != nil {
		return // stopped before the clock was published
	}
	s.clock = clock
	s.started = time.Now()

	if params.BeaconConfig().GloasForkEpoch == math.MaxUint64 {
		log.Debug("Gloas is not scheduled; envelope coverage runtime is a no-op")
		return
	}

	if err := s.store.load(s.ctx); err != nil {
		log.WithError(err).Error("Could not load envelope coverage record")
		s.setStatus(err)
		return
	}
	if snap := s.store.snapshot(); snap.Initialized {
		coverageLowSlot.Set(float64(snap.Low))
		coverageHighSlot.Set(float64(snap.High))
	}

	var stateCh chan *feed.Event
	if s.stateNotifier != nil {
		stateCh = make(chan *feed.Event, 16)
		sub := s.stateNotifier.StateFeed().Subscribe(stateCh)
		defer sub.Unsubscribe()
	}

	tick := s.tickInterval
	if tick == 0 {
		tick = time.Duration(params.BeaconConfig().SecondsPerSlot*uint64(params.BeaconConfig().SlotsPerEpoch)) * time.Second
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	// Reconcile durable state once at startup, then on every wake.
	s.reconcile(s.ctx)
	for {
		select {
		case <-s.ctx.Done():
			return
		case ev := <-stateCh:
			s.translateEvent(ev)
			continue
		case <-s.notifier.wake():
		case <-ticker.C:
		}
		s.reconcile(s.ctx)
	}
}

// translateEvent folds state feed events into the coalescing notifier.
func (s *Service) translateEvent(ev *feed.Event) {
	if ev == nil {
		return
	}
	switch ev.Type {
	case statefeed.ExecutionPayloadProcessed:
		// Authoritative post-save live hook.
		s.notifier.Notify()
	case statefeed.NewHead, statefeed.NewHeadV2, statefeed.Reorg:
		s.notifier.NotifyHead()
	default:
	}
}

// notePublished records first-publication timing for a non-empty interval.
func (s *Service) notePublished(low, high primitives.Slot) {
	if s.firstPublished || high <= low {
		return
	}
	s.firstPublished = true
	migrationFirstPublishSeconds.Set(time.Since(s.started).Seconds())
}

// noteFloorReached records when the lower bound first reached the retention
// floor.
func (s *Service) noteFloorReached() {
	if s.reachedFloor {
		return
	}
	s.reachedFloor = true
	migrationDurationSeconds.Set(time.Since(s.started).Seconds())
}

// ServeRead is one generation-coherent serving view: the coverage snapshot,
// the in-process serve epoch, the canonical head root captured in the same
// read window, and at most quota canonical-revealed index roots inside
// [begin, pEnd) where pEnd = high unless the requested end is below high.
type ServeRead struct {
	Coverage Snapshot
	Epoch    uint64
	HeadRoot [32]byte
	Roots    []dbiface.RevealedEnvelopeSlotRoot
}

// CoherentServeRead copies the coverage snapshot, serve epoch, canonical
// head, and at most quota index roots under one short coordinator read lock
// (writers hold the exclusive lock through their combined Bolt commit and
// snapshot publication, so an old bound can never be paired with post-commit
// index contents). The caller performs all validation, reconstruction, and
// stream I/O after this returns.
func (s *Service) CoherentServeRead(ctx context.Context, begin, end primitives.Slot, quota uint64) (*ServeRead, error) {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()

	read := &ServeRead{
		Coverage: snapshotFromProto(s.store.cov),
		Epoch:    s.store.serveEpoch.Load(),
	}
	if cv := s.chainView(); cv != nil {
		hr, err := cv.HeadRoot(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "resolve head root for serve read")
		}
		read.HeadRoot = bytesutil.ToBytes32(hr)
	}
	snap := read.Coverage
	if snap.Initialized && begin >= snap.Low {
		pEnd := snap.High
		if end < snap.High {
			// Guarded increment: only when end < high, so it cannot wrap.
			pEnd = end + 1
		}
		if begin < pEnd {
			roots, err := s.db.RevealedEnvelopeRoots(ctx, begin, pEnd, quota)
			if err != nil {
				return nil, errors.Wrap(err, "seek revealed envelope index")
			}
			read.Roots = roots
		}
	}
	return read, nil
}

// ServeEpoch returns the current in-process serve epoch. A serving handler
// compares it with the epoch captured by its CoherentServeRead after
// validating its complete candidate response and before streaming the first
// chunk.
func (s *Service) ServeEpoch() uint64 {
	return s.store.serveEpoch.Load()
}

// Snapshot returns the currently published coverage snapshot.
func (s *Service) Snapshot() Snapshot {
	return s.store.snapshot()
}

// WakeReconcile asks the coordinator to reconcile durable state; serving
// anomalies use it so invariant violations are investigated promptly.
func (s *Service) WakeReconcile() {
	s.notifier.Notify()
}
