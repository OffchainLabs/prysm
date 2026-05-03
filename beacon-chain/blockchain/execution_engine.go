package blockchain

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/async/event"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	blocktypes "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var defaultLatestValidHash = bytesutil.PadTo([]byte{0xff}, 32)

// notifyForkchoiceUpdate signals execution engine the fork choice updates. Execution engine should:
// 1. Re-organizes the execution payload chain and corresponding state to make head_block_hash the head.
// 2. Applies finality to the execution state: it irreversibly persists the chain of all execution payloads and corresponding state, up to and including finalized_block_hash.
func (s *Service) notifyForkchoiceUpdate(ctx context.Context, arg *fcuConfig) (*enginev1.PayloadIDBytes, error) {
	return nil, nil
}

func (s *Service) firePayloadAttributesEvent(f event.SubscriberSender, block interfaces.ReadOnlySignedBeaconBlock, root [32]byte, nextSlot primitives.Slot) {
	// If we're syncing a block in the past and init-sync is still running, we shouldn't fire this event.
	if !s.cfg.SyncChecker.Synced() {
		return
	}
	// the fcu args have differing amounts of completeness based on the code path,
	// and there is work we only want to do if a client is actually listening to the events beacon api endpoint.
	// temporary solution: just fire a blank event and fill in the details in the api handler.
	f.Send(&feed.Event{
		Type: statefeed.PayloadAttributes,
		Data: payloadattribute.EventData{HeadBlock: block, HeadRoot: root, ProposalSlot: nextSlot},
	})
}

// getPayloadHash returns the payload hash given the block root.
// if the block is before bellatrix fork epoch, it returns the zero hash.
func (s *Service) getPayloadHash(ctx context.Context, root []byte) ([32]byte, error) {
	blk, err := s.getBlock(ctx, s.ensureRootNotZeros(bytesutil.ToBytes32(root)))
	if err != nil {
		return [32]byte{}, err
	}
	if blocks.IsPreBellatrixVersion(blk.Block().Version()) {
		return params.BeaconConfig().ZeroHash, nil
	}
	payload, err := blk.Block().Body().Execution()
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not get execution payload")
	}
	return bytesutil.ToBytes32(payload.BlockHash()), nil
}

// notifyNewPayload signals execution engine on a new payload.
// It returns true if the EL has returned VALID for the block
// stVersion should represent the version of the pre-state; header should also be from the pre-state.
func (s *Service) notifyNewPayload(ctx context.Context, stVersion int, header interfaces.ExecutionData, blk blocktypes.ROBlock) (bool, error) {
	return true, nil
}

// pruneInvalidBlock deals with the event that an invalid block was detected by the execution layer
func (s *Service) pruneInvalidBlock(ctx context.Context, root, parentRoot, parentHash [32]byte, lvh [32]byte) error {
	newPayloadInvalidNodeCount.Inc()
	invalidRoots, err := s.cfg.ForkChoiceStore.SetOptimisticToInvalid(ctx, root, parentRoot, parentHash, lvh)
	if err != nil {
		return err
	}
	if err := s.removeInvalidBlockAndState(ctx, invalidRoots); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		"blockRoot":            fmt.Sprintf("%#x", root),
		"invalidChildrenCount": len(invalidRoots),
	}).Warn("Pruned invalid blocks")
	return invalidBlock{
		invalidAncestorRoots: invalidRoots,
		error:                ErrInvalidPayload,
		lastValidHash:        lvh,
	}
}

// getPayloadAttributes returns the payload attributes for the given state and slot.
// The attribute is required to initiate a payload build process in the context of an `engine_forkchoiceUpdated` call.
func (s *Service) getPayloadAttribute(ctx context.Context, st state.BeaconState, slot primitives.Slot, headRoot []byte, headFull bool) payloadattribute.Attributer {
	emptyAttri := payloadattribute.EmptyWithVersion(st.Version())

	// If it is an epoch boundary then process slots to get the right
	// shuffling before checking if the proposer is tracked. Otherwise
	// perform this check before. This is cheap as the NSC has already been updated.
	var val cache.TrackedValidator
	var ok bool
	e := slots.ToEpoch(slot)
	stateEpoch := slots.ToEpoch(st.Slot())
	fuluAndNextEpoch := st.Version() >= version.Fulu && e == stateEpoch+1
	if e == stateEpoch || fuluAndNextEpoch {
		val, ok = s.trackedProposer(st, slot)
		if !ok {
			return emptyAttri
		}
	}
	if slot > st.Slot() {
		// At this point either we know we are proposing on a future slot or we need to still compute the
		// right proposer index pre-Fulu, either way we need to copy the state to process it.
		st = st.Copy()
		var err error
		st, err = transition.ProcessSlotsUsingNextSlotCache(ctx, st, headRoot, slot)
		if err != nil {
			log.WithError(err).Error("Could not process slots to get payload attribute")
			return emptyAttri
		}
	}
	if e > stateEpoch && !fuluAndNextEpoch {
		emptyAttri := payloadattribute.EmptyWithVersion(st.Version())
		val, ok = s.trackedProposer(st, slot)
		if !ok {
			return emptyAttri
		}
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

	v := st.Version()
	switch {
	case v >= version.Gloas:
		withdrawals, err := s.computePayloadWithdrawals(ctx, st, bytesutil.ToBytes32(headRoot), headFull)
		if err != nil {
			log.WithError(err).Error("Could not get withdrawals for payload attribute")
			return emptyAttri
		}
		return payloadAttributesGloas(uint64(t.Unix()), prevRando, val.FeeRecipient[:], headRoot, withdrawals, slot)
	case v >= version.Deneb:
		return payloadAttributesDeneb(st, uint64(t.Unix()), prevRando, val.FeeRecipient[:], headRoot)
	case v >= version.Capella:
		return payloadAttributesCapella(st, uint64(t.Unix()), prevRando, val.FeeRecipient[:])
	case v >= version.Bellatrix:
		return payloadAttributesBellatrix(uint64(t.Unix()), prevRando, val.FeeRecipient[:])
	default:
		log.WithField("version", version.String(v)).Error("Could not get payload attribute due to unknown state version")
		return payloadattribute.EmptyWithVersion(v)
	}
}

func payloadAttributesGloas(timestamp uint64, prevRandao, feeRecipient, parentBeaconBlockRoot []byte, withdrawals []*enginev1.Withdrawal, slot primitives.Slot) payloadattribute.Attributer {
	attr, err := payloadattribute.New(&enginev1.PayloadAttributesV4{
		Timestamp:             timestamp,
		PrevRandao:            prevRandao,
		SuggestedFeeRecipient: feeRecipient,
		Withdrawals:           withdrawals,
		ParentBeaconBlockRoot: parentBeaconBlockRoot,
		SlotNumber:            uint64(slot),
	})
	if err != nil {
		log.WithError(err).Error("Could not get payload attribute")
		return payloadattribute.EmptyWithVersion(version.Gloas)
	}
	return attr
}

func payloadAttributesDeneb(st state.BeaconState, timestamp uint64, prevRandao, feeRecipient, parentBeaconBlockRoot []byte) payloadattribute.Attributer {
	withdrawals, _, err := st.ExpectedWithdrawals()
	if err != nil {
		log.WithError(err).Error("Could not get expected withdrawals to get payload attribute")
		return payloadattribute.EmptyWithVersion(st.Version())
	}
	attr, err := payloadattribute.New(&enginev1.PayloadAttributesV3{
		Timestamp:             timestamp,
		PrevRandao:            prevRandao,
		SuggestedFeeRecipient: feeRecipient,
		Withdrawals:           withdrawals,
		ParentBeaconBlockRoot: parentBeaconBlockRoot,
	})
	if err != nil {
		log.WithError(err).Error("Could not get payload attribute")
		return payloadattribute.EmptyWithVersion(st.Version())
	}
	return attr
}

func payloadAttributesCapella(st state.BeaconState, timestamp uint64, prevRandao, feeRecipient []byte) payloadattribute.Attributer {
	withdrawals, _, err := st.ExpectedWithdrawals()
	if err != nil {
		log.WithError(err).Error("Could not get expected withdrawals to get payload attribute")
		return payloadattribute.EmptyWithVersion(st.Version())
	}
	attr, err := payloadattribute.New(&enginev1.PayloadAttributesV2{
		Timestamp:             timestamp,
		PrevRandao:            prevRandao,
		SuggestedFeeRecipient: feeRecipient,
		Withdrawals:           withdrawals,
	})
	if err != nil {
		log.WithError(err).Error("Could not get payload attribute")
		return payloadattribute.EmptyWithVersion(st.Version())
	}
	return attr
}

func payloadAttributesBellatrix(timestamp uint64, prevRandao, feeRecipient []byte) payloadattribute.Attributer {
	attr, err := payloadattribute.New(&enginev1.PayloadAttributes{
		Timestamp:             timestamp,
		PrevRandao:            prevRandao,
		SuggestedFeeRecipient: feeRecipient,
	})
	if err != nil {
		log.WithError(err).Error("Could not get payload attribute")
		return payloadattribute.EmptyWithVersion(version.Bellatrix)
	}
	return attr
}

// removeInvalidBlockAndState removes the invalid block, blob and its corresponding state from the cache and DB.
func (s *Service) removeInvalidBlockAndState(ctx context.Context, blkRoots [][32]byte) error {
	for _, root := range blkRoots {
		if err := s.cfg.StateGen.DeleteStateFromCaches(ctx, root); err != nil {
			return err
		}
		// Delete block also deletes the state as well.
		if err := s.cfg.BeaconDB.DeleteBlock(ctx, root); err != nil {
			// TODO(10487): If a caller requests to delete a root that's justified and finalized. We should gracefully shutdown.
			// This is an irreparable condition, it would me a justified or finalized block has become invalid.
			return err
		}
		if err := s.blobStorage.Remove(root); err != nil {
			// Blobs may not exist for some blocks, leading to deletion failures. Log such errors at debug level.
			log.WithError(err).Debug("Could not remove blob from blob storage")
		}
		if err := s.dataColumnStorage.Remove(root); err != nil {
			log.WithError(err).Errorf("Could not remove data columns from data column storage for root %#x", root)
		}
	}
	return nil
}

func kzgCommitmentsToVersionedHashes(body interfaces.ReadOnlyBeaconBlockBody) ([]common.Hash, error) {
	commitments, err := body.BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(invalidBlock{error: err}, "could not get blob kzg commitments")
	}

	versionedHashes := make([]common.Hash, len(commitments))
	for i, commitment := range commitments {
		versionedHashes[i] = primitives.ConvertKzgCommitmentToVersionedHash(commitment)
	}
	return versionedHashes, nil
}
