package blockchain

import (
	"context"
	"fmt"
	"time"

	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (s *Service) isNewHead(r [32]byte) bool {
	s.headLock.RLock()
	defer s.headLock.RUnlock()

	currentHeadRoot := s.originBlockRoot
	if s.head != nil {
		currentHeadRoot = s.headRoot()
	}

	return r != currentHeadRoot || r == [32]byte{}
}

func (s *Service) getStateAndBlock(ctx context.Context, r [32]byte) (state.BeaconState, interfaces.ReadOnlySignedBeaconBlock, error) {
	if !s.hasBlockInInitSyncOrDB(ctx, r) {
		return nil, nil, errors.New("block does not exist")
	}
	newHeadBlock, err := s.getBlock(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	headState, err := s.cfg.StateGen.StateByRoot(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	return headState, newHeadBlock, nil
}

type fcuConfig struct {
	headState     state.BeaconState
	headBlock     interfaces.ReadOnlySignedBeaconBlock
	headRoot      [32]byte
	proposingSlot primitives.Slot
	attributes    payloadattribute.Attributer
}

// sendFCU handles the logic to notify the engine of a forckhoice update
// when processing an incoming block during regular sync. It
// always updates the shuffling caches and handles epoch transitions .
func (s *Service) sendFCU(cfg *postBlockProcessConfig, fcuArgs *fcuConfig) {
	if cfg.postState.Version() < version.Fulu {
		// update the caches to compute the right proposer index
		// this function is called under a forkchoice lock which we need to release.
		s.ForkChoicer().Unlock()
		s.updateCachesPostBlockProcessing(cfg)
		s.ForkChoicer().Lock()
	}
	if err := s.getFCUArgs(cfg, fcuArgs); err != nil {
		log.WithError(err).Error("Could not get forkchoice update argument")
		return
	}
	// If head has not been updated and attributes are nil, we can skip the FCU.
	if !s.isNewHead(cfg.headRoot) && (fcuArgs.attributes == nil || fcuArgs.attributes.IsEmpty()) {
		return
	}
	// If we are proposing and we aim to reorg the block, we have already sent FCU with attributes on lateBlockTasks
	if fcuArgs.attributes != nil && !fcuArgs.attributes.IsEmpty() && s.shouldOverrideFCU(cfg.headRoot, s.CurrentSlot()+1) {
		return
	}
	if s.inRegularSync() {
		go s.forkchoiceUpdateWithExecution(cfg.ctx, fcuArgs)
	}

	if s.isNewHead(fcuArgs.headRoot) {
		if err := s.saveHead(cfg.ctx, fcuArgs.headRoot, fcuArgs.headBlock, fcuArgs.headState); err != nil {
			log.WithError(err).Error("Could not save head")
		}
		s.pruneAttsFromPool(s.ctx, fcuArgs.headState, fcuArgs.headBlock)
	}
}

// fockchoiceUpdateWithExecution is a wrapper around notifyForkchoiceUpdate. It gets a forkchoice lock and calls the engine.
// The caller of this function should NOT have a lock in forkchoice store.
func (s *Service) forkchoiceUpdateWithExecution(ctx context.Context, args *fcuConfig) {
	_, span := trace.StartSpan(ctx, "beacon-chain.blockchain.forkchoiceUpdateWithExecution")
	defer span.End()
	// Note: Use the service context here to avoid the parent context being ended during a forkchoice update.
	ctx = trace.NewContext(s.ctx, span)
	s.ForkChoicer().Lock()
	defer s.ForkChoicer().Unlock()
	_, err := s.notifyForkchoiceUpdate(ctx, args)
	if err != nil {
		log.WithError(err).Error("Could not notify forkchoice update")
	}
}

// shouldOverrideFCU checks whether the incoming block is still subject to being
// reorged or not by the next proposer.
func (s *Service) shouldOverrideFCU(newHeadRoot [32]byte, proposingSlot primitives.Slot) bool {
	headWeight, err := s.cfg.ForkChoiceStore.Weight(newHeadRoot)
	if err != nil {
		log.WithError(err).WithField("root", fmt.Sprintf("%#x", newHeadRoot)).Warn("Could not determine node weight")
	}
	currentSlot := s.CurrentSlot()
	if proposingSlot == currentSlot {
		proposerHead := s.cfg.ForkChoiceStore.GetProposerHead()
		if proposerHead != newHeadRoot {
			return true
		}
		log.WithFields(logrus.Fields{
			"root":   fmt.Sprintf("%#x", newHeadRoot),
			"weight": headWeight,
		}).Infof("Attempted late block reorg aborted due to attestations at %d seconds",
			params.BeaconConfig().SecondsPerSlot)
		lateBlockFailedAttemptSecondThreshold.Inc()
	} else {
		if s.cfg.ForkChoiceStore.ShouldOverrideFCU() {
			return true
		}
		sss, err := slots.SinceSlotStart(currentSlot, s.genesisTime, time.Now())
		if err != nil {
			log.WithError(err).Error("Could not compute seconds since slot start")
		}
		if sss >= doublylinkedtree.ProcessAttestationsThreshold {
			log.WithFields(logrus.Fields{
				"root":           fmt.Sprintf("%#x", newHeadRoot),
				"weight":         headWeight,
				"sinceSlotStart": sss,
				"threshold":      doublylinkedtree.ProcessAttestationsThreshold,
			}).Info("Attempted late block reorg aborted due to attestations after threshold")
			lateBlockFailedAttemptFirstThreshold.Inc()
		}
	}
	return false
}
