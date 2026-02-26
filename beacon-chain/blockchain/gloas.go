package blockchain

import (
	"context"
	"math"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensus_blocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
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

// getLookupParentRoot returns the root that serves as key to generate the parent state for the passed beacon block.
// if it is based on empty or it is pre-Gloas, it is the parent root of the block, otherwise if it is based on full it is
// the parent hash.
// The caller of this function should not hold a lock on forkchoice.
func (s *Service) getLookupParentRoot(b consensus_blocks.ROBlock) ([32]byte, error) {
	bl := b.Block()
	parentRoot := bl.ParentRoot()
	parentSlot, err := s.cfg.ForkChoiceStore.Slot(parentRoot)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "failed to get slot for parent root")
	}
	if slots.ToEpoch(parentSlot) < params.BeaconConfig().GloasForkEpoch {
		return parentRoot, nil
	}
	if b.Version() < version.Gloas {
		return parentRoot, nil
	}

	if slots.ToEpoch(parentSlot) < params.BeaconConfig().GloasForkEpoch {
		return parentRoot, nil
	}
	blockHash, err := s.cfg.ForkChoiceStore.BlockHash(parentRoot)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "failed to get block hash for parent root")
	}
	bid, err := bl.Body().SignedExecutionPayloadBid()
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "failed to get signed execution payload bid from block body")
	}
	if bid == nil || bid.Message == nil || len(bid.Message.ParentBlockHash) != 32 {
		return [32]byte{}, errors.New("invalid signed execution payload bid message")
	}
	parentHash := [32]byte(bid.Message.ParentBlockHash)
	if blockHash == parentHash {
		return parentHash, nil
	}
	return parentRoot, nil
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

// This is a Gloas version of getPayloadAttribute that avoids all the clutter that was originally due to the proposer Index.
// It is guaranteed to be called for the current slot + 1 and the head state to have been advanced to at least the current epoch.
func (s *Service) getPayloadAttributeGloas(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot, headRoot, accessRoot []byte) payloadattribute.Attributer {
	emptyAttri := payloadattribute.EmptyWithVersion(st.Version())
	val, proposing := s.checkIfProposing(st, slot)
	if !proposing {
		return emptyAttri
	}

	st, err := transition.ProcessSlotsIfNeeded(ctx, st, accessRoot, slot)
	if err != nil {
		log.WithError(err).Error("Could not process slots to get payload attribute")
		return emptyAttri
	}

	// Get previous randao.
	prevRando, err := helpers.RandaoMix(st, time.CurrentEpoch(st))
	if err != nil {
		log.WithError(err).Error("Could not get randao mix to get payload attribute")
		return emptyAttri
	}

	// Get timestamp.
	t, err := slots.StartTime(s.genesisTime, slot)
	if err != nil {
		log.WithError(err).Error("Could not get timestamp to get payload attribute")
		return emptyAttri
	}

	withdrawals, err := st.WithdrawalsForPayload()
	if err != nil {
		log.WithError(err).Error("Could not get payload withdrawals to get payload attribute")
		return emptyAttri
	}

	attr, err := payloadattribute.New(&enginev1.PayloadAttributesV3{
		Timestamp:             uint64(t.Unix()),
		PrevRandao:            prevRando,
		SuggestedFeeRecipient: val.FeeRecipient[:],
		Withdrawals:           withdrawals,
		ParentBeaconBlockRoot: headRoot,
	})
	if err != nil {
		log.WithError(err).Error("Could not get payload attribute")
		return emptyAttri
	}
	return attr
}

// latePayloadTasks updates the NSC and epoch boundary caches when there is no payload in the current slot (and there is a block)
// The case where the block was also missing would have been dealt by lateBlockTasks already.
// We call FCU only if we are proposing next slot, as the execution head is assumed to not have changed.
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
	attr := s.getPayloadAttributeGloas(ctx, st, currentSlot+1, r, r)
	if attr == nil || attr.IsEmpty() {
		return
	}
	beaconLatePayloadTaskTriggeredTotal.Inc()
	// Head is the empty block.
	bh, err := st.LatestBlockHash()
	if err != nil {
		log.WithError(err).Error("Could not get latest block hash to notify engine")
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
