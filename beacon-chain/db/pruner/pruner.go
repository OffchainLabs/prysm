package pruner

import (
	"context"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filters"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/iface"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
	"time"
)

var log = logrus.WithField("prefix", "db-pruner")

// WeakSubjectivityPruner defines a service that prunes beacon chain DB based on weak subjectivity period.
type WeakSubjectivityPruner struct {
	db           db.Database
	headFetcher  blockchain.HeadFetcher
	genesisTime  time.Time
	pruningEpoch primitives.Epoch
	done         chan struct{}
}

func New(db iface.Database, headFetcher blockchain.HeadFetcher, genesisTime time.Time) *WeakSubjectivityPruner {
	return &WeakSubjectivityPruner{
		db:          db,
		headFetcher: headFetcher,
		genesisTime: genesisTime,
		done:        make(chan struct{}),
	}
}

func (p *WeakSubjectivityPruner) Start(ctx context.Context) {
	log.Info("Starting Beacon DB pruner service")
	go p.run(ctx)
}

func (p *WeakSubjectivityPruner) Stop() {
	log.Info("Stopping Beacon DB pruner service")
	close(p.done)
}

func (p *WeakSubjectivityPruner) run(ctx context.Context) {
	ticker := slots.NewSlotTicker(p.genesisTime, params.BeaconConfig().SecondsPerSlot)
	defer ticker.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		case slot := <-ticker.C():
			if !slots.IsEpochStart(slot) {
				continue
			}

			if err := p.prune(ctx); err != nil {
				log.WithError(err).Error("Failed to prune database")
			}
		}
	}
}

// prune deletes historical chain data beyond the weak subjectivity period.
func (p *WeakSubjectivityPruner) prune(ctx context.Context) error {
	// Get current finalized epoch.
	finalized, err := p.db.FinalizedCheckpoint(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get finalized checkpoint")
	}
	finalizedEpoch := finalized.Epoch

	// Get head state to compute weak subjectivity period.
	headState, err := p.headFetcher.HeadState(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get head state")
	}

	// Calculate weak subjectivity period.
	wsPeriod, err := helpers.ComputeWeakSubjectivityPeriod(ctx, headState, params.BeaconConfig())
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

	startSlot := params.BeaconConfig().GenesisSlot
	endSlot, err := slots.EpochStart(pruneEpoch)
	if err != nil {
		return errors.Wrap(err, "could not get epoch start slot")
	}

	filter := filters.NewFilter()
	filter.SetStartSlot(startSlot)
	filter.SetEndSlot(endSlot)

	roots, err := p.db.BlockRoots(ctx, filter)
	if err != nil {
		return errors.Wrap(err, "could not get block roots")
	}

	for _, root := range roots {
		if err = p.db.DeleteBlock(ctx, root); err != nil {
			return errors.Wrap(err, "could not delete block")
		}
		if err = p.db.DeleteState(ctx, root); err != nil {
			return errors.Wrap(err, "could not delete state")
		}
	}

	// Update pruning checkpoint.
	p.pruningEpoch = pruneEpoch

	return nil
}
