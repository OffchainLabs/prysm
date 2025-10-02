package verification

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	forkchoicetypes "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/runtime/logging"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

var (
	// GossipDataColumnSidecarRequirements defines the set of requirements that DataColumnSidecars received on gossip
	// must satisfy in order to upgrade an RODataColumn to a VerifiedRODataColumn.
	// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#data_column_sidecar_subnet_id
	GossipDataColumnSidecarRequirements = []Requirement{
		RequireValidFields,
		RequireCorrectSubnet,
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

	// ByRangeRequestDataColumnSidecarRequirements defines the set of requirements that DataColumnSidecars received
	// via the by range request must satisfy in order to upgrade an RODataColumn to a VerifiedRODataColumn.
	// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#datacolumnsidecarsbyrange-v1
	ByRangeRequestDataColumnSidecarRequirements = []Requirement{
		RequireValidFields,
		RequireSidecarInclusionProven,
		RequireSidecarKzgProofVerified,
	}

	// ByRootRequestDataColumnSidecarRequirements defines the set of requirements that DataColumnSidecars received
	// via the by root request must satisfy in order to upgrade an RODataColumn to a VerifiedRODataColumn.
	// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#datacolumnsidecarsbyroot-v1
	ByRootRequestDataColumnSidecarRequirements = []Requirement{
		RequireValidFields,
		RequireSidecarInclusionProven,
		RequireSidecarKzgProofVerified,
	}

	// SpectestDataColumnSidecarRequirements is used by the forkchoice spectests when verifying data columns used in the on_block tests.
	SpectestDataColumnSidecarRequirements = requirementList(GossipDataColumnSidecarRequirements).excluding(
		RequireSidecarParentSeen, RequireSidecarParentValid)

	errColumnsInvalid = errors.New("data columns failed verification")
	errBadTopicLength = errors.New("topic length is invalid")
	errBadTopic       = errors.New("topic is not of the one expected")
)

type (
	RODataColumnsVerifier struct {
		*sharedResources
		results                     *results
		dataColumns                 []blocks.RODataColumn
		verifyDataColumnsCommitment rodataColumnsCommitmentVerifier
		stateByRoot                 map[[fieldparams.RootLength]byte]state.BeaconState
	}

	rodataColumnsCommitmentVerifier func([]blocks.RODataColumn) error
)

var _ DataColumnsVerifier = &RODataColumnsVerifier{}

// VerifiedRODataColumns "upgrades" wrapped RODataColumns to VerifiedRODataColumns.
// If any of the verifications ran against the data columns failed, or some required verifications
// were not run, an error will be returned.
func (dv *RODataColumnsVerifier) VerifiedRODataColumns() ([]blocks.VerifiedRODataColumn, error) {
	if !dv.results.allSatisfied() {
		return nil, dv.results.errors(errColumnsInvalid)
	}

	verifiedRODataColumns := make([]blocks.VerifiedRODataColumn, 0, len(dv.dataColumns))
	for _, dataColumn := range dv.dataColumns {
		verifiedRODataColumn := blocks.NewVerifiedRODataColumn(dataColumn)
		verifiedRODataColumns = append(verifiedRODataColumns, verifiedRODataColumn)
	}

	return verifiedRODataColumns, nil
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

func (dv *RODataColumnsVerifier) ValidFields() (err error) {
	if ok, err := dv.results.cached(RequireValidFields); ok {
		return err
	}

	defer dv.recordResult(RequireValidFields, &err)

	for _, dataColumn := range dv.dataColumns {
		if err := peerdas.VerifyDataColumnSidecar(dataColumn); err != nil {
			return columnErrBuilder(errors.Wrap(err, "verify data column sidecar"))
		}
	}

	return nil
}

func (dv *RODataColumnsVerifier) CorrectSubnet(dataColumnSidecarSubTopic string, expectedTopics []string) (err error) {
	if ok, err := dv.results.cached(RequireCorrectSubnet); ok {
		return err
	}

	defer dv.recordResult(RequireCorrectSubnet, &err)

	if len(expectedTopics) != len(dv.dataColumns) {
		return columnErrBuilder(errBadTopicLength)
	}

	for i := range dv.dataColumns {
		// We add a trailing slash to avoid, for example,
		// an actual topic /eth2/9dc47cc6/data_column_sidecar_1
		// to match with /eth2/9dc47cc6/data_column_sidecar_120
		expectedTopic := expectedTopics[i] + "/"

		actualSubnet := peerdas.ComputeSubnetForDataColumnSidecar(dv.dataColumns[i].Index)
		actualSubTopic := fmt.Sprintf(dataColumnSidecarSubTopic, actualSubnet)

		if !strings.Contains(expectedTopic, actualSubTopic) {
			return columnErrBuilder(errBadTopic)
		}
	}

	return nil
}

func (dv *RODataColumnsVerifier) NotFromFutureSlot() (err error) {
	if ok, err := dv.results.cached(RequireNotFromFutureSlot); ok {
		return err
	}

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

		// Skip if the data column slot is the same as the current slot.
		if currentSlot == dataColumnSlot {
			continue
		}

		// earliestStart represents the time the slot starts, lowered by MAXIMUM_GOSSIP_CLOCK_DISPARITY.
		// We lower the time by MAXIMUM_GOSSIP_CLOCK_DISPARITY in case system time is running slightly behind real time.
		earliestStart, err := dv.clock.SlotStart(dataColumnSlot)
		if err != nil {
			return columnErrBuilder(errors.Wrap(err, "failed to determine slot start time from clock waiter"))
		}
		earliestStart = earliestStart.Add(-maximumGossipClockDisparity)

		// If the system time is still before earliestStart, we consider the column from a future slot and return an error.
		if now.Before(earliestStart) {
			return columnErrBuilder(errFromFutureSlot)
		}
	}

	return nil
}

func (dv *RODataColumnsVerifier) SlotAboveFinalized() (err error) {
	if ok, err := dv.results.cached(RequireSlotAboveFinalized); ok {
		return err
	}

	defer dv.recordResult(RequireSlotAboveFinalized, &err)

	// Retrieve the finalized checkpoint.
	finalizedCheckpoint := dv.fc.FinalizedCheckpoint()

	// Compute the first slot of the finalized checkpoint epoch.
	startSlot, err := slots.EpochStart(finalizedCheckpoint.Epoch)
	if err != nil {
		return columnErrBuilder(errors.Wrap(err, "epoch start"))
	}

	for _, dataColumn := range dv.dataColumns {
		// Check if the data column slot is after first slot of the epoch corresponding to the finalized checkpoint.
		if dataColumn.Slot() <= startSlot {
			return columnErrBuilder(errSlotNotAfterFinalized)
		}
	}

	return nil
}

func (dv *RODataColumnsVerifier) ValidProposerSignature(ctx context.Context) (err error) {
	if ok, err := dv.results.cached(RequireValidProposerSignature); ok {
		return err
	}

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

			columnVerificationProposerSignatureCache.WithLabelValues("hit-invalid").Inc()
			return columnErrBuilder(ErrInvalidProposerSignature)
		}

		// If yes, we can skip the full verification.
		if seen {
			columnVerificationProposerSignatureCache.WithLabelValues("hit-valid").Inc()
			continue
		}

		columnVerificationProposerSignatureCache.WithLabelValues("miss").Inc()

		// Retrieve the parent state.
		parentState, err := dv.parentState(ctx, dataColumn)
		if err != nil {
			return columnErrBuilder(errors.Wrap(err, "parent state"))
		}

		// Full verification, which will subsequently be cached for anything sharing the signature cache.
		if err = dv.sc.VerifySignature(signatureData, parentState); err != nil {
			return columnErrBuilder(errors.Wrap(err, "verify signature"))
		}
	}

	return nil
}

func (dv *RODataColumnsVerifier) SidecarParentSeen(parentSeen func([fieldparams.RootLength]byte) bool) (err error) {
	if ok, err := dv.results.cached(RequireSidecarParentSeen); ok {
		return err
	}

	defer dv.recordResult(RequireSidecarParentSeen, &err)

	for _, dataColumn := range dv.dataColumns {
		// Skip if the parent root has been seen.
		parentRoot := dataColumn.ParentRoot()
		if parentSeen != nil && parentSeen(parentRoot) {
			continue
		}

		if !dv.fc.HasNode(parentRoot) {
			return columnErrBuilder(errSidecarParentNotSeen)
		}
	}

	return nil
}

func (dv *RODataColumnsVerifier) SidecarParentValid(badParent func([fieldparams.RootLength]byte) bool) (err error) {
	if ok, err := dv.results.cached(RequireSidecarParentValid); ok {
		return err
	}

	defer dv.recordResult(RequireSidecarParentValid, &err)

	for _, dataColumn := range dv.dataColumns {
		if badParent != nil && badParent(dataColumn.ParentRoot()) {
			return columnErrBuilder(errSidecarParentInvalid)
		}
	}

	return nil
}

func (dv *RODataColumnsVerifier) SidecarParentSlotLower() (err error) {
	if ok, err := dv.results.cached(RequireSidecarParentSlotLower); ok {
		return err
	}

	defer dv.recordResult(RequireSidecarParentSlotLower, &err)

	for _, dataColumn := range dv.dataColumns {
		// Compute the slot of the parent block.
		parentSlot, err := dv.fc.Slot(dataColumn.ParentRoot())
		if err != nil {
			return columnErrBuilder(errors.Wrap(err, "slot"))
		}

		// Check if the data column slot is after the parent slot.
		if parentSlot >= dataColumn.Slot() {
			return columnErrBuilder(errSlotNotAfterParent)
		}
	}

	return nil
}

func (dv *RODataColumnsVerifier) SidecarDescendsFromFinalized() (err error) {
	if ok, err := dv.results.cached(RequireSidecarDescendsFromFinalized); ok {
		return err
	}

	defer dv.recordResult(RequireSidecarDescendsFromFinalized, &err)

	for _, dataColumn := range dv.dataColumns {
		// Extract the root of the parent block corresponding to the data column.
		parentRoot := dataColumn.ParentRoot()

		if !dv.fc.HasNode(parentRoot) {
			return columnErrBuilder(errSidecarNotFinalizedDescendent)
		}
	}

	return nil
}

func (dv *RODataColumnsVerifier) SidecarInclusionProven() (err error) {
	if ok, err := dv.results.cached(RequireSidecarInclusionProven); ok {
		return err
	}

	defer dv.recordResult(RequireSidecarInclusionProven, &err)

	startTime := time.Now()

	for _, dataColumn := range dv.dataColumns {
		k, keyErr := inclusionProofKey(dataColumn)
		if keyErr == nil {
			if _, ok := dv.ic.Get(k); ok {
				continue
			}
		} else {
			log.WithError(keyErr).Error("Failed to get inclusion proof key")
		}

		if err = peerdas.VerifyDataColumnSidecarInclusionProof(dataColumn); err != nil {
			return columnErrBuilder(ErrSidecarInclusionProofInvalid)
		}

		if keyErr == nil {
			dv.ic.Add(k, struct{}{})
		}
	}

	dataColumnSidecarInclusionProofVerificationHistogram.Observe(float64(time.Since(startTime).Milliseconds()))

	return nil
}

func (dv *RODataColumnsVerifier) SidecarKzgProofVerified() (err error) {
	if ok, err := dv.results.cached(RequireSidecarKzgProofVerified); ok {
		return err
	}

	defer dv.recordResult(RequireSidecarKzgProofVerified, &err)

	startTime := time.Now()

	err = dv.verifyDataColumnsCommitment(dv.dataColumns)
	if err != nil {
		return columnErrBuilder(errors.Wrap(err, "verify data column commitment"))
	}

	dataColumnBatchKZGVerificationHistogram.Observe(float64(time.Since(startTime).Milliseconds()))
	return nil
}

func (dv *RODataColumnsVerifier) SidecarProposerExpected(ctx context.Context) (err error) {
	if ok, err := dv.results.cached(RequireSidecarProposerExpected); ok {
		return err
	}

	defer dv.recordResult(RequireSidecarProposerExpected, &err)

	type slotParentRoot struct {
		slot       primitives.Slot
		parentRoot [fieldparams.RootLength]byte
	}

	targetRootBySlotParentRoot := make(map[slotParentRoot][fieldparams.RootLength]byte)

	var targetRootFromCache = func(slot primitives.Slot, parentRoot [fieldparams.RootLength]byte) ([fieldparams.RootLength]byte, error) {
		// Use cached values if available.
		slotParentRoot := slotParentRoot{slot: slot, parentRoot: parentRoot}
		if root, ok := targetRootBySlotParentRoot[slotParentRoot]; ok {
			return root, nil
		}

		// Compute the epoch of the data column slot.
		dataColumnEpoch := slots.ToEpoch(slot)
		if dataColumnEpoch > 0 {
			dataColumnEpoch = dataColumnEpoch - 1
		}

		// Compute the target root for the epoch.
		targetRoot, err := dv.fc.TargetRootForEpoch(parentRoot, dataColumnEpoch)
		if err != nil {
			return [fieldparams.RootLength]byte{}, columnErrBuilder(errors.Wrap(err, "target root from epoch"))
		}

		// Store the target root in the cache.
		targetRootBySlotParentRoot[slotParentRoot] = targetRoot

		return targetRoot, nil
	}

	for _, dataColumn := range dv.dataColumns {
		// Extract the slot of the data column.
		dataColumnSlot := dataColumn.Slot()

		// Extract the root of the parent block corresponding to the data column.
		parentRoot := dataColumn.ParentRoot()

		// Compute the target root for the data column.
		targetRoot, err := targetRootFromCache(dataColumnSlot, parentRoot)
		if err != nil {
			return columnErrBuilder(errors.Wrap(err, "target root"))
		}

		// Compute the epoch of the data column slot.
		dataColumnEpoch := slots.ToEpoch(dataColumnSlot)
		if dataColumnEpoch > 0 {
			dataColumnEpoch = dataColumnEpoch - 1
		}

		// Create a checkpoint for the target root.
		checkpoint := &forkchoicetypes.Checkpoint{Root: targetRoot, Epoch: dataColumnEpoch}

		// Try to extract the proposer index from the data column in the cache.
		idx, cached := dv.pc.Proposer(checkpoint, dataColumnSlot)

		if !cached {
			// Retrieve the parent state.
			parentState, err := dv.parentState(ctx, dataColumn)
			if err != nil {
				return columnErrBuilder(errors.Wrap(err, "parent state"))
			}

			idx, err = dv.pc.ComputeProposer(ctx, parentRoot, dataColumnSlot, parentState)
			if err != nil {
				return columnErrBuilder(errors.Wrap(err, "compute proposer"))
			}
		}

		if idx != dataColumn.ProposerIndex() {
			return columnErrBuilder(errSidecarUnexpectedProposer)
		}
	}

	return nil
}

// parentState retrieves the parent state of the data column from the cache if possible, else retrieves it from the state by rooter.
func (dv *RODataColumnsVerifier) parentState(ctx context.Context, dataColumn blocks.RODataColumn) (state.BeaconState, error) {
	parentRoot := dataColumn.ParentRoot()

	// If the parent root is already in the cache, return it.
	if st, ok := dv.stateByRoot[parentRoot]; ok {
		return st, nil
	}

	// Retrieve the parent state from the state by rooter.
	st, err := dv.sr.StateByRoot(ctx, parentRoot)
	if err != nil {
		return nil, errors.Wrap(err, "state by root")
	}

	// Store the parent state in the cache.
	dv.stateByRoot[parentRoot] = st

	return st, nil
}

func columnToSignatureData(d blocks.RODataColumn) signatureData {
	return signatureData{
		Root:      d.BlockRoot(),
		Parent:    d.ParentRoot(),
		Signature: bytesutil.ToBytes96(d.SignedBlockHeader.Signature),
		Proposer:  d.ProposerIndex(),
		Slot:      d.Slot(),
	}
}

func columnErrBuilder(baseErr error) error {
	return errors.Wrap(baseErr, errColumnsInvalid.Error())
}

// incluseionProofKey computes a unique key based on the KZG commitments,
// the KZG commitments inclusion proof, and the signed block header root.
func inclusionProofKey(c blocks.RODataColumn) ([32]byte, error) {
	const (
		commsIncProofLen       = 4
		commsIncProofByteCount = commsIncProofLen * 32
	)

	if len(c.KzgCommitmentsInclusionProof) != commsIncProofLen {
		// This should be already enforced by ssz unmarshaling; still check so we don't panic on array bounds.
		return [32]byte{}, columnErrBuilder(ErrSidecarInclusionProofInvalid)
	}

	commsByteCount := len(c.KzgCommitments) * fieldparams.KzgCommitmentSize
	unhashedKey := make([]byte, 0, commsIncProofByteCount+fieldparams.RootLength+commsByteCount)

	// Include the commitments inclusion proof in the key.
	for _, proof := range c.KzgCommitmentsInclusionProof {
		unhashedKey = append(unhashedKey, proof...)
	}

	// Include the block root in the key.
	root, err := c.SignedBlockHeader.HashTreeRoot()
	if err != nil {
		return [32]byte{}, columnErrBuilder(errors.Wrap(err, "hash tree root"))
	}

	unhashedKey = append(unhashedKey, root[:]...)

	// Include the commitments in the key.
	for _, commitment := range c.KzgCommitments {
		unhashedKey = append(unhashedKey, commitment...)
	}

	return sha256.Sum256(unhashedKey), nil
}
