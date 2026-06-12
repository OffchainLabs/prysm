package blockchain

import (
	"context"
	"math"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	coregloas "github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

func (s *Service) waitUntilEpoch(target primitives.Epoch, secondsPerSlot uint64) error {
	if slots.ToEpoch(s.CurrentSlot()) >= target {
		return nil
	}
	ticker := slots.NewSlotTicker(s.genesisTime, secondsPerSlot)
	defer ticker.Done()
	for {
		select {
		case slot := <-ticker.C():
			if slots.ToEpoch(slot) >= target {
				return nil
			}
		case <-s.ctx.Done():
			return s.ctx.Err()
		}
	}
}

func (s *Service) runLatePayloadTasks() {
	if err := s.waitForSync(); err != nil {
		log.WithError(err).Error("Failed to wait for initial sync")
		return
	}
	cfg := params.BeaconConfig()
	if cfg.GloasForkEpoch == math.MaxUint64 {
		return
	}
	if err := s.waitUntilEpoch(cfg.GloasForkEpoch, cfg.SecondsPerSlot); err != nil {
		return
	}
	offset := cfg.SlotComponentDuration(cfg.PayloadAttestationDueBPS)
	ticker := slots.NewSlotTickerWithOffset(s.genesisTime, offset, cfg.SecondsPerSlot)
	defer ticker.Done()
	for {
		select {
		case <-ticker.C():
			s.latePayloadTasks(s.ctx)
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting late payload tasks routine")
			return
		}
	}
}

func (s *Service) checkIfProposing(st state.ReadOnlyBeaconState, slot primitives.Slot) (cache.TrackedValidator, bool) {
	e := slots.ToEpoch(slot)
	stateEpoch := slots.ToEpoch(st.Slot())
	fuluAndNextEpoch := st.Version() >= version.Fulu && e == stateEpoch+1
	if e == stateEpoch || fuluAndNextEpoch {
		return s.trackedProposer(st, slot)
	}
	return cache.TrackedValidator{}, false
}

// computePayloadWithdrawals returns the withdrawals for the next payload.
// If the parent's payload was delivered (full), it applies the parent's
// execution requests on a state copy before computing withdrawals.
// If the parent was empty, it returns the existing payload_expected_withdrawals.
func (s *Service) computePayloadWithdrawals(ctx context.Context, st state.BeaconState, parentRoot [32]byte, headFull bool) ([]*enginev1.Withdrawal, error) {
	if slots.ToEpoch(s.HeadSlot()) < params.BeaconConfig().GloasForkEpoch {
		result, err := st.ExpectedWithdrawalsGloas()
		if err != nil {
			return nil, errors.Wrap(err, "could not compute expected withdrawals")
		}
		return result.Withdrawals, nil
	}
	if !headFull {
		return st.PayloadExpectedWithdrawals()
	}
	// TODO: replace DB lookup with a single-entry cache (blockroot → envelope).
	envelope, err := s.cfg.BeaconDB.ExecutionPayloadEnvelope(ctx, parentRoot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get parent execution payload envelope")
	}
	if err := coregloas.ApplyParentExecutionPayload(ctx, st, envelope.Message.ExecutionRequests); err != nil {
		return nil, errors.Wrap(err, "could not apply parent execution payload")
	}
	result, err := st.ExpectedWithdrawalsGloas()
	if err != nil {
		return nil, errors.Wrap(err, "could not compute expected withdrawals")
	}
	return result.Withdrawals, nil
}

// This is a Gloas version of getPayloadAttribute that avoids all the clutter that was originally due to the proposer Index.
// It is guaranteed to be called for the current slot + 1 and the head state to have been advanced to at least the current epoch.
func (s *Service) getLatePayloadAttribute(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot, headRoot []byte) payloadattribute.Attributer {
	emptyAttri := payloadattribute.EmptyWithVersion(st.Version())
	val, proposing := s.checkIfProposing(st, slot)
	if !proposing {
		return emptyAttri
	}

	var err error
	st, err = transition.ProcessSlotsIfNeeded(ctx, st, headRoot, slot)
	if err != nil {
		log.WithError(err).Error("Could not process slots to get payload attribute")
		return emptyAttri
	}

	prevRando, err := helpers.RandaoMix(st, time.CurrentEpoch(st))
	if err != nil {
		log.WithError(err).Error("Could not get randao mix to get payload attribute")
		return emptyAttri
	}

	t, err := slots.StartTime(s.genesisTime, slot)
	if err != nil {
		log.WithError(err).Error("Could not get timestamp to get payload attribute")
		return emptyAttri
	}

	withdrawals, err := st.PayloadExpectedWithdrawals()
	if err != nil {
		log.WithError(err).Error("Could not get payload withdrawals to get payload attribute")
		return emptyAttri
	}

	attr, err := payloadattribute.New(&enginev1.PayloadAttributesV4{
		Timestamp:             uint64(t.Unix()),
		PrevRandao:            prevRando,
		SuggestedFeeRecipient: val.FeeRecipient[:],
		Withdrawals:           withdrawals,
		ParentBeaconBlockRoot: headRoot,
		SlotNumber:            uint64(slot),
		TargetGasLimit:        val.GasLimit,
	})
	if err != nil {
		log.WithError(err).Error("Could not get payload attribute")
		return emptyAttri
	}
	return attr
}

// latePayloadTasks sends an FCU when no payload arrived for the current slot's block.
// The case where the block was also missing would have been dealt by lateBlockTasks already.
func (s *Service) latePayloadTasks(ctx context.Context) {
	currentSlot := s.CurrentSlot()
	if currentSlot != s.HeadSlot() {
		// We must've already sent a FCU and updated the caches in lateBlockTaks.
		return
	}
	r, err := s.HeadRoot(ctx)
	if err != nil {
		log.WithError(err).Error("Failed to get head root")
		return
	}
	hr := [32]byte(r)
	if s.payloadBeingSynced.isSyncing(hr) {
		return
	}
	if s.HasFullNode(hr) {
		return
	}
	st, err := s.HeadStateReadOnly(ctx)
	if err != nil {
		log.WithError(err).Error("Failed to get head state")
		return
	}
	if !s.inRegularSync() {
		return
	}
	attr := s.getLatePayloadAttribute(ctx, st, currentSlot+1, r)
	if attr == nil || attr.IsEmpty() {
		return
	}
	beaconLatePayloadTaskTriggeredTotal.Inc()
	bh, err := st.LatestBlockHash()
	if err != nil {
		log.WithError(err).Error("Could not get latest block hash")
		return
	}
	pid, err := s.notifyForkchoiceUpdateGloas(ctx, bh, attr)
	if err != nil {
		log.WithError(err).Error("Could not notify forkchoice update")
		return
	}
	if pid == nil {
		log.Warn("Received nil payload ID from forkchoice update.")
		return
	}
	var pId [8]byte
	copy(pId[:], pid[:])
	s.cfg.PayloadIDCache.Set(currentSlot+1, hr, pId)
}

func (s *Service) fcuFromReorgData(hr [32]byte, hash [32]byte, attr payloadattribute.Attributer, proposingSlot primitives.Slot) {
	pid, err := s.notifyForkchoiceUpdateGloas(s.ctx, hash, attr)
	if err != nil {
		log.WithError(err).Error("Could not update forkchoice with engine")
	}
	if pid == nil {
		if !attr.IsEmpty() {
			log.Warn("Engine did not return a payload ID for the fork choice update with attributes")
		}
		return
	}
	var pId [8]byte
	copy(pId[:], pid[:])
	s.cfg.PayloadIDCache.Set(proposingSlot, hr, pId)
}

// This saves head and prunes atts from the pool only if the head is new and if we are either
// 1. Not proposing next slot or, if we are,
// 2. The incoming head block is not late.
// If we are going to attempt to reorg the block we do not save head in the blockchain package
// and continue treating the previous head as the tip of the chain.
func (s *Service) saveHeadIfNeeded(ctx context.Context, cfg *postBlockProcessConfig) {
	full := false
	if !s.isNewHead(cfg.headRoot, full) {
		return
	}
	proposingSlot := s.CurrentSlot() + 1
	if s.shouldOverrideFCU(cfg.headRoot, proposingSlot) {
		attr := s.getPayloadAttribute(ctx, cfg.postState, proposingSlot, cfg.headRoot[:], full)
		if !attr.IsEmpty() {
			return
		}
	}
	if err := s.saveHead(ctx, cfg.headRoot, cfg.roblock, cfg.postState, full); err != nil {
		log.WithError(err).Error("Could not save head")
	}
	s.pruneAttsFromPool(ctx, cfg.postState, cfg.roblock)
}
