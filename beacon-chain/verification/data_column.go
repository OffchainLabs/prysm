package verification

import (
	"context"

	"github.com/pkg/errors"
	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
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

type RODataColumnVerifier struct {
	*sharedResources
	results                    *results
	dataColumn                 blocks.RODataColumn
	parent                     state.BeaconState
	verifyDataColumnCommitment rodataColumnCommitmentVerifier
}

type rodataColumnCommitmentVerifier func(blocks.RODataColumn) (bool, error)

var _ DataColumnVerifier = &RODataColumnVerifier{}

// VerifiedRODataColumn "upgrades" the wrapped ROBlob to a VerifiedROBlob.
// If any of the verifications ran against the blob failed, or some required verifications
// were not run, an error will be returned.
func (dv *RODataColumnVerifier) VerifiedRODataColumn() (blocks.VerifiedRODataColumn, error) {
	if dv.results.allSatisfied() {
		return blocks.NewVerifiedRODataColumn(dv.dataColumn), nil
	}
	return blocks.VerifiedRODataColumn{}, dv.results.errors(ErrColumnInvalid)
}

// SatisfyRequirement allows the caller to assert that a requirement has been satisfied.
// This gives us a way to tick the box for a requirement where the usual method would be impractical.
// For example, when batch syncing, forkchoice is only updated at the end of the batch. So the checks that use
// forkchoice, like descends from finalized or parent seen, would necessarily fail. Allowing the caller to
// assert the requirement has been satisfied ensures we have an easy way to audit which piece of code is satisfying
// a requirement outside of this package.
func (dv *RODataColumnVerifier) SatisfyRequirement(req Requirement) {
	dv.recordResult(req, nil)
}

func (dv *RODataColumnVerifier) recordResult(req Requirement, err *error) {
	if err == nil || *err == nil {
		dv.results.record(req, nil)
		return
	}
	dv.results.record(req, *err)
}

// DataColumnIndexInBounds represents the follow spec verification:
// [REJECT] The sidecar's index is consistent with NUMBER_OF_COLUMNS -- i.e. data_column_sidecar.index < NUMBER_OF_COLUMNS.
func (dv *RODataColumnVerifier) DataColumnIndexInBounds() (err error) {
	defer dv.recordResult(RequireDataColumnIndexInBounds, &err)
	if dv.dataColumn.ColumnIndex >= fieldparams.NumberOfColumns {
		log.WithFields(logging.DataColumnFields(dv.dataColumn)).Debug("Sidecar index >= NUMBER_OF_COLUMNS")
		return columnErrBuilder(ErrColumnIndexInvalid)
	}
	return nil
}

// NotFromFutureSlot represents the spec verification:
// [IGNORE] The sidecar is not from a future slot (with a MAXIMUM_GOSSIP_CLOCK_DISPARITY allowance)
// -- i.e. validate that block_header.slot <= current_slot
func (dv *RODataColumnVerifier) NotFromFutureSlot() (err error) {
	defer dv.recordResult(RequireNotFromFutureSlot, &err)
	if dv.clock.CurrentSlot() == dv.dataColumn.Slot() {
		return nil
	}
	// earliestStart represents the time the slot starts, lowered by MAXIMUM_GOSSIP_CLOCK_DISPARITY.
	// We lower the time by MAXIMUM_GOSSIP_CLOCK_DISPARITY in case system time is running slightly behind real time.
	earliestStart := dv.clock.SlotStart(dv.dataColumn.Slot()).Add(-1 * params.BeaconConfig().MaximumGossipClockDisparityDuration())
	// If the system time is still before earliestStart, we consider the column from a future slot and return an error.
	if dv.clock.Now().Before(earliestStart) {
		log.WithFields(logging.DataColumnFields(dv.dataColumn)).Debug("Sidecar slot is too far in the future")
		return columnErrBuilder(ErrFromFutureSlot)
	}
	return nil
}

// SlotAboveFinalized represents the spec verification:
// [IGNORE] The sidecar is from a slot greater than the latest finalized slot
// -- i.e. validate that block_header.slot > compute_start_slot_at_epoch(state.finalized_checkpoint.epoch)
func (dv *RODataColumnVerifier) SlotAboveFinalized() (err error) {
	defer dv.recordResult(RequireSlotAboveFinalized, &err)
	fcp := dv.fc.FinalizedCheckpoint()
	fSlot, err := slots.EpochStart(fcp.Epoch)
	if err != nil {
		return errors.Wrapf(columnErrBuilder(ErrSlotNotAfterFinalized), "error computing epoch start slot for finalized checkpoint (%d) %s", fcp.Epoch, err.Error())
	}
	if dv.dataColumn.Slot() <= fSlot {
		log.WithFields(logging.DataColumnFields(dv.dataColumn)).Debug("Sidecar slot is not after finalized checkpoint")
		return columnErrBuilder(ErrSlotNotAfterFinalized)
	}
	return nil
}

// ValidProposerSignature represents the spec verification:
// [REJECT] The proposer signature of sidecar.signed_block_header, is valid with respect to the block_header.proposer_index pubkey.
func (dv *RODataColumnVerifier) ValidProposerSignature(ctx context.Context) (err error) {
	defer dv.recordResult(RequireValidProposerSignature, &err)
	sd := columnToSignatureData(dv.dataColumn)
	// First check if there is a cached verification that can be reused.
	seen, err := dv.sc.SignatureVerified(sd)
	if seen {
		columnVerificationProposerSignatureCache.WithLabelValues("hit-valid").Inc()
		if err != nil {
			log.WithFields(logging.DataColumnFields(dv.dataColumn)).WithError(err).Debug("Reusing failed proposer signature validation from cache")
			blobVerificationProposerSignatureCache.WithLabelValues("hit-invalid").Inc()
			return columnErrBuilder(ErrInvalidProposerSignature)
		}
		return nil
	}
	columnVerificationProposerSignatureCache.WithLabelValues("miss").Inc()

	// Retrieve the parent state to fallback to full verification.
	parent, err := dv.parentState(ctx)
	if err != nil {
		log.WithFields(logging.DataColumnFields(dv.dataColumn)).WithError(err).Debug("Could not replay parent state for column signature verification")
		return columnErrBuilder(ErrInvalidProposerSignature)
	}
	// Full verification, which will subsequently be cached for anything sharing the signature cache.
	if err = dv.sc.VerifySignature(sd, parent); err != nil {
		log.WithFields(logging.DataColumnFields(dv.dataColumn)).WithError(err).Debug("Signature verification failed")
		return columnErrBuilder(ErrInvalidProposerSignature)
	}
	return nil
}

// SidecarParentSeen represents the spec verification:
// [IGNORE] The sidecar's block's parent (defined by block_header.parent_root) has been seen
// (via both gossip and non-gossip sources) (a client MAY queue sidecars for processing once the parent block is retrieved).
func (dv *RODataColumnVerifier) SidecarParentSeen(parentSeen func([32]byte) bool) (err error) {
	defer dv.recordResult(RequireSidecarParentSeen, &err)
	if parentSeen != nil && parentSeen(dv.dataColumn.ParentRoot()) {
		return nil
	}
	if dv.fc.HasNode(dv.dataColumn.ParentRoot()) {
		return nil
	}
	log.WithFields(logging.DataColumnFields(dv.dataColumn)).Debug("Parent root has not been seen")
	return columnErrBuilder(ErrSidecarParentNotSeen)
}

// SidecarParentValid represents the spec verification:
// [REJECT] The sidecar's block's parent (defined by block_header.parent_root) passes validation.
func (dv *RODataColumnVerifier) SidecarParentValid(badParent func([32]byte) bool) (err error) {
	defer dv.recordResult(RequireSidecarParentValid, &err)
	if badParent != nil && badParent(dv.dataColumn.ParentRoot()) {
		log.WithFields(logging.DataColumnFields(dv.dataColumn)).Debug("Parent root is invalid")
		return columnErrBuilder(ErrSidecarParentInvalid)
	}
	return nil
}

// SidecarParentSlotLower represents the spec verification:
// [REJECT] The sidecar is from a higher slot than the sidecar's block's parent (defined by block_header.parent_root).
func (dv *RODataColumnVerifier) SidecarParentSlotLower() (err error) {
	defer dv.recordResult(RequireSidecarParentSlotLower, &err)
	parentSlot, err := dv.fc.Slot(dv.dataColumn.ParentRoot())
	if err != nil {
		return errors.Wrap(columnErrBuilder(ErrSlotNotAfterParent), "parent root not in forkchoice")
	}
	if parentSlot >= dv.dataColumn.Slot() {
		return ErrSlotNotAfterParent
	}
	return nil
}

// SidecarDescendsFromFinalized represents the spec verification:
// [REJECT] The current finalized_checkpoint is an ancestor of the sidecar's block
// -- i.e. get_checkpoint_block(store, block_header.parent_root, store.finalized_checkpoint.epoch) == store.finalized_checkpoint.root.
func (dv *RODataColumnVerifier) SidecarDescendsFromFinalized() (err error) {
	defer dv.recordResult(RequireSidecarDescendsFromFinalized, &err)
	if !dv.fc.HasNode(dv.dataColumn.ParentRoot()) {
		log.WithFields(logging.DataColumnFields(dv.dataColumn)).Debug("Parent root not in forkchoice")
		return columnErrBuilder(ErrSidecarNotFinalizedDescendent)
	}
	return nil
}

// SidecarInclusionProven represents the spec verification:
// [REJECT] The sidecar's kzg_commitments field inclusion proof is valid as verified by verify_data_column_sidecar_inclusion_proof(sidecar).
func (dv *RODataColumnVerifier) SidecarInclusionProven() (err error) {
	defer dv.recordResult(RequireSidecarInclusionProven, &err)
	if err = blocks.VerifyKZGInclusionProofColumn(dv.dataColumn); err != nil {
		log.WithError(err).WithFields(logging.DataColumnFields(dv.dataColumn)).Debug("Sidecar inclusion proof verification failed")
		return columnErrBuilder(ErrSidecarInclusionProofInvalid)
	}
	return nil
}

// SidecarKzgProofVerified represents the spec verification:
// [REJECT] The sidecar's column data is valid as verified by verify_data_column_sidecar_kzg_proofs(sidecar).
func (dv *RODataColumnVerifier) SidecarKzgProofVerified() (err error) {
	defer dv.recordResult(RequireSidecarKzgProofVerified, &err)
	ok, err := dv.verifyDataColumnCommitment(dv.dataColumn)
	if err != nil {
		log.WithError(err).WithFields(logging.DataColumnFields(dv.dataColumn)).Debug("KZG commitment proof verification failed")
		return columnErrBuilder(ErrSidecarKzgProofInvalid)
	}
	if !ok {
		log.WithFields(logging.DataColumnFields(dv.dataColumn)).Debug("KZG commitment proof verification failed")
		return columnErrBuilder(ErrSidecarKzgProofInvalid)
	}
	return nil
}

// SidecarProposerExpected represents the spec verification:
// [REJECT] The sidecar is proposed by the expected proposer_index for the block's slot
// in the context of the current shuffling (defined by block_header.parent_root/block_header.slot).
// If the proposer_index cannot immediately be verified against the expected shuffling, the sidecar MAY be queued
// for later processing while proposers for the block's branch are calculated -- in such a case do not REJECT, instead IGNORE this message.
func (dv *RODataColumnVerifier) SidecarProposerExpected(ctx context.Context) (err error) {
	defer dv.recordResult(RequireSidecarProposerExpected, &err)
	e := slots.ToEpoch(dv.dataColumn.Slot())
	if e > 0 {
		e = e - 1
	}
	r, err := dv.fc.TargetRootForEpoch(dv.dataColumn.ParentRoot(), e)
	if err != nil {
		return columnErrBuilder(ErrSidecarUnexpectedProposer)
	}
	c := &forkchoicetypes.Checkpoint{Root: r, Epoch: e}
	idx, cached := dv.pc.Proposer(c, dv.dataColumn.Slot())
	if !cached {
		pst, err := dv.parentState(ctx)
		if err != nil {
			log.WithError(err).WithFields(logging.DataColumnFields(dv.dataColumn)).Debug("State replay to parent_root failed")
			return columnErrBuilder(ErrSidecarUnexpectedProposer)
		}
		idx, err = dv.pc.ComputeProposer(ctx, dv.dataColumn.ParentRoot(), dv.dataColumn.Slot(), pst)
		if err != nil {
			log.WithError(err).WithFields(logging.DataColumnFields(dv.dataColumn)).Debug("Error computing proposer index from parent state")
			return columnErrBuilder(ErrSidecarUnexpectedProposer)
		}
	}
	if idx != dv.dataColumn.ProposerIndex() {
		log.WithError(columnErrBuilder(ErrSidecarUnexpectedProposer)).
			WithFields(logging.DataColumnFields(dv.dataColumn)).WithField("expectedProposer", idx).
			Debug("unexpected column proposer")
		return columnErrBuilder(ErrSidecarUnexpectedProposer)
	}
	return nil
}

func (dv *RODataColumnVerifier) parentState(ctx context.Context) (state.BeaconState, error) {
	if dv.parent != nil {
		return dv.parent, nil
	}
	st, err := dv.sr.StateByRoot(ctx, dv.dataColumn.ParentRoot())
	if err != nil {
		return nil, err
	}
	dv.parent = st
	return dv.parent, nil
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
