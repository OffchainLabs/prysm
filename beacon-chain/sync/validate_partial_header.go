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
var errColumnNotFulu = errors.New("partial column is not a fulu type")

func (s *Service) partialVerifierFromTrustedColumn(_ context.Context, col *blocks.PartialDataColumn) (*verification.PartialColumnVerifier, error) {
	if col == nil {
		return nil, errHeaderNil
	}
	// Gloas partial columns carry no signed block header or inclusion proof, so the Fulu
	// header path does not apply. Seed a verifier from the bid commitments instead.
	if col.IsGloas() {
		return s.partialVerifierFromTrustedGloasColumn(col)
	}
	sbh, err := col.SignedBlockHeader()
	if err != nil {
		return nil, errColumnNotFulu
	}
	if sbh == nil || sbh.Header == nil {
		return nil, errHeaderNil
	}

	if col.KzgCommitmentCount() == 0 {
		return nil, errHeaderEmptyCommitments
	}

	dcv := s.newColumnsVerifier([]blocks.RODataColumn{col.RODataColumn}, verification.PartialColumnRequirements)
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

// partialVerifierFromTrustedGloasColumn builds a partial column verifier for a trusted Gloas
// partial data column. Gloas columns have no header to check; the column's bid commitments must
// already be set, and only the fork-neutral field/KZG requirements are used (satisfied as the
// column completes). The Fulu header-check requirements do not apply.
func (s *Service) partialVerifierFromTrustedGloasColumn(col *blocks.PartialDataColumn) (*verification.PartialColumnVerifier, error) {
	if col.KzgCommitmentCount() == 0 {
		return nil, errHeaderEmptyCommitments
	}
	dcv := s.newColumnsVerifier([]blocks.RODataColumn{col.RODataColumn}, verification.GloasPartialColumnRequirements)
	return verification.NewPartialColumnVerifier(dcv, col), nil
}

// validatePartialDataColumn validates only the header-applicable checks for a partial data column.
func (s *Service) validatePartialDataColumnHeader(ctx context.Context, col *blocks.PartialDataColumn) (*verification.PartialColumnVerifier, pubsub.ValidationResult, error) {
	if col == nil {
		return nil, pubsub.ValidationIgnore, errHeaderNil
	}
	sbh, err := col.SignedBlockHeader()
	if err != nil {
		return nil, pubsub.ValidationIgnore, errColumnNotFulu
	}
	if sbh == nil || sbh.Header == nil {
		return nil, pubsub.ValidationIgnore, errHeaderNil
	}

	// [REJECT] kzg_commitments list is non-empty
	if col.KzgCommitmentCount() == 0 {
		return nil, pubsub.ValidationReject, errHeaderEmptyCommitments
	}

	dcv := s.newColumnsVerifier([]blocks.RODataColumn{col.RODataColumn}, verification.PartialColumnRequirements)
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
	parentRoot := bytesutil.ToBytes32(sbh.Header.ParentRoot)
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
