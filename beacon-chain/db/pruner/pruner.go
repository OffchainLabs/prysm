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
		defaultRetentionEpochs := helpers.MinEpochsForBlockRequests() + 1
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

// Service defines a service that prunes beacon chain DB based on MIN_EPOCHS_FOR_BLOCK_REQUESTS.
type Service struct {
	ctx            context.Context
	db             db.Database
	ps             func(current primitives.Slot) primitives.Slot
	prunedUpto     primitives.Slot
	done           chan struct{}
	slotTicker     slots.Ticker
	backfillWaiter func() error
	initSyncWaiter func() error
}

func New(ctx context.Context, db iface.Database, genesisTime uint64, initSyncWaiter, backfillWaiter func() error, opts ...ServiceOption) (*Service, error) {
	p := &Service{
		ctx:            ctx,
		db:             db,
		ps:             pruneStartSlotFunc(helpers.MinEpochsForBlockRequests() + 1), // Default retention epochs is MIN_EPOCHS_FOR_BLOCK_REQUESTS + 1 from the current slot.
		done:           make(chan struct{}),
		slotTicker:     slots.NewSlotTicker(slots.StartTime(genesisTime, 0), params.BeaconConfig().SecondsPerSlot),
		initSyncWaiter: initSyncWaiter,
		backfillWaiter: backfillWaiter,
	}

	for _, o := range opts {
		o(p)
	}

	return p, nil
}

func (p *Service) Start() {
	log.Info("Starting Beacon DB pruner service")
	p.run()
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
	if p.initSyncWaiter != nil {
		log.Info("Waiting for initial sync service to complete before starting pruner")
		if err := p.initSyncWaiter(); err != nil {
			log.WithError(err).Error("Failed to start database pruner, error waiting for initial sync completion")
			return
		}
	}
	if p.backfillWaiter != nil {
		log.Info("Waiting for backfill service to complete before starting pruner")
		if err := p.backfillWaiter(); err != nil {
			log.WithError(err).Error("Failed to start database pruner, error waiting for backfill completion")
			return
		}
	}

	defer p.slotTicker.Done()

	for {
		select {
		case <-p.ctx.Done():
			log.Debug("Stopping Beacon DB pruner service", "prunedUpto", p.prunedUpto)
			return
		case <-p.done:
			log.Debug("Stopping Beacon DB pruner service", "prunedUpto", p.prunedUpto)
			return
		case slot := <-p.slotTicker.C():
			// Prune at the middle of every epoch since we do a lot of things around epoch boundaries.
			if slots.SinceEpochStarts(slot) != (params.BeaconConfig().SlotsPerEpoch / 2) {
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
	// Prune everything up to this slot (inclusive).
	pruneUpto := p.ps(slot)

	// Can't prune beyond genesis.
	if pruneUpto == 0 {
		return nil
	}

	// Skip if already pruned up to this slot.
	if pruneUpto <= p.prunedUpto {
		return nil
	}

	log.WithFields(logrus.Fields{
		"pruneUpto": pruneUpto,
	}).Debug("Pruning chain data")

	tt := time.Now()
	if err := p.db.DeleteHistoricalDataBeforeSlot(p.ctx, pruneUpto); err != nil {
		return errors.Wrapf(err, "could not delete upto slot %d", pruneUpto)
	}

	log.WithFields(logrus.Fields{
		"prunedUpto":  pruneUpto,
		"duration":    time.Since(tt),
		"currentSlot": slot,
	}).Debug("Successfully pruned chain data")

	// Update pruning checkpoint.
	p.prunedUpto = pruneUpto

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
