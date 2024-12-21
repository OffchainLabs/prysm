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

type ServiceOption func(*Service) error

// WithMinimumSlot allows the user to specify a different prune minimum slot than the spec default of current - MIN_EPOCHS_FOR_BLOCK_REQUESTS - 1.
// If this value is greater than current - MIN_EPOCHS_FOR_BLOCK_REQUESTS - 1, it will be ignored with a warning log.
func WithMinimumSlot(s primitives.Slot) ServiceOption {
	ms := func(current primitives.Slot) primitives.Slot {
		specMin := pruneStartSlot(current)
		if s < specMin {
			return s
		}
		log.WithField("userSlot", s).WithField("specMinSlot", specMin).
			Warn("Ignoring user-specified slot > MIN_EPOCHS_FOR_BLOCK_REQUESTS.")
		return specMin
	}
	return func(s *Service) error {
		s.ps = ms
		return nil
	}
}

// Service defines a service that prunes beacon chain DB based on MIN_EPOCHS_FOR_BLOCK_REQUESTS.
type Service struct {
	ctx         context.Context
	db          db.Database
	genesisTime time.Time
	ps          func(current primitives.Slot) primitives.Slot
	prunedSlot  primitives.Slot
	done        chan struct{}
}

func New(ctx context.Context, db iface.Database, genesisTime time.Time, opts ...ServiceOption) (*Service, error) {
	p := &Service{
		ctx:         ctx,
		db:          db,
		genesisTime: genesisTime,
		ps:          pruneStartSlot,
		done:        make(chan struct{}),
	}

	for _, o := range opts {
		if err := o(p); err != nil {
			return nil, err
		}
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
	ticker := slots.NewSlotTicker(p.genesisTime, params.BeaconConfig().SecondsPerSlot)
	defer ticker.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.done:
			return
		case slot := <-ticker.C():
			// Prune at the middle of every epoch.
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
	if err := p.db.DeleteBlocksAndStatesBeforeSlot(p.ctx, pruneSlot); err != nil {
		return errors.Wrap(err, "could not delete before slot")
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

// pruneStartSlot determines the start slot to start pruning.
// MIN_EPOCHS_FOR_BLOCK_REQUESTS - 1 from the current slot.
func pruneStartSlot(current primitives.Slot) primitives.Slot {
	oe := helpers.MinEpochsForBlockRequests()
	if oe > slots.MaxSafeEpoch() {
		oe = slots.MaxSafeEpoch()
	}
	offset := slots.UnsafeEpochStart(oe)
	if offset >= current {
		return 0
	}
	return current - offset - params.BeaconConfig().SlotsPerEpoch // Stay one epoch behind.
}
