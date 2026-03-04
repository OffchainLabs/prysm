package sync

import (
	"context"
	stderrors "errors"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/pkg/errors"
)

var errNilPartialDataColumn = errors.New("nil partial data column")
var errHeaderEmptyCommitments = errors.New("header has no kzg commitments")

func (s *Service) partialVerifierFromTrustedColumn(ctx context.Context, col *blocks.PartialDataColumn) (*verification.PartialColumnVerifier,
	error) {
	if col == nil || col.SignedBlockHeader == nil || col.SignedBlockHeader.Header == nil {
		return nil, errNilPartialDataColumn
	}

	if len(col.KzgCommitments) == 0 {
		return nil, errHeaderEmptyCommitments
	}

	roDataColumn, err := blocks.NewRODataColumn(col.DataColumnSidecar)
	if err != nil {
		return nil, err
	}

	dcv := s.newColumnsVerifier([]blocks.RODataColumn{roDataColumn}, verification.PartialColumnRequirements)
	verifier := verification.NewPartialColumnVerifier(dcv, col)
	verifier.MarkIncludedCellsVerified()

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
func (s *Service) validatePartialDataColumnHeader(ctx context.Context, col *blocks.PartialDataColumn) (*verification.PartialColumnVerifier,
	bool, error) {
	if col == nil || col.SignedBlockHeader == nil || col.SignedBlockHeader.Header == nil {
		return nil, false, errNilPartialDataColumn
	}

	if len(col.KzgCommitments) == 0 {
		return nil, true, errHeaderEmptyCommitments
	}

	roDataColumn, err := blocks.NewRODataColumn(col.DataColumnSidecar)
	if err != nil {
		return nil, false, err
	}

	dcv := s.newColumnsVerifier([]blocks.RODataColumn{roDataColumn}, verification.PartialColumnRequirements)
	verifier := verification.NewPartialColumnVerifier(dcv, col)

	if err := verifier.NotFromFutureSlot(); err != nil {
		return verifier, false, err
	}

	if err := verifier.SlotAboveFinalized(); err != nil {
		return verifier, false, err
	}

	if err := verifier.SidecarParentSeen(s.hasBadBlock); err != nil {
		return verifier, false, err
	}

	if err := verifier.SidecarParentValid(s.hasBadBlock); err != nil {
		return verifier, true, err
	}

	if err := verifier.SidecarParentSlotLower(); err != nil {
		if stderrors.Is(err, verification.ErrSidecarParentSlotUnavailable) {
			return verifier, false, err
		}
		return verifier, true, err
	}

	if err := verifier.SidecarDescendsFromFinalized(); err != nil {
		return verifier, true, err
	}

	if err := verifier.SidecarInclusionProven(); err != nil {
		return verifier, true, err
	}

	if err := verifier.SidecarProposerExpected(ctx); err != nil {
		return verifier, true, err
	}

	if err := verifier.ValidProposerSignature(ctx); err != nil {
		return verifier, true, err
	}

	return verifier, false, nil
}
