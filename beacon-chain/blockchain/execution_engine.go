package blockchain

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v6/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
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
func (s *Service) notifyNewPayload(ctx context.Context, preStateVersion int,
	preStateHeader interfaces.ExecutionData, blk interfaces.ReadOnlySignedBeaconBlock) (bool, error) {
	return true, nil
}

// reportInvalidBlock deals with the event that an invalid block was detected by the execution layer
func (s *Service) pruneInvalidBlock(ctx context.Context, root, parentRoot, lvh [32]byte) error {
	newPayloadInvalidNodeCount.Inc()
	invalidRoots, err := s.cfg.ForkChoiceStore.SetOptimisticToInvalid(ctx, root, parentRoot, lvh)
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
func (s *Service) getPayloadAttribute(ctx context.Context, st state.BeaconState, slot primitives.Slot, headRoot []byte) payloadattribute.Attributer {
	emptyAttri := payloadattribute.EmptyWithVersion(st.Version())

	// If it is an epoch boundary then process slots to get the right
	// shuffling before checking if the proposer is tracked. Otherwise
	// perform this check before. This is cheap as the NSC has already been updated.
	var val cache.TrackedValidator
	var ok bool
	e := slots.ToEpoch(slot)
	stateEpoch := slots.ToEpoch(st.Slot())
	if e == stateEpoch {
		val, ok = s.trackedProposer(st, slot)
		if !ok {
			return emptyAttri
		}
	}
	st = st.Copy()
	if slot > st.Slot() {
		var err error
		st, err = transition.ProcessSlotsUsingNextSlotCache(ctx, st, headRoot, slot)
		if err != nil {
			log.WithError(err).Error("Could not process slots to get payload attribute")
			return emptyAttri
		}
	}
	if e > stateEpoch {
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
	t, err := slots.ToTime(uint64(s.genesisTime.Unix()), slot)
	if err != nil {
		log.WithError(err).Error("Could not get timestamp to get payload attribute")
		return emptyAttri
	}

	v := st.Version()

	if v >= version.Deneb {
		withdrawals, _, err := st.ExpectedWithdrawals()
		if err != nil {
			log.WithError(err).Error("Could not get expected withdrawals to get payload attribute")
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

	if v >= version.Capella {
		withdrawals, _, err := st.ExpectedWithdrawals()
		if err != nil {
			log.WithError(err).Error("Could not get expected withdrawals to get payload attribute")
			return emptyAttri
		}

		attr, err := payloadattribute.New(&enginev1.PayloadAttributesV2{
			Timestamp:             uint64(t.Unix()),
			PrevRandao:            prevRando,
			SuggestedFeeRecipient: val.FeeRecipient[:],
			Withdrawals:           withdrawals,
		})
		if err != nil {
			log.WithError(err).Error("Could not get payload attribute")
			return emptyAttri
		}

		return attr
	}

	if v >= version.Bellatrix {
		attr, err := payloadattribute.New(&enginev1.PayloadAttributes{
			Timestamp:             uint64(t.Unix()),
			PrevRandao:            prevRando,
			SuggestedFeeRecipient: val.FeeRecipient[:],
		})
		if err != nil {
			log.WithError(err).Error("Could not get payload attribute")
			return emptyAttri
		}

		return attr
	}

	log.WithField("version", version.String(st.Version())).Error("Could not get payload attribute due to unknown state version")
	return emptyAttri
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
