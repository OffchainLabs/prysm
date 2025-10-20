package pruner

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "db-pruner")

const (
	// defaultPrunableBatchSize is the number of slots that can be pruned at once.
	defaultPrunableBatchSize = 32
	// defaultPruningWindow is the duration of one pruning window.
	defaultPruningWindow = time.Second * 3
	// defaultNumBatchesToPrune is the number of batches to prune in one pruning window.
	defaultNumBatchesToPrune = 15
)

// custodyUpdater is a tiny interface that p2p service implements; kept here to avoid
// importing the p2p package and creating a cycle.
type custodyUpdater interface {
	UpdateEarliestAvailableSlot(earliestAvailableSlot primitives.Slot) error
}

type ServiceOption func(*Service)

// WithRetentionPeriod allows the user to specify a different data retention period than the spec default.
// The retention period is specified in epochs, and must be >= MIN_EPOCHS_FOR_BLOCK_REQUESTS.
func WithRetentionPeriod(retentionEpochs primitives.Epoch) ServiceOption {
	return func(s *Service) {
		defaultRetentionEpochs := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests) + 1
		if retentionEpochs < defaultRetentionEpochs {
			log.WithField("userEpochs", retentionEpochs).
				WithField("minRequired", defaultRetentionEpochs).
				Warn("Retention period too low, ignoring and using minimum required value")
			retentionEpochs = defaultRetentionEpochs
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
	custody        custodyUpdater
}

func New(ctx context.Context, db iface.Database, genesisTime time.Time, initSyncWaiter, backfillWaiter func() error, custody custodyUpdater, opts ...ServiceOption) (*Service, error) {
	if custody == nil {
		return nil, errors.New("custody updater is required for pruner but was not provided")
	}

	p := &Service{
		ctx:            ctx,
		db:             db,
		ps:             pruneStartSlotFunc(primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests) + 1), // Default retention epochs is MIN_EPOCHS_FOR_BLOCK_REQUESTS + 1 from the current slot.
		done:           make(chan struct{}),
		slotTicker:     slots.NewSlotTicker(slots.UnsafeStartTime(genesisTime, 0), params.BeaconConfig().SecondsPerSlot),
		initSyncWaiter: initSyncWaiter,
		backfillWaiter: backfillWaiter,
		custody:        custody,
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
	numBatches, err := p.pruneBatches(pruneUpto)
	if err != nil {
		return errors.Wrap(err, "failed to prune batches")
	}

	earliestAvailableSlot := pruneUpto + 1

	// Update pruning checkpoint.
	p.prunedUpto = pruneUpto

	// Update the earliest available slot after pruning
	if err := p.updateEarliestAvailableSlot(earliestAvailableSlot); err != nil {
		return errors.Wrap(err, "update earliest available slot")
	}

	log.WithFields(logrus.Fields{
		"prunedUpto":            pruneUpto,
		"earliestAvailableSlot": earliestAvailableSlot,
		"duration":              time.Since(tt),
		"currentSlot":           slot,
		"batchSize":             defaultPrunableBatchSize,
		"numBatches":            numBatches,
	}).Debug("Successfully pruned chain data")

	return nil
}

// updateEarliestAvailableSlot updates the earliest available slot via the injected custody updater
// and also persists it to the database.
func (p *Service) updateEarliestAvailableSlot(earliestAvailableSlot primitives.Slot) error {
	if !params.FuluEnabled() {
		return nil
	}

	// Update the p2p in-memory state
	if err := p.custody.UpdateEarliestAvailableSlot(earliestAvailableSlot); err != nil {
		return errors.Wrapf(err, "update earliest available slot after pruning to %d", earliestAvailableSlot)
	}

	// Persist to database to ensure it survives restarts
	if err := p.db.UpdateEarliestAvailableSlot(p.ctx, earliestAvailableSlot); err != nil {
		return errors.Wrapf(err, "update earliest available slot in database for slot %d", earliestAvailableSlot)
	}

	return nil
}

func (p *Service) pruneBatches(pruneUpto primitives.Slot) (int, error) {
	ctx, cancel := context.WithTimeout(p.ctx, defaultPruningWindow)
	defer cancel()

	numBatches := 0
	for {
		select {
		case <-ctx.Done():
			return numBatches, nil
		default:
			for i := 0; i < defaultNumBatchesToPrune; i++ {
				slotsDeleted, err := p.db.DeleteHistoricalDataBeforeSlot(ctx, pruneUpto, defaultPrunableBatchSize)
				if err != nil {
					return 0, errors.Wrapf(err, "could not delete upto slot %d", pruneUpto)
				}

				// Return if there's nothing to delete.
				if slotsDeleted == 0 {
					return numBatches, nil
				}

				numBatches++
			}
		}
	}
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
