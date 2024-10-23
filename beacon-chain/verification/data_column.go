package verification

import (
	"context"

	"github.com/pkg/errors"
	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/runtime/logging"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

var allColumnSidecarRequirements = []Requirement{
	RequireDataColumnIndexInBounds,
	RequireNotFromFutureSlot,
	RequireSlotAboveFinalized,
	RequireValidProposerSignature,
	RequireSidecarParentSeen,
	RequireSidecarParentValid,
	RequireSidecarParentSlotLower,
	RequireSidecarDescendsFromFinalized,
	RequireSidecarInclusionProven,
	RequireSidecarKzgProofVerified,
	RequireSidecarProposerExpected,
}

// GossipColumnSidecarRequirements defines the set of requirements that DataColumnSidecars received on gossip
// must satisfy in order to upgrade an RODataColumn to a VerifiedRODataColumn.
var GossipColumnSidecarRequirements = requirementList(allColumnSidecarRequirements).excluding()

// SpectestColumnSidecarRequirements is used by the forkchoice spectests when verifying blobs used in the on_block tests.
// The only requirements we exclude for these tests are the parent validity and seen tests, as these are specific to
// gossip processing and require the bad block cache that we only use there.
var SpectestColumnSidecarRequirements = requirementList(GossipColumnSidecarRequirements).excluding(
	RequireSidecarParentSeen, RequireSidecarParentValid)

// SamplingColumnSidecarRequirements are the column verification requirements that are necessary for columns
// received via sampling.
var SamplingColumnSidecarRequirements = requirementList(allColumnSidecarRequirements).excluding(
	RequireNotFromFutureSlot,
	RequireSlotAboveFinalized,
	RequireSidecarParentSeen,
	RequireSidecarParentValid,
	RequireSidecarParentSlotLower,
	RequireSidecarDescendsFromFinalized,
	RequireSidecarProposerExpected,
)

// InitsyncColumnSidecarRequirements is the list of verification requirements to be used by the init-sync service
// for batch-mode syncing. Because we only perform batch verification as part of the IsDataAvailable method
// for data columns after the block has been verified, and the blobs to be verified are keyed in the cache by the
// block root, the list of required verifications is much shorter than gossip.
var InitsyncColumnSidecarRequirements = requirementList(SamplingColumnSidecarRequirements).excluding()

// BackfillColumnSidecarRequirements is the same as InitsyncColumnSidecarRequirements.
var BackfillColumnSidecarRequirements = requirementList(InitsyncColumnSidecarRequirements).excluding()

// PendingQueueColumnSidecarRequirements is the same as InitsyncColumnSidecarRequirements, used by the pending blocks queue.
var PendingQueueColumnSidecarRequirements = requirementList(InitsyncColumnSidecarRequirements).excluding()

var (
	// ErrColumnIndexInvalid means the column failed verification.
	ErrColumnInvalid = errors.New("data column failed verification")
	// ErrColumnIndexInvalid means RequireDataColumnIndexInBounds failed.
	ErrColumnIndexInvalid = errors.New("incorrect column sidecar index")
)

type RODataColumnsVerifier struct {
	*sharedResources
	results                     *results
	dataColumns                 []blocks.RODataColumn
	verifyDataColumnsCommitment rodataColumnsCommitmentVerifier
}

type rodataColumnsCommitmentVerifier func([]blocks.RODataColumn) (bool, error)

var _ DataColumnsVerifier = &RODataColumnsVerifier{}

// VerifiedRODataColumns "upgrades" the wrapped ROBlob to a VerifiedROBlob.
// If any of the verifications ran against the blob failed, or some required verifications
// were not run, an error will be returned.
func (dv *RODataColumnsVerifier) VerifiedRODataColumns() ([]blocks.VerifiedRODataColumn, error) {
	if dv.results.allSatisfied() {
		verifiedRODataColumns := make([]blocks.VerifiedRODataColumn, 0, len(dv.dataColumns))
		for _, dataColumn := range dv.dataColumns {
			verifiedRODataColumn := blocks.NewVerifiedRODataColumn(dataColumn)
			verifiedRODataColumns = append(verifiedRODataColumns, verifiedRODataColumn)
		}

		return verifiedRODataColumns, nil
	}

	return nil, dv.results.errors(ErrColumnInvalid)
}

// SatisfyRequirement allows the caller to assert that a requirement has been satisfied.
// This gives us a way to tick the box for a requirement where the usual method would be impractical.
// For example, when batch syncing, forkchoice is only updated at the end of the batch. So the checks that use
// forkchoice, like descends from finalized or parent seen, would necessarily fail. Allowing the caller to
// assert the requirement has been satisfied ensures we have an easy way to audit which piece of code is satisfying
// a requirement outside of this package.
func (dv *RODataColumnsVerifier) SatisfyRequirement(req Requirement) {
	dv.recordResult(req, nil)
}

func (dv *RODataColumnsVerifier) recordResult(req Requirement, err *error) {
	if err == nil || *err == nil {
		dv.results.record(req, nil)
		return
	}
	dv.results.record(req, *err)
}

// DataColumnsIndexInBounds represents the follow spec verification:
// [REJECT] The sidecar's index is consistent with NUMBER_OF_COLUMNS -- i.e. data_column_sidecar.index < NUMBER_OF_COLUMNS.
func (dv *RODataColumnsVerifier) DataColumnsIndexInBounds() (err error) {
	defer dv.recordResult(RequireDataColumnIndexInBounds, &err)

	for _, dataColumn := range dv.dataColumns {
		if dataColumn.ColumnIndex >= fieldparams.NumberOfColumns {
			fields := logging.DataColumnFields(dataColumn)
			log.WithFields(fields).Debug("Sidecar index >= NUMBER_OF_COLUMNS")
			return columnErrBuilder(ErrColumnIndexInvalid)
		}
	}

	return nil
}

// NotFromFutureSlot represents the spec verification:
// [IGNORE] The sidecar is not from a future slot (with a MAXIMUM_GOSSIP_CLOCK_DISPARITY allowance)
// -- i.e. validate that block_header.slot <= current_slot
func (dv *RODataColumnsVerifier) NotFromFutureSlot() (err error) {
	defer dv.recordResult(RequireNotFromFutureSlot, &err)

	// Retrieve the current slot.
	currentSlot := dv.clock.CurrentSlot()

	// Get the current time.
	now := dv.clock.Now()

	// Retrieve the maximum gossip clock disparity.
	maximumGossipClockDisparity := params.BeaconConfig().MaximumGossipClockDisparityDuration()

	for _, dataColumn := range dv.dataColumns {
		// Extract the data column slot.
		dataColumnSlot := dataColumn.Slot()

		// Skip if the data column slotis the same as the current slot.
		if currentSlot == dataColumnSlot {
			continue
		}

		// earliestStart represents the time the slot starts, lowered by MAXIMUM_GOSSIP_CLOCK_DISPARITY.
		// We lower the time by MAXIMUM_GOSSIP_CLOCK_DISPARITY in case system time is running slightly behind real time.
		earliestStart := dv.clock.SlotStart(dataColumnSlot).Add(-maximumGossipClockDisparity)

		// If the system time is still before earliestStart, we consider the column from a future slot and return an error.
		if now.Before(earliestStart) {
			fields := logging.DataColumnFields(dataColumn)
			log.WithFields(fields).Debug("Sidecar slot is too far in the future")

			return columnErrBuilder(ErrFromFutureSlot)
		}
	}

	return nil
}

// SlotAboveFinalized represents the spec verification:
// [IGNORE] The sidecar is from a slot greater than the latest finalized slot
// -- i.e. validate that block_header.slot > compute_start_slot_at_epoch(state.finalized_checkpoint.epoch)
func (dv *RODataColumnsVerifier) SlotAboveFinalized() (err error) {
	defer dv.recordResult(RequireSlotAboveFinalized, &err)

	// Retrieve the finalized checkpoint.
	finalizedCheckpoint := dv.fc.FinalizedCheckpoint()

	// Compute the first slot of the finalized checkpoint epoch.
	startSlot, err := slots.EpochStart(finalizedCheckpoint.Epoch)
	if err != nil {
		return errors.Wrapf(
			columnErrBuilder(ErrSlotNotAfterFinalized),
			"error computing epoch start slot for finalized checkpoint (%d) %s",
			finalizedCheckpoint.Epoch,
			err.Error(),
		)
	}

	for _, dataColumn := range dv.dataColumns {
		// Extract the data column slot.
		dataColumnSlot := dataColumn.Slot()

		// Check if the data column slot is after first slot of the epoch corresponding to the finalized checkpoint.
		if dataColumnSlot <= startSlot {
			fields := logging.DataColumnFields(dataColumn)
			log.WithFields(fields).Debug("Sidecar slot is not after finalized checkpoint")

			return columnErrBuilder(ErrSlotNotAfterFinalized)
		}
	}

	return nil
}

// ValidProposerSignature represents the spec verification:
// [REJECT] The proposer signature of sidecar.signed_block_header, is valid with respect to the block_header.proposer_index pubkey.
func (dv *RODataColumnsVerifier) ValidProposerSignature(ctx context.Context) (err error) {
	defer dv.recordResult(RequireValidProposerSignature, &err)

	for _, dataColumn := range dv.dataColumns {
		// Extract the signature data from the data column.
		signatureData := columnToSignatureData(dataColumn)

		// Get logging fields.
		fields := logging.DataColumnFields(dataColumn)
		log := log.WithFields(fields)

		// First check if there is a cached verification that can be reused.
		seen, err := dv.sc.SignatureVerified(signatureData)
		if err != nil {
			log.WithError(err).Debug("Reusing failed proposer signature validation from cache")

			blobVerificationProposerSignatureCache.WithLabelValues("hit-invalid").Inc()
			return columnErrBuilder(ErrInvalidProposerSignature)
		}

		// If yes, we can skip the full verification.
		if seen {
			columnVerificationProposerSignatureCache.WithLabelValues("hit-valid").Inc()
			continue
		}

		columnVerificationProposerSignatureCache.WithLabelValues("miss").Inc()

		// Retrieve the root of the parent block corresponding to the data column.
		parentRoot := dataColumn.ParentRoot()

		// Retrieve the parentState state to fallback to full verification.
		parentState, err := dv.sr.StateByRoot(ctx, parentRoot)
		if err != nil {
			log.WithError(err).Debug("Could not replay parent state for column signature verification")
			return columnErrBuilder(ErrInvalidProposerSignature)
		}

		// Full verification, which will subsequently be cached for anything sharing the signature cache.
		if err = dv.sc.VerifySignature(signatureData, parentState); err != nil {
			log.WithError(err).Debug("Signature verification failed")
			return columnErrBuilder(ErrInvalidProposerSignature)
		}
	}

	return nil
}

// SidecarParentSeen represents the spec verification:
// [IGNORE] The sidecar's block's parent (defined by block_header.parent_root) has been seen
// (via both gossip and non-gossip sources) (a client MAY queue sidecars for processing once the parent block is retrieved).
func (dv *RODataColumnsVerifier) SidecarParentSeen(parentSeen func([32]byte) bool) (err error) {
	defer dv.recordResult(RequireSidecarParentSeen, &err)

	for _, dataColumn := range dv.dataColumns {
		// Extract the root of the parent block corresponding to the data column.
		parentRoot := dataColumn.ParentRoot()

		// Skip if the parent root has been seen.
		if parentSeen != nil && parentSeen(parentRoot) {
			continue
		}

		if !dv.fc.HasNode(parentRoot) {
			fields := logging.DataColumnFields(dataColumn)
			log.WithFields(fields).Debug("Parent root has not been seen")
			return columnErrBuilder(ErrSidecarParentNotSeen)
		}
	}

	return nil
}

// SidecarParentValid represents the spec verification:
// [REJECT] The sidecar's block's parent (defined by block_header.parent_root) passes validation.
func (dv *RODataColumnsVerifier) SidecarParentValid(badParent func([32]byte) bool) (err error) {
	defer dv.recordResult(RequireSidecarParentValid, &err)

	for _, dataColumn := range dv.dataColumns {
		// Extract the root of the parent block corresponding to the data column.
		parentRoot := dataColumn.ParentRoot()

		if badParent != nil && badParent(parentRoot) {
			fields := logging.DataColumnFields(dataColumn)
			log.WithFields(fields).Debug("Parent root is invalid")
			return columnErrBuilder(ErrSidecarParentInvalid)
		}
	}

	return nil
}

// SidecarParentSlotLower represents the spec verification:
// [REJECT] The sidecar is from a higher slot than the sidecar's block's parent (defined by block_header.parent_root).
func (dv *RODataColumnsVerifier) SidecarParentSlotLower() (err error) {
	defer dv.recordResult(RequireSidecarParentSlotLower, &err)

	for _, dataColumn := range dv.dataColumns {
		// Extract the root of the parent block corresponding to the data column.
		parentRoot := dataColumn.ParentRoot()

		// Compute the slot of the parent block.
		parentSlot, err := dv.fc.Slot(parentRoot)
		if err != nil {
			return errors.Wrap(columnErrBuilder(ErrSlotNotAfterParent), "parent root not in forkchoice")
		}

		// Extract the slot of the data column.
		dataColumnSlot := dataColumn.Slot()

		// Check if the data column slot is after the parent slot.
		if parentSlot >= dataColumnSlot {
			fields := logging.DataColumnFields(dataColumn)
			log.WithFields(fields).Debug("Sidecar slot is not after parent slot")
			return ErrSlotNotAfterParent
		}
	}

	return nil
}

// SidecarDescendsFromFinalized represents the spec verification:
// [REJECT] The current finalized_checkpoint is an ancestor of the sidecar's block
// -- i.e. get_checkpoint_block(store, block_header.parent_root, store.finalized_checkpoint.epoch) == store.finalized_checkpoint.root.
func (dv *RODataColumnsVerifier) SidecarDescendsFromFinalized() (err error) {
	defer dv.recordResult(RequireSidecarDescendsFromFinalized, &err)

	for _, dataColumn := range dv.dataColumns {
		// Extract the root of the parent block corresponding to the data column.
		parentRoot := dataColumn.ParentRoot()

		if !dv.fc.HasNode(parentRoot) {
			fields := logging.DataColumnFields(dataColumn)
			log.WithFields(fields).Debug("Parent root not in forkchoice")
			return columnErrBuilder(ErrSidecarNotFinalizedDescendent)
		}
	}

	return nil
}

// SidecarInclusionProven represents the spec verification:
// [REJECT] The sidecar's kzg_commitments field inclusion proof is valid as verified by verify_data_column_sidecar_inclusion_proof(sidecar).
func (dv *RODataColumnsVerifier) SidecarInclusionProven() (err error) {
	defer dv.recordResult(RequireSidecarInclusionProven, &err)

	for _, dataColumn := range dv.dataColumns {
		if err = blocks.VerifyKZGInclusionProofColumn(dataColumn); err != nil {
			fields := logging.DataColumnFields(dataColumn)
			log.WithError(err).WithFields(fields).Debug("Sidecar inclusion proof verification failed")
			return columnErrBuilder(ErrSidecarInclusionProofInvalid)
		}
	}

	return nil
}

// SidecarKzgProofVerified represents the spec verification:
// [REJECT] The sidecar's column data is valid as verified by verify_data_column_sidecar_kzg_proofs(sidecar).
func (dv *RODataColumnsVerifier) SidecarKzgProofVerified() (err error) {
	defer dv.recordResult(RequireSidecarKzgProofVerified, &err)

	ok, err := dv.verifyDataColumnsCommitment(dv.dataColumns)
	if err != nil {
		for _, dataColumn := range dv.dataColumns {
			fields := logging.DataColumnFields(dataColumn)
			log.WithError(err).WithFields(fields).Debug("Error verifying KZG commitment proof in the batch containing this sidecar")
		}
		return columnErrBuilder(ErrSidecarKzgProofInvalid)
	}

	if ok {
		return nil
	}

	for _, dataColumn := range dv.dataColumns {
		fields := logging.DataColumnFields(dataColumn)
		log.WithFields(fields).Debug("KZG commitment proof verification failed in the batch containing this sidecar")
	}

	return columnErrBuilder(ErrSidecarKzgProofInvalid)
}

// SidecarProposerExpected represents the spec verification:
// [REJECT] The sidecar is proposed by the expected proposer_index for the block's slot
// in the context of the current shuffling (defined by block_header.parent_root/block_header.slot).
// If the proposer_index cannot immediately be verified against the expected shuffling, the sidecar MAY be queued
// for later processing while proposers for the block's branch are calculated -- in such a case do not REJECT, instead IGNORE this message.
func (dv *RODataColumnsVerifier) SidecarProposerExpected(ctx context.Context) (err error) {
	defer dv.recordResult(RequireSidecarProposerExpected, &err)

	for _, dataColumn := range dv.dataColumns {
		// Extract the slot of the data column.
		dataColumnSlot := dataColumn.Slot()

		// Compute the epoch of the data column slot.
		dataColumnEpoch := slots.ToEpoch(dataColumnSlot)
		if dataColumnEpoch > 0 {
			dataColumnEpoch = dataColumnEpoch - 1
		}

		// Extract the root of the parent block corresponding to the data column.
		parentRoot := dataColumn.ParentRoot()

		// Compute the target root for the epoch.
		targetRoot, err := dv.fc.TargetRootForEpoch(parentRoot, dataColumnEpoch)
		if err != nil {
			return columnErrBuilder(ErrSidecarUnexpectedProposer)
		}

		// Create a checkpoint for the target root.
		checkpoint := &forkchoicetypes.Checkpoint{Root: targetRoot, Epoch: dataColumnEpoch}

		// Try to extract the proposer index from the data column in the cache.
		idx, cached := dv.pc.Proposer(checkpoint, dataColumnSlot)

		if !cached {
			// Retrieve the root of the parent block corresponding to the data column.
			parentRoot := dataColumn.ParentRoot()

			// Retrieve the parentState state to fallback to full verification.
			parentState, err := dv.sr.StateByRoot(ctx, parentRoot)
			if err != nil {
				fields := logging.DataColumnFields(dataColumn)
				log.WithError(err).WithFields(fields).Debug("State replay to parent_root failed")
				return columnErrBuilder(ErrSidecarUnexpectedProposer)
			}

			idx, err = dv.pc.ComputeProposer(ctx, parentRoot, dataColumnSlot, parentState)
			if err != nil {
				fields := logging.DataColumnFields(dataColumn)
				log.WithError(err).WithFields(fields).Debug("Error computing proposer index from parent state")
				return columnErrBuilder(ErrSidecarUnexpectedProposer)
			}
		}

		if idx != dataColumn.ProposerIndex() {
			fields := logging.DataColumnFields(dataColumn)
			log.WithError(columnErrBuilder(ErrSidecarUnexpectedProposer)).
				WithFields(fields).
				WithField("expectedProposer", idx).
				Debug("Unexpected column proposer")

			return columnErrBuilder(ErrSidecarUnexpectedProposer)
		}
	}

	return nil
}

func columnToSignatureData(d blocks.RODataColumn) SignatureData {
	return SignatureData{
		Root:      d.BlockRoot(),
		Parent:    d.ParentRoot(),
		Signature: bytesutil.ToBytes96(d.SignedBlockHeader.Signature),
		Proposer:  d.ProposerIndex(),
		Slot:      d.Slot(),
	}
}

func columnErrBuilder(baseErr error) error {
	return errors.Wrap(baseErr, ErrColumnInvalid.Error())
}
