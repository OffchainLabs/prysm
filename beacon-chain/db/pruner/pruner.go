package pruner

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/iface"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "db-pruner")

// Pruner defines a service that prunes beacon chain DB based on weak subjectivity period.
type Pruner struct {
	ctx          context.Context
	db           db.Database
	headFetcher  blockchain.HeadFetcher
	genesisTime  time.Time
	pruningEpoch primitives.Epoch
	done         chan struct{}
}

func New(ctx context.Context, db iface.Database, headFetcher blockchain.HeadFetcher, genesisTime time.Time) *Pruner {
	return &Pruner{
		ctx:         ctx,
		db:          db,
		headFetcher: headFetcher,
		genesisTime: genesisTime,
		done:        make(chan struct{}),
	}
}

func (p *Pruner) Start() {
	log.Info("Starting Beacon DB pruner service")
	go p.run()
}

func (p *Pruner) Stop() error {
	log.Info("Stopping Beacon DB pruner service")
	close(p.done)
	return nil
}

func (p *Pruner) Status() error {
	return nil
}

func (p *Pruner) run() {
	ticker := slots.NewSlotTicker(p.genesisTime, params.BeaconConfig().SecondsPerSlot)
	defer ticker.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.done:
			return
		case slot := <-ticker.C():
			// Prune at the start of every epoch.
			// TODO: prune at the middle of epoch.
			if !slots.IsEpochStart(slot) {
				continue
			}

			if err := p.prune(); err != nil {
				log.WithError(err).Error("Failed to prune database")
			}
		}
	}
}

// prune deletes historical chain data beyond the weak subjectivity period.
func (p *Pruner) prune() error {
	// Get current finalized epoch.
	finalized, err := p.db.FinalizedCheckpoint(p.ctx)
	if err != nil {
		return errors.Wrap(err, "could not get finalized checkpoint")
	}
	finalizedEpoch := finalized.Epoch

	// Get head state to compute weak subjectivity period.
	headState, err := p.headFetcher.HeadState(p.ctx)
	if err != nil {
		return errors.Wrap(err, "could not get head state")
	}

	// Calculate weak subjectivity period.
	wsPeriod, err := helpers.ComputeWeakSubjectivityPeriod(p.ctx, headState, params.BeaconConfig())
	if err != nil {
		return errors.Wrap(err, "could not compute weak subjectivity period")
	}

	// Calculate pruning point
	if finalizedEpoch <= wsPeriod {
		// Too early to prune
		return nil
	}
	pruneEpoch := finalizedEpoch - wsPeriod

	// Skip if already pruned up to this epoch.
	if pruneEpoch <= p.pruningEpoch {
		return nil
	}

	log.WithFields(logrus.Fields{
		"finalizedEpoch": finalizedEpoch,
		"pruneEpoch":     pruneEpoch,
		"wsPeriod":       wsPeriod,
	}).Info("Pruning chain data before weak subjectivity period")

	// Prune everything before this slot.
	pruneSlot, err := slots.EpochStart(pruneEpoch)
	if err != nil {
		return errors.Wrap(err, "could not get epoch start slot")
	}

	if err = p.db.DeleteBeforeSlot(p.ctx, pruneSlot); err != nil {
		return errors.Wrap(err, "could not delete before slot")
	}
	// Update pruning checkpoint.
	p.pruningEpoch = pruneEpoch

	return nil
}
