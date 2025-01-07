package pruner

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/iface"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "db-pruner")

type ServiceOption func(*Service)

// WithRetentionPeriod allows the user to specify a different data retention period than the spec default.
// The retention period is specified in epochs, and must be >= MIN_EPOCHS_FOR_BLOCK_REQUESTS.
func WithRetentionPeriod(retentionEpochs primitives.Epoch) ServiceOption {
	return func(s *Service) {
		defaultRetentionEpochs := helpers.MinEpochsForBlockRequests() - 1
		if retentionEpochs < defaultRetentionEpochs {
			log.WithField("userEpochs", retentionEpochs).
				WithField("minRequired", defaultRetentionEpochs).
				Warn("Retention period too low, using minimum required value")
		}

		s.ps = pruneStartSlotFunc(retentionEpochs)
	}
}

func WithSlotTicker(slotTicker slots.Ticker) ServiceOption {
	return func(s *Service) {
		s.slotTicker = slotTicker
	}
}

type SyncChecker interface {
	Synced() bool
}

type BackfillChecker interface {
	IsComplete() bool
}

// Service defines a service that prunes beacon chain DB based on MIN_EPOCHS_FOR_BLOCK_REQUESTS.
type Service struct {
	ctx             context.Context
	db              db.Database
	ps              func(current primitives.Slot) primitives.Slot
	prunedSlot      primitives.Slot
	done            chan struct{}
	slotTicker      slots.Ticker
	syncChecker     SyncChecker
	backfillChecker BackfillChecker
}

func New(ctx context.Context, db iface.Database, genesisTime time.Time, syncChecker SyncChecker, backfillChecker BackfillChecker, opts ...ServiceOption) (*Service, error) {
	p := &Service{
		ctx:             ctx,
		db:              db,
		ps:              pruneStartSlotFunc(helpers.MinEpochsForBlockRequests() - 1), // Default retention epochs is MIN_EPOCHS_FOR_BLOCK_REQUESTS - 1 from the current slot.
		done:            make(chan struct{}),
		slotTicker:      slots.NewSlotTicker(genesisTime, params.BeaconConfig().SecondsPerSlot),
		syncChecker:     syncChecker,
		backfillChecker: backfillChecker,
	}

	for _, o := range opts {
		o(p)
	}

	return p, nil
}

func (p *Service) Start() {
	log.Info("Starting Beacon DB pruner service")
	go p.run()
}

func (p *Service) Stop() error {
	log.Info("Stopping Beacon DB pruner service")
	close(p.done)
	return nil
}

func (p *Service) Status() error {
	return nil
}

func (p *Service) run() {
	defer p.slotTicker.Done()

	for {
		select {
		case <-p.ctx.Done():
			log.Debug("Stopping Beacon DB pruner service", "prunedSlot", p.prunedSlot)
			return
		case <-p.done:
			log.Debug("Stopping Beacon DB pruner service", "prunedSlot", p.prunedSlot)
			return
		case slot := <-p.slotTicker.C():
			// Prune at the middle of every epoch since we do a lot of things around epoch boundaries.
			if slots.SinceEpochStarts(slot) != (params.BeaconConfig().SlotsPerEpoch / 2) {
				continue
			}

			// Skip pruning if syncing is in progress.
			if !p.syncChecker.Synced() {
				log.Debug("Skipping pruning as initial sync is in progress")
				continue
			}

			// Skip pruning if backfill is in progress.
			if !p.backfillChecker.IsComplete() {
				log.Debug("Skipping pruning as backfill is in progress")
				continue
			}

			if err := p.prune(slot); err != nil {
				log.WithError(err).Error("Failed to prune database")
			}
		}
	}
}

// prune deletes historical chain data beyond the pruneSlot.
func (p *Service) prune(slot primitives.Slot) error {
	// Prune everything from this slot.
	pruneSlot := p.ps(slot)

	// Can't prune beyond genesis.
	if pruneSlot == 0 {
		return nil
	}

	// Skip if already pruned up to this slot.
	if pruneSlot <= p.prunedSlot {
		return nil
	}

	log.WithFields(logrus.Fields{
		"pruneSlot": pruneSlot,
	}).Debug("Pruning chain data")

	tt := time.Now()
	if err := p.db.DeleteHistoricalDataBeforeSlot(p.ctx, pruneSlot); err != nil {
		return errors.Wrapf(err, "could not delete before slot %d", pruneSlot)
	}

	log.WithFields(logrus.Fields{
		"pruneSlot":   pruneSlot,
		"duration":    time.Since(tt),
		"currentSlot": slot,
	}).Debug("Successfully pruned chain data")

	// Update pruning checkpoint.
	p.prunedSlot = pruneSlot

	return nil
}

// pruneStartSlotFunc returns the function to determine the start slot to start pruning.
func pruneStartSlotFunc(retentionEpochs primitives.Epoch) func(primitives.Slot) primitives.Slot {
	return func(current primitives.Slot) primitives.Slot {
		if retentionEpochs > slots.MaxSafeEpoch() {
			retentionEpochs = slots.MaxSafeEpoch()
		}
		offset := slots.UnsafeEpochStart(retentionEpochs)
		if offset >= current {
			return 0
		}
		return current - offset
	}
}
