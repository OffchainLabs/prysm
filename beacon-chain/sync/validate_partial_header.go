package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

var (
	// REJECT errors - peer should be penalized
	errHeaderEmptyCommitments       = errors.New("header has no kzg commitments")
	errHeaderParentInvalid          = errors.New("header parent invalid")
	errHeaderSlotNotAfterParent     = errors.New("header slot not after parent")
	errHeaderNotFinalizedDescendant = errors.New("header not finalized descendant")
	errHeaderInvalidInclusionProof  = errors.New("invalid inclusion proof")
	errHeaderInvalidSignature       = errors.New("invalid proposer signature")
	errHeaderUnexpectedProposer     = errors.New("unexpected proposer index")

	// IGNORE errors - don't penalize peer
	errHeaderNil               = errors.New("nil header")
	errHeaderFromFuture        = errors.New("header is from future slot")
	errHeaderNotAboveFinalized = errors.New("header slot not above finalized")
	errHeaderParentNotSeen     = errors.New("header parent not seen")
)

// validatePartialDataColumnHeader validates a PartialDataColumnHeader per the consensus spec.
// Returns (reject, err) where reject=true means the peer should be penalized.
// TODO: we should consolidate this with the existing DataColumn validation pipeline.
func (s *Service) validatePartialDataColumnHeader(ctx context.Context, header *ethpb.PartialDataColumnHeader) (reject bool, err error) {
	if header == nil || header.SignedBlockHeader == nil || header.SignedBlockHeader.Header == nil {
		return false, errHeaderNil // IGNORE
	}

	blockHeader := header.SignedBlockHeader.Header
	headerSlot := blockHeader.Slot
	parentRoot := bytesutil.ToBytes32(blockHeader.ParentRoot)

	// [REJECT] kzg_commitments list is non-empty
	if len(header.KzgCommitments) == 0 {
		return true, errHeaderEmptyCommitments
	}

	// [IGNORE] Not from future slot (with MAXIMUM_GOSSIP_CLOCK_DISPARITY allowance)
	currentSlot := s.cfg.clock.CurrentSlot()
	if headerSlot > currentSlot {
		maxDisparity := params.BeaconConfig().MaximumGossipClockDisparityDuration()
		slotStart, err := s.cfg.clock.SlotStart(headerSlot)
		if err != nil {
			return false, err
		}
		if s.cfg.clock.Now().Before(slotStart.Add(-maxDisparity)) {
			return false, errHeaderFromFuture // IGNORE
		}
	}

	// [IGNORE] Slot above finalized
	finalizedCheckpoint := s.cfg.chain.FinalizedCheckpt()
	startSlot, err := slots.EpochStart(finalizedCheckpoint.Epoch)
	if err != nil {
		return false, err
	}
	if headerSlot <= startSlot {
		return false, errHeaderNotAboveFinalized // IGNORE
	}

	// [IGNORE] Parent has been seen
	if !s.cfg.chain.HasBlock(ctx, parentRoot) {
		return false, errHeaderParentNotSeen // IGNORE
	}

	// [REJECT] Parent passes validation (not a bad block)
	if s.hasBadBlock(parentRoot) {
		return true, errHeaderParentInvalid
	}

	// [REJECT] Header slot > parent slot
	parentSlot, err := s.cfg.chain.RecentBlockSlot(parentRoot)
	if err != nil {
		return false, errors.Wrap(err, "get parent slot")
	}
	if headerSlot <= parentSlot {
		return true, errHeaderSlotNotAfterParent
	}

	// [REJECT] Finalized checkpoint is ancestor (parent is in forkchoice)
	if !s.cfg.chain.InForkchoice(parentRoot) {
		return true, errHeaderNotFinalizedDescendant
	}

	// [REJECT] Inclusion proof valid
	if err := peerdas.VerifyPartialDataColumnHeaderInclusionProof(header); err != nil {
		return true, errHeaderInvalidInclusionProof
	}

	// [REJECT] Valid proposer signature
	parentState, err := s.cfg.stateGen.StateByRoot(ctx, parentRoot)
	if err != nil {
		return false, errors.Wrap(err, "get parent state")
	}

	proposerIdx := blockHeader.ProposerIndex
	proposer, err := parentState.ValidatorAtIndex(proposerIdx)
	if err != nil {
		return false, errors.Wrap(err, "get proposer")
	}

	domain, err := signing.Domain(
		parentState.Fork(),
		slots.ToEpoch(headerSlot),
		params.BeaconConfig().DomainBeaconProposer,
		parentState.GenesisValidatorsRoot(),
	)
	if err != nil {
		return false, errors.Wrap(err, "get domain")
	}

	if err := signing.VerifyBlockHeaderSigningRoot(
		blockHeader,
		proposer.PublicKey,
		header.SignedBlockHeader.Signature,
		domain,
	); err != nil {
		return true, errHeaderInvalidSignature
	}

	// [REJECT] Expected proposer for slot
	expectedProposer, err := helpers.BeaconProposerIndexAtSlot(ctx, parentState, headerSlot)
	if err != nil {
		return false, errors.Wrap(err, "compute expected proposer")
	}
	if expectedProposer != proposerIdx {
		return true, errHeaderUnexpectedProposer
	}

	return false, nil // Valid header
}
