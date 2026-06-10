package sync

import (
	"context"
	stderrors "errors"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

var errHeaderEmptyCommitments = errors.New("header has no kzg commitments")
var errHeaderParentNotSeen = errors.New("header parent not seen")
var errHeaderNil = errors.New("nil header")

func (s *Service) partialVerifierFromTrustedColumn(ctx context.Context, col *blocks.PartialDataColumn) (*verification.PartialColumnVerifier, error) {
	if col == nil || col.SignedBlockHeader == nil || col.SignedBlockHeader.Header == nil {
		return nil, errHeaderNil
	}

	if len(col.KzgCommitments) == 0 {
		return nil, errHeaderEmptyCommitments
	}

	roDataColumn, err := blocks.NewRODataColumn(col.DataColumnSidecar)
	if err != nil {
		return nil, errors.Wrap(err, "roDataColumn conversion failure")
	}

	dcv := s.newColumnsVerifier([]blocks.RODataColumn{roDataColumn}, verification.PartialColumnRequirements)
	verifier := verification.NewPartialColumnVerifier(dcv, col)

	// mark all header checks as completed
	verifier.SatisfyRequirement(verification.RequireNotFromFutureSlot)
	verifier.SatisfyRequirement(verification.RequireSlotAboveFinalized)
	verifier.SatisfyRequirement(verification.RequireSidecarParentSeen)
	verifier.SatisfyRequirement(verification.RequireSidecarParentValid)
	verifier.SatisfyRequirement(verification.RequireSidecarParentSlotLower)
	verifier.SatisfyRequirement(verification.RequireSidecarDescendsFromFinalized)
	verifier.SatisfyRequirement(verification.RequireSidecarInclusionProven)
	verifier.SatisfyRequirement(verification.RequireSidecarProposerExpected)
	verifier.SatisfyRequirement(verification.RequireValidProposerSignature)

	return verifier, nil
}

// validatePartialDataColumn validates only the header-applicable checks for a partial data column.
func (s *Service) validatePartialDataColumnHeader(ctx context.Context, col *blocks.PartialDataColumn) (*verification.PartialColumnVerifier, pubsub.ValidationResult, error) {
	if col == nil || col.SignedBlockHeader == nil || col.SignedBlockHeader.Header == nil {
		return nil, pubsub.ValidationIgnore, errHeaderNil
	}

	// [REJECT] kzg_commitments list is non-empty
	if len(col.KzgCommitments) == 0 {
		return nil, pubsub.ValidationReject, errHeaderEmptyCommitments
	}

	roDataColumn, err := blocks.NewRODataColumn(col.DataColumnSidecar)
	if err != nil {
		return nil, pubsub.ValidationIgnore, errors.Wrap(err, "roDataColumn conversion failure")
	}

	dcv := s.newColumnsVerifier([]blocks.RODataColumn{roDataColumn}, verification.PartialColumnRequirements)
	verifier := verification.NewPartialColumnVerifier(dcv, col)

	// [IGNORE] Not from future slot (with MAXIMUM_GOSSIP_CLOCK_DISPARITY allowance)
	if err := verifier.NotFromFutureSlot(); err != nil {
		return verifier, pubsub.ValidationIgnore, errors.Wrap(err, "partial data column header validation")
	}

	// [IGNORE] Slot above finalized
	if err := verifier.SlotAboveFinalized(); err != nil {
		return verifier, pubsub.ValidationIgnore, errors.Wrap(err, "partial data column header validation")
	}

	// [IGNORE] Parent has been seen
	parentRoot := bytesutil.ToBytes32(col.SignedBlockHeader.Header.ParentRoot)
	if !s.cfg.chain.HasBlock(ctx, parentRoot) {
		return verifier, pubsub.ValidationIgnore, errHeaderParentNotSeen
	}

	if err := verifier.SidecarParentSeen(s.hasBadBlock); err != nil {
		return verifier, pubsub.ValidationIgnore, errors.Wrap(err, "partial data column header validation")
	}

	// [REJECT] Parent passes validation (not a bad block)
	if err := verifier.SidecarParentValid(s.hasBadBlock); err != nil {
		return verifier, pubsub.ValidationReject, errors.Wrap(err, "partial data column header validation")
	}

	// [REJECT] Header slot > parent slot
	if err := verifier.SidecarParentSlotLower(); err != nil {
		if stderrors.Is(err, verification.ErrSidecarParentUnknown) {
			return verifier, pubsub.ValidationIgnore, errors.Wrap(err, "partial data column header validation")
		}
		return verifier, pubsub.ValidationReject, errors.Wrap(err, "partial data column header validation")
	}

	// [REJECT] Finalized checkpoint is ancestor (parent is in forkchoice)
	if err := verifier.SidecarDescendsFromFinalized(); err != nil {
		return verifier, pubsub.ValidationReject, errors.Wrap(err, "partial data column header validation")
	}

	// [REJECT] Inclusion proof valid
	if err := verifier.SidecarInclusionProven(); err != nil {
		return verifier, pubsub.ValidationReject, errors.Wrap(err, "partial data column header validation")
	}

	// [REJECT] Expected proposer for slot
	if err := verifier.SidecarProposerExpected(ctx); err != nil {
		return verifier, pubsub.ValidationReject, errors.Wrap(err, "partial data column header validation")
	}

	// [REJECT] Valid proposer signature
	if err := verifier.ValidProposerSignature(ctx); err != nil {
		return verifier, pubsub.ValidationReject, errors.Wrap(err, "partial data column header validation")
	}

	return verifier, pubsub.ValidationAccept, nil
}
