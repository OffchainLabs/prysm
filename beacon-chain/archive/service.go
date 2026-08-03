// Package archive regenerates the historical beacon states of an archive node. Once backfill has downloaded
// every block down to the archive origin, this service replays the chain forward from the origin state and
// persists a state at every boundary of the state-diff tree, making any historical state cheap to serve.
package archive

import (
	"context"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filters"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// retryInterval is how long the service waits before retrying after a failed round, and how long it idles
// when it has caught up but finalization has not yet let it hand off.
var retryInterval = 30 * time.Second

const progressLogInterval = 60 * time.Second

// Database is the subset of the beacon db the walk needs.
type Database interface {
	Blocks(ctx context.Context, f *filters.QueryFilter) ([]interfaces.ReadOnlySignedBeaconBlock, [][32]byte, error)
	HighestRootsBelowSlot(ctx context.Context, slot primitives.Slot) (primitives.Slot, [][32]byte, error)
	IsFinalizedBlock(ctx context.Context, blockRoot [32]byte) bool
	FinalizedCheckpoint(ctx context.Context) (*ethpb.Checkpoint, error)
	SaveState(ctx context.Context, st state.ReadOnlyBeaconState, blockRoot [32]byte) error
	StateBySlotFromDiffTree(ctx context.Context, slot primitives.Slot) (state.BeaconState, error)
	ArchiveStatus(ctx context.Context) (*kv.ArchiveStatus, error)
	SaveArchiveStatus(ctx context.Context, as *kv.ArchiveStatus) error
}

// StateManager is the subset of stategen the service drives.
type StateManager interface {
	ArchivePending() bool
	SetArchivePending(pending bool)
	CompleteArchiveRegeneration(ctx context.Context, nextUnwrittenBoundary primitives.Slot) (bool, error)
}

// Service regenerates historical states into the state-diff tree.
type Service struct {
	ctx            context.Context
	db             Database
	sg             StateManager
	cw             startup.ClockWaiter
	backfillWaiter func() error
	lock           sync.RWMutex
	archiveStatus  *kv.ArchiveStatus
	progressLogger *rateLimitedLogger
}

var _ runtime.Service = (*Service)(nil)

// New creates the archive regeneration service. backfillWaiter blocks until backfill has finished importing
// blocks down to the archive origin.
func New(ctx context.Context, d Database, sg StateManager, cw startup.ClockWaiter, backfillWaiter func() error) *Service {
	return &Service{
		ctx:            ctx,
		db:             d,
		sg:             sg,
		cw:             cw,
		backfillWaiter: backfillWaiter,
		progressLogger: newRateLimitedLogger(log, progressLogInterval),
	}
}

// Start runs the regeneration loop in the current goroutine until the walk hands cold state migration back to
// the normal finalization-driven path, or the node shuts down.
func (s *Service) Start() {
	if !s.sg.ArchivePending() {
		log.Debug("Archive state regeneration is not pending; service is idle")
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	as, err := s.db.ArchiveStatus(ctx)
	if err != nil {
		log.WithError(err).Error("Could not read the archive status; state regeneration will not run")
		return
	}
	s.setStatus(as)

	if _, err := s.cw.WaitForClock(ctx); err != nil {
		log.WithError(err).Error("Service failed to start while waiting for genesis data")
		return
	}
	// The walk needs every block above the origin, so it cannot start until backfill is done.
	log.WithField("originSlot", as.OriginSlot).Info("Waiting for backfill to complete before regenerating states")
	if s.backfillWaiter != nil {
		if err := s.backfillWaiter(); err != nil {
			log.WithError(err).Error("Error waiting for backfill to complete")
			return
		}
	}
	log.WithFields(logrus.Fields{
		"originSlot":             as.OriginSlot,
		"regeneratedThroughSlot": as.RegeneratedThroughSlot,
	}).Info("Starting archive state regeneration")

	for {
		if ctx.Err() != nil {
			return
		}
		done, err := s.round(ctx)
		if err != nil {
			log.WithError(err).Error("Archive state regeneration round failed; retrying")
		}
		if done {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryInterval):
		}
	}
}

// round walks as far as the current finalized checkpoint allows and then attempts the handoff. It reports
// whether regeneration is complete.
func (s *Service) round(ctx context.Context) (bool, error) {
	target, err := s.target(ctx)
	if err != nil {
		return false, err
	}
	as := s.status()
	regenTargetSlot.Set(float64(target))

	if as.RegeneratedThroughSlot < target {
		s.logProgress(as, target)
		if err := s.walk(ctx, target); err != nil {
			return false, err
		}
		as = s.status()
	}

	next := nextBoundary(as.OriginSlot, as.RegeneratedThroughSlot)
	handedOff, err := s.sg.CompleteArchiveRegeneration(ctx, next)
	if err != nil {
		return false, errors.Wrap(err, "could not complete archive regeneration")
	}
	if !handedOff {
		// Finalization moved while the walk was running; keep going.
		return false, nil
	}

	as.Complete = true
	if err := s.db.SaveArchiveStatus(ctx, as); err != nil {
		// The handoff already happened in memory, so failing to persist only costs a redundant walk on the
		// next boot. Surface it rather than unwinding.
		return true, errors.Wrap(err, "could not persist archive completion")
	}
	s.setStatus(as)
	log.WithField("regeneratedThroughSlot", as.RegeneratedThroughSlot).
		Info("Archive state regeneration finished; all historical states are available")
	return true, nil
}

// target is the highest slot the walk may reach: the start of the finalized epoch. Blocks below it are all in
// epochs older than the finalized one, so the finalized index holds a real container for each of them rather
// than a needs-reindexing marker.
func (s *Service) target(ctx context.Context) (primitives.Slot, error) {
	cp, err := s.db.FinalizedCheckpoint(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "could not read the finalized checkpoint")
	}
	target, err := slots.EpochStart(cp.Epoch)
	if err != nil {
		return 0, errors.Wrap(err, "could not compute the finalized epoch start slot")
	}
	return target, nil
}

func (s *Service) status() *kv.ArchiveStatus {
	s.lock.RLock()
	defer s.lock.RUnlock()
	cp := *s.archiveStatus
	return &cp
}

func (s *Service) setStatus(as *kv.ArchiveStatus) {
	s.lock.Lock()
	defer s.lock.Unlock()
	cp := *as
	s.archiveStatus = &cp
}

func (*Service) Stop() error {
	return nil
}

func (*Service) Status() error {
	return nil
}
