package slasher

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/pkg/errors"
	slashertypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/slasher/types"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// Takes in a list of indexed attestation wrappers and returns any
// found attester slashings to the caller.
func (s *Service) checkSlashableAttestations(
	ctx context.Context, currentEpoch primitives.Epoch, atts []*slashertypes.IndexedAttestationWrapper,
) (map[[fieldparams.RootLength]byte]ethpb.AttSlashing, error) {
	slashings := map[[fieldparams.RootLength]byte]ethpb.AttSlashing{}

	// Double votes
	doubleVoteSlashings, err := s.checkDoubleVotes(ctx, atts)
	if err != nil {
		return nil, errors.Wrap(err, "could not check slashable double votes")
	}

	for root, slashing := range doubleVoteSlashings {
		slashings[root] = slashing
	}

	// Save the attestation records to our database.
	// If multiple attestations are provided for the same validator index + target epoch combination,
	// then the first (validator index + target epoch) => signing root) link is kept into the database.
	if err := s.serviceCfg.Database.SaveAttestationRecordsForValidators(ctx, atts); err != nil {
		return nil, errors.Wrap(err, couldNotSaveAttRecord)
	}

	// Surrounding / surrounded votes
	surroundSlashings, err := s.checkSurroundVotes(ctx, atts, currentEpoch)
	if err != nil {
		return nil, errors.Wrap(err, "could not check slashable surround votes")
	}

	for root, slashing := range surroundSlashings {
		slashings[root] = slashing
	}

	return slashings, nil
}

// Check for surrounding and surrounded votes in our database given a list of incoming attestations.
func (s *Service) checkSurroundVotes(
	ctx context.Context,
	attWrappers []*slashertypes.IndexedAttestationWrapper,
	currentEpoch primitives.Epoch,
) (map[[fieldparams.RootLength]byte]ethpb.AttSlashing, error) {
	// With 256 validators and 16 epochs per chunk, there is 4096 `uint16` elements per chunk.
	// 4096 `uint16` elements = 8192 bytes = 8KB
	// 25_600 chunks * 8KB = 200MB
	const maxChunkBeforeFlush = 25_600

	slashings := map[[fieldparams.RootLength]byte]ethpb.AttSlashing{}

	// Group attestation wrappers by validator chunk index.
	attWrappersByValidatorChunkIndex := s.groupByValidatorChunkIndex(attWrappers)
	attWrappersByValidatorChunkIndexCount := len(attWrappersByValidatorChunkIndex)

	minChunkByChunkIndexByValidatorChunkIndex := make(map[uint64]map[uint64]Chunker, attWrappersByValidatorChunkIndexCount)
	maxChunkByChunkIndexByValidatorChunkIndex := make(map[uint64]map[uint64]Chunker, attWrappersByValidatorChunkIndexCount)

	chunksCounts := 0

	for validatorChunkIndex, attWrappers := range attWrappersByValidatorChunkIndex {
		minChunkByChunkIndex, err := s.updatedChunkByChunkIndex(ctx, slashertypes.MinSpan, currentEpoch, validatorChunkIndex)
		if err != nil {
			return nil, errors.Wrap(err, "could not update updatedMinChunks")
		}

		maxChunkByChunkIndex, err := s.updatedChunkByChunkIndex(ctx, slashertypes.MaxSpan, currentEpoch, validatorChunkIndex)
		if err != nil {
			return nil, errors.Wrap(err, "could not update updatedMaxChunks")
		}

		chunksCounts += len(minChunkByChunkIndex) + len(maxChunkByChunkIndex)

		// Group (already grouped by validator chunk index) attestation wrappers by chunk index.
		attWrappersByChunkIndex := s.groupByChunkIndex(attWrappers)

		// Check for surrounding votes.
		surroundingSlashings, err := s.updateSpans(ctx, minChunkByChunkIndex, attWrappersByChunkIndex, slashertypes.MinSpan, validatorChunkIndex, currentEpoch)
		if err != nil {
			return nil, errors.Wrapf(err, "could not update min attestation spans for validator chunk index %d", validatorChunkIndex)
		}

		for root, slashing := range surroundingSlashings {
			slashings[root] = slashing
		}

		// Check for surrounded votes.
		surroundedSlashings, err := s.updateSpans(ctx, maxChunkByChunkIndex, attWrappersByChunkIndex, slashertypes.MaxSpan, validatorChunkIndex, currentEpoch)
		if err != nil {
			return nil, errors.Wrapf(err, "could not update max attestation spans for validator chunk index %d", validatorChunkIndex)
		}

		for root, slashing := range surroundedSlashings {
			slashings[root] = slashing
		}

		// Memoize the updated chunks for the current validator chunk index.
		minChunkByChunkIndexByValidatorChunkIndex[validatorChunkIndex] = minChunkByChunkIndex
		maxChunkByChunkIndexByValidatorChunkIndex[validatorChunkIndex] = maxChunkByChunkIndex

		if chunksCounts >= maxChunkBeforeFlush {
			// Save the updated chunks to disk if we have reached the maximum number of chunks to store in memory.
			if err := s.saveChunksToDisk(ctx, slashertypes.MinSpan, minChunkByChunkIndexByValidatorChunkIndex); err != nil {
				return nil, errors.Wrap(err, "could not save updated min chunks to disk")
			}

			if err := s.saveChunksToDisk(ctx, slashertypes.MaxSpan, maxChunkByChunkIndexByValidatorChunkIndex); err != nil {
				return nil, errors.Wrap(err, "could not save updated max chunks to disk")
			}

			// Reset the chunks counts.
			chunksCounts = 0

			// Reset memoized chunks.
			minChunkByChunkIndexByValidatorChunkIndex = make(map[uint64]map[uint64]Chunker, attWrappersByValidatorChunkIndexCount)
			maxChunkByChunkIndexByValidatorChunkIndex = make(map[uint64]map[uint64]Chunker, attWrappersByValidatorChunkIndexCount)
		}

		// Update the latest updated epoch for all validators involved to the current chunk.
		indexes := s.params.ValidatorIndexesInChunk(validatorChunkIndex)
		for _, index := range indexes {
			s.latestEpochUpdatedForValidator[index] = currentEpoch
		}
	}

	// Save the updated chunks to disk.
	if err := s.saveChunksToDisk(ctx, slashertypes.MinSpan, minChunkByChunkIndexByValidatorChunkIndex); err != nil {
		return nil, errors.Wrap(err, "could not save updated min chunks to disk")
	}

	if err := s.saveChunksToDisk(ctx, slashertypes.MaxSpan, maxChunkByChunkIndexByValidatorChunkIndex); err != nil {
		return nil, errors.Wrap(err, "could not save updated max chunks to disk")
	}

	return slashings, nil
}

// Check for double votes in our database given a list of incoming attestations.
func (s *Service) checkDoubleVotes(
	ctx context.Context, incomingAttWrappers []*slashertypes.IndexedAttestationWrapper,
) (map[[fieldparams.RootLength]byte]ethpb.AttSlashing, error) {
	ctx, span := trace.StartSpan(ctx, "Slasher.checkDoubleVotesOnDisk")
	defer span.End()

	type attestationInfo struct {
		validatorIndex uint64
		epoch          primitives.Epoch
	}

	slashings := map[[fieldparams.RootLength]byte]ethpb.AttSlashing{}

	// Check each incoming attestation for double votes against other incoming attestations.
	existingAttWrappers := make(map[attestationInfo]*slashertypes.IndexedAttestationWrapper)

	for _, incomingAttWrapper := range incomingAttWrappers {
		targetEpoch := incomingAttWrapper.IndexedAttestation.GetData().Target.Epoch

		for _, validatorIndex := range incomingAttWrapper.IndexedAttestation.GetAttestingIndices() {
			info := attestationInfo{
				validatorIndex: validatorIndex,
				epoch:          targetEpoch,
			}

			existingAttWrapper, ok := existingAttWrappers[info]
			if !ok {
				// This is the first attestation for this `validator index x epoch` combination.
				// There is no double vote. This attestation is memoized for future checks.
				existingAttWrappers[info] = incomingAttWrapper
				continue
			}

			if existingAttWrapper.DataRoot == incomingAttWrapper.DataRoot {
				// Both attestations are the same, this is not a double vote.
				continue
			}

			// There is two different attestations for the same `validator index x epoch` combination.
			// This is a double vote.
			doubleVotesTotal.Inc()

			var slashing ethpb.AttSlashing

			// Both attestations should have the same type. If not, we convert both to Electra attestations.
			unifyAttWrapperVersion(existingAttWrapper, incomingAttWrapper)

			postElectra := existingAttWrapper.IndexedAttestation.Version() >= version.Electra
			if postElectra {
				existing, ok := existingAttWrapper.IndexedAttestation.(*ethpb.IndexedAttestationElectra)
				if !ok {
					return nil, fmt.Errorf(
						"existing attestation has wrong type (expected %T, got %T)",
						&ethpb.IndexedAttestationElectra{},
						existingAttWrapper.IndexedAttestation,
					)
				}
				incoming, ok := incomingAttWrapper.IndexedAttestation.(*ethpb.IndexedAttestationElectra)
				if !ok {
					return nil, fmt.Errorf(
						"incoming attestation has wrong type (expected %T, got %T)",
						&ethpb.IndexedAttestationElectra{},
						incomingAttWrapper.IndexedAttestation,
					)
				}
				slashing = &ethpb.AttesterSlashingElectra{
					Attestation_1: existing,
					Attestation_2: incoming,
				}

				// Ensure the attestation with the lower data root is the first attestation.
				if bytes.Compare(existingAttWrapper.DataRoot[:], incomingAttWrapper.DataRoot[:]) > 0 {
					slashing = &ethpb.AttesterSlashingElectra{
						Attestation_1: incoming,
						Attestation_2: existing,
					}
				}
			} else {
				existing, ok := existingAttWrapper.IndexedAttestation.(*ethpb.IndexedAttestation)
				if !ok {
					return nil, fmt.Errorf(
						"existing attestation has wrong type (expected %T, got %T)",
						&ethpb.IndexedAttestation{},
						existingAttWrapper.IndexedAttestation,
					)
				}
				incoming, ok := incomingAttWrapper.IndexedAttestation.(*ethpb.IndexedAttestation)
				if !ok {
					return nil, fmt.Errorf(
						"incoming attestation has wrong type (expected %T, got %T)",
						&ethpb.IndexedAttestation{},
						incomingAttWrapper.IndexedAttestation,
					)
				}
				slashing = &ethpb.AttesterSlashing{
					Attestation_1: existing,
					Attestation_2: incoming,
				}

				// Ensure the attestation with the lower data root is the first attestation.
				if bytes.Compare(existingAttWrapper.DataRoot[:], incomingAttWrapper.DataRoot[:]) > 0 {
					slashing = &ethpb.AttesterSlashing{
						Attestation_1: incoming,
						Attestation_2: existing,
					}
				}
			}

			root, err := slashing.HashTreeRoot()
			if err != nil {
				return nil, errors.Wrap(err, "could not hash tree root for attester slashing")
			}

			slashings[root] = slashing
		}
	}

	// Check each incoming attestation for double votes against the database.
	doubleVotes, err := s.serviceCfg.Database.CheckAttesterDoubleVotes(ctx, incomingAttWrappers)

	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve potential double votes from disk")
	}

	for _, doubleVote := range doubleVotes {
		doubleVotesTotal.Inc()

		wrapper_1 := doubleVote.Wrapper_1
		wrapper_2 := doubleVote.Wrapper_2

		var slashing ethpb.AttSlashing

		// Both attestations should have the same type. If not, we convert both to Electra attestations.
		unifyAttWrapperVersion(wrapper_1, wrapper_2)

		postElectra := wrapper_1.IndexedAttestation.Version() >= version.Electra
		if postElectra {
			att_1, ok := wrapper_1.IndexedAttestation.(*ethpb.IndexedAttestationElectra)
			if !ok {
				return nil, fmt.Errorf(
					"first attestation has wrong type (expected %T, got %T)",
					&ethpb.IndexedAttestationElectra{},
					wrapper_1.IndexedAttestation,
				)
			}
			att_2, ok := wrapper_2.IndexedAttestation.(*ethpb.IndexedAttestationElectra)
			if !ok {
				return nil, fmt.Errorf(
					"second attestation has wrong type (expected %T, got %T)",
					&ethpb.IndexedAttestationElectra{},
					wrapper_2.IndexedAttestation,
				)
			}
			slashing = &ethpb.AttesterSlashingElectra{
				Attestation_1: att_1,
				Attestation_2: att_2,
			}

			// Ensure the attestation with the lower data root is the first attestation.
			if bytes.Compare(wrapper_1.DataRoot[:], wrapper_2.DataRoot[:]) > 0 {
				slashing = &ethpb.AttesterSlashingElectra{
					Attestation_1: att_2,
					Attestation_2: att_1,
				}
			}
		} else {
			att_1, ok := wrapper_1.IndexedAttestation.(*ethpb.IndexedAttestation)
			if !ok {
				return nil, fmt.Errorf(
					"first attestation has wrong type (expected %T, got %T)",
					&ethpb.IndexedAttestation{},
					wrapper_1.IndexedAttestation,
				)
			}
			att_2, ok := wrapper_2.IndexedAttestation.(*ethpb.IndexedAttestation)
			if !ok {
				return nil, fmt.Errorf(
					"second attestation has wrong type (expected %T, got %T)",
					&ethpb.IndexedAttestation{},
					wrapper_2.IndexedAttestation,
				)
			}
			slashing = &ethpb.AttesterSlashing{
				Attestation_1: att_1,
				Attestation_2: att_2,
			}

			// Ensure the attestation with the lower data root is the first attestation.
			if bytes.Compare(wrapper_1.DataRoot[:], wrapper_2.DataRoot[:]) > 0 {
				slashing = &ethpb.AttesterSlashing{
					Attestation_1: att_2,
					Attestation_2: att_1,
				}
			}
		}

		root, err := slashing.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "could not hash tree root for attester slashing")
		}

		slashings[root] = slashing
	}

	return slashings, nil
}

// updatedChunkByChunkIndex loads the chunks from the database for validators corresponding to
// the `validatorChunkIndex`.
// It then updates the chunks with the neutral element for corresponding validators from
// the epoch just after the latest updated epoch to the current epoch.
// A mapping between chunk index and chunk is returned to the caller.
func (s *Service) updatedChunkByChunkIndex(
	ctx context.Context,
	chunkKind slashertypes.ChunkKind,
	currentEpoch primitives.Epoch,
	validatorChunkIndex uint64,
) (map[uint64]Chunker, error) {
	// Every validator may have a first epoch to update.
	// For a given validator,
	// - If it has no latest updated epoch, then the first epoch to update is set to 0.
	// - If the latest updated epoch is the current epoch, then there is no epoch to update.
	//   Thus, then there is no first epoch to update.
	// - In all other cases, the first epoch to update is the latest updated epoch + 1.

	// minFirstEpochToUpdate is set to the smallest first epoch to update for all validators in the chunk
	// corresponding to the `validatorChunkIndex`.
	var (
		minFirstEpochToUpdate *primitives.Epoch
		neededChunkIndexesMap map[uint64]bool

		err error
	)
	validatorIndexes := s.params.ValidatorIndexesInChunk(validatorChunkIndex)

	if neededChunkIndexesMap, err = s.findNeededChunkIndexes(validatorIndexes, currentEpoch, minFirstEpochToUpdate); err != nil {
		return nil, errors.Wrap(err, "could not find the needed chunk indexed")
	}

	// Transform the map of needed chunk indexes to a slice.
	neededChunkIndexes := slices.Collect(maps.Keys(neededChunkIndexesMap))

	// Retrieve needed chunks from the database.
	chunkByChunkIndex, err := s.loadChunksFromDisk(ctx, validatorChunkIndex, chunkKind, neededChunkIndexes)
	if err != nil {
		return nil, errors.Wrap(err, "could not load chunks from disk")
	}

	for _, validatorIndex := range validatorIndexes {
		// Retrieve the first epoch to write for the validator index.
		isAnEpochToUpdate, firstEpochToUpdate, err := s.firstEpochToUpdate(validatorIndex, currentEpoch)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get first epoch to write for validator index %d with current epoch %d", validatorIndex, currentEpoch)
		}

		if !isAnEpochToUpdate {
			// If there is no epoch to write, skip.
			continue
		}

		epochToUpdate := firstEpochToUpdate

		for epochToUpdate <= currentEpoch {
			// Get the chunk index for the epoch to write.
			chunkIndex := s.params.chunkIndex(epochToUpdate)

			// Get the chunk corresponding to the chunk index from the `chunkByChunkIndex` map.
			currentChunk, ok := chunkByChunkIndex[chunkIndex]
			if !ok {
				return nil, errors.Errorf("chunk at index %d does not exist", chunkIndex)
			}

			// Update the current chunk with the neutral element for the validator index for the epoch to write.
			for s.params.chunkIndex(epochToUpdate) == chunkIndex && epochToUpdate <= currentEpoch {
				if err := setChunkRawDistance(
					s.params,
					currentChunk.Chunk(),
					validatorIndex,
					epochToUpdate,
					currentChunk.NeutralElement(),
				); err != nil {
					return nil, err
				}

				epochToUpdate++
			}

			chunkByChunkIndex[chunkIndex] = currentChunk
		}
	}

	return chunkByChunkIndex, nil
}

// findNeededChunkIndexes returns a map of chunk indexes
// it loops over the validator indexes and finds the first epoch to update for each validator index.
func (s *Service) findNeededChunkIndexes(
	validatorIndexes []primitives.ValidatorIndex,
	currentEpoch primitives.Epoch,
	minFirstEpochToUpdate *primitives.Epoch,
) (map[uint64]bool, error) {
	neededChunkIndexesMap := map[uint64]bool{}

	for _, validatorIndex := range validatorIndexes {
		// Retrieve the first epoch to write for the validator index.
		isAnEpochToUpdate, firstEpochToUpdate, err := s.firstEpochToUpdate(validatorIndex, currentEpoch)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get first epoch to write for validator index %d with current epoch %d", validatorIndex, currentEpoch)
		}

		if !isAnEpochToUpdate {
			// If there is no epoch to write, skip.
			continue
		}

		// If, for this validator index, the chunk corresponding to the first epoch to write
		// (and all following epochs until the current epoch) are already flagged as needed,
		// skip.
		if minFirstEpochToUpdate != nil && *minFirstEpochToUpdate <= firstEpochToUpdate {
			continue
		}

		minFirstEpochToUpdate = &firstEpochToUpdate

		// Add new needed chunk indexes to the map.
		for i := firstEpochToUpdate; i <= currentEpoch; i++ {
			chunkIndex := s.params.chunkIndex(i)
			neededChunkIndexesMap[chunkIndex] = true
		}
	}
	return neededChunkIndexesMap, nil
}

// firstEpochToUpdate, given a validator index and the current epoch, returns a boolean indicating
// if there is an epoch to write. If it is the case, it returns the first epoch to write.
func (s *Service) firstEpochToUpdate(validatorIndex primitives.ValidatorIndex, currentEpoch primitives.Epoch) (bool, primitives.Epoch, error) {
	latestEpochUpdated, ok := s.latestEpochUpdatedForValidator[validatorIndex]

	// Start from the epoch just after the latest updated epoch.
	epochToUpdate, err := latestEpochUpdated.SafeAdd(1)
	if err != nil {
		return false, primitives.Epoch(0), errors.Wrap(err, "could not add 1 to latest updated epoch")
	}

	if !ok {
		epochToUpdate = 0
	}

	if latestEpochUpdated == currentEpoch {
		// If the latest updated epoch is the current epoch, we do not need to update anything.
		return false, primitives.Epoch(0), nil
	}

	// Latest updated epoch should not be greater than the current epoch.
	if latestEpochUpdated > currentEpoch {
		return false, primitives.Epoch(0), errors.Errorf("epoch to write `%d` should not be greater than the current epoch `%d`", epochToUpdate, currentEpoch)
	}

	// It is useless to update more than `historyLength` epochs, since
	// the chunks are circular and we will be overwritten at least one.
	if currentEpoch-epochToUpdate >= s.params.historyLength {
		epochToUpdate = currentEpoch + 1 - s.params.historyLength
	}

	return true, epochToUpdate, nil
}

// Updates spans and detects any slashable attester offenses along the way.
//  1. Determine the chunks we need to use for updating for the validator indices
//     in a validator chunk index, then retrieve those chunks from the database.
//  2. Using the chunks from step (1):
//     for every attestation by chunk index:
//     for each validator in the attestation's attesting indices:
//     - Check if the attestation is slashable, if so return a slashing object.
//  3. Save the updated chunks to disk.
func (s *Service) updateSpans(
	ctx context.Context,
	updatedChunks map[uint64]Chunker,
	attWrapperByChunkIdx map[uint64][]*slashertypes.IndexedAttestationWrapper,
	kind slashertypes.ChunkKind,
	validatorChunkIndex uint64,
	currentEpoch primitives.Epoch,
) (map[[fieldparams.RootLength]byte]ethpb.AttSlashing, error) {
	ctx, span := trace.StartSpan(ctx, "Slasher.updateSpans")
	defer span.End()

	// Apply the attestations to the related chunks and find any
	// slashings along the way.
	slashings := map[[fieldparams.RootLength]byte]ethpb.AttSlashing{}

	for _, attWrappers := range attWrapperByChunkIdx {
		for _, attWrapper := range attWrappers {
			for _, validatorIdx := range attWrapper.IndexedAttestation.GetAttestingIndices() {
				validatorIndex := primitives.ValidatorIndex(validatorIdx)
				computedValidatorChunkIdx := s.params.validatorChunkIndex(validatorIndex)

				// Every validator chunk index represents a range of validators.
				// It is possible that the validator index in this loop iteration is
				// not part of the validator chunk index we are updating chunks for.
				//
				// For example, if there are 4 validators per validator chunk index,
				// then validator chunk index 0 contains validator indices [0, 1, 2, 3]
				// If we see an attestation with attesting indices [3, 4, 5] and we are updating
				// chunks for validator chunk index 0, only validator index 3 should make
				// it past this line.
				if validatorChunkIndex != computedValidatorChunkIdx {
					continue
				}

				slashing, err := s.applyAttestationForValidator(
					ctx, updatedChunks, attWrapper, kind, validatorChunkIndex, validatorIndex, currentEpoch,
				)

				if err != nil {
					return nil, errors.Wrapf(err, "could not apply attestation for validator index %d", validatorIndex)
				}

				if slashing == nil {
					continue
				}

				root, err := slashing.HashTreeRoot()
				if err != nil {
					return nil, errors.Wrap(err, "could not hash tree root for attester slashing")
				}

				slashings[root] = slashing
			}
		}
	}

	// Write the updated chunks to disk.
	return slashings, nil
}

// Checks if an incoming attestation is slashable based on the validator chunk it
// corresponds to. If a slashable offense is found, we return it to the caller.
// If not, then update every single chunk the attestation covers, starting from its
// source epoch up to its target.
func (s *Service) applyAttestationForValidator(
	ctx context.Context,
	chunksByChunkIdx map[uint64]Chunker,
	attestation *slashertypes.IndexedAttestationWrapper,
	chunkKind slashertypes.ChunkKind,
	validatorChunkIndex uint64,
	validatorIndex primitives.ValidatorIndex,
	currentEpoch primitives.Epoch,
) (ethpb.AttSlashing, error) {
	ctx, span := trace.StartSpan(ctx, "Slasher.applyAttestationForValidator")
	defer span.End()

	var err error

	sourceEpoch := attestation.IndexedAttestation.GetData().Source.Epoch
	targetEpoch := attestation.IndexedAttestation.GetData().Target.Epoch

	attestationDistance.Observe(float64(targetEpoch) - float64(sourceEpoch))
	chunkIndex := s.params.chunkIndex(sourceEpoch)

	chunk, ok := chunksByChunkIdx[chunkIndex]
	if !ok {
		chunk, err = s.getChunkFromDatabase(ctx, chunkKind, validatorChunkIndex, chunkIndex)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get chunk at index %d", chunkIndex)
		}
	}

	// Check slashable, if so, return the slashing.
	slashing, err := chunk.CheckSlashable(
		ctx,
		s.serviceCfg.Database,
		validatorIndex,
		attestation,
	)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"could not check if attestation for validator index %d is slashable",
			validatorIndex,
		)
	}
	if slashing != nil {
		return slashing, nil
	}

	// Get the first start epoch for the chunk. If it does not exist or
	// is not possible based on the input arguments, do not continue with the update.
	startEpoch, exists := chunk.StartEpoch(sourceEpoch, currentEpoch)
	if !exists {
		return nil, nil
	}

	// Given a single attestation could span across multiple chunks
	// for a validator min or max span, we attempt to update the current chunk
	// for the source epoch of the attestation. If the update function tells
	// us we need to proceed to the next chunk, we continue by determining
	// the start epoch of the next chunk. We exit once no longer need to
	// keep updating chunks.
	for {
		chunkIndex = s.params.chunkIndex(startEpoch)

		chunk, ok := chunksByChunkIdx[chunkIndex]
		if !ok {
			chunk, err = s.getChunkFromDatabase(ctx, chunkKind, validatorChunkIndex, chunkIndex)
			if err != nil {
				return nil, errors.Wrapf(err, "could not get chunk at index %d", chunkIndex)
			}
		}

		keepGoing, err := chunk.Update(
			chunkIndex,
			currentEpoch,
			validatorIndex,
			startEpoch,
			targetEpoch,
		)

		if err != nil {
			return nil, errors.Wrapf(
				err,
				"could not update chunk at chunk index %d for validator index %d and current epoch %d",
				chunkIndex,
				validatorIndex,
				currentEpoch,
			)
		}

		// We update the chunksByChunkIdx map with the chunk we just updated.
		chunksByChunkIdx[chunkIndex] = chunk
		if !keepGoing {
			break
		}

		// Move to first epoch of next chunk if needed.
		startEpoch = chunk.NextChunkStartEpoch(startEpoch)
	}

	return nil, nil
}

// Retrieve a chunk from database.
func (s *Service) getChunkFromDatabase(
	ctx context.Context,
	chunkKind slashertypes.ChunkKind,
	validatorChunkIndex uint64,
	chunkIndex uint64,
) (Chunker, error) {
	// We can ensure we load the appropriate chunk we need by fetching from the DB.
	diskChunks, err := s.loadChunksFromDisk(ctx, validatorChunkIndex, chunkKind, []uint64{chunkIndex})
	if err != nil {
		return nil, errors.Wrapf(err, "could not load chunk at index %d", chunkIndex)
	}

	if chunk, ok := diskChunks[chunkIndex]; ok {
		return chunk, nil
	}

	return nil, fmt.Errorf("could not retrieve chunk at chunk index %d from disk", chunkIndex)
}

// Load chunks for a specified list of chunk indices. We attempt to load it from the database.
// If the data exists, then we initialize a chunk of a specified kind. Otherwise, we create
// an empty chunk, add it to our map, and then return it to the caller.
func (s *Service) loadChunksFromDisk(
	ctx context.Context,
	validatorChunkIndex uint64,
	chunkKind slashertypes.ChunkKind,
	chunkIndexes []uint64,
) (map[uint64]Chunker, error) {
	ctx, span := trace.StartSpan(ctx, "Slasher.loadChunks")
	defer span.End()

	chunksCount := len(chunkIndexes)

	if chunksCount == 0 {
		return map[uint64]Chunker{}, nil
	}

	// Build chunk keys.
	chunkKeys := make([][]byte, 0, chunksCount)
	for _, chunkIndex := range chunkIndexes {
		chunkKey := s.params.flatSliceID(validatorChunkIndex, chunkIndex)
		chunkKeys = append(chunkKeys, chunkKey)
	}

	// Load the chunks from the database.
	rawChunks, chunksExist, err := s.serviceCfg.Database.LoadSlasherChunks(ctx, chunkKind, chunkKeys)
	if err != nil {
		return nil, errors.Wrapf(err, "could not load slasher chunk index")
	}

	// Perform basic checks.
	if len(rawChunks) != chunksCount {
		return nil, errors.Errorf("expected %d chunks, got %d", chunksCount, len(rawChunks))
	}

	if len(chunksExist) != chunksCount {
		return nil, errors.Errorf("expected %d chunks exist, got %d", chunksCount, len(chunksExist))
	}

	// Initialize the chunks.
	chunksByChunkIdx := make(map[uint64]Chunker, chunksCount)
	for i := 0; i < len(rawChunks); i++ {
		// If the chunk exists in the database, we initialize it from the raw bytes data.
		// If it does not exist, we initialize an empty chunk.
		var (
			chunk Chunker
			err   error
		)

		chunkExists := chunksExist[i]

		switch chunkKind {
		case slashertypes.MinSpan:
			if chunkExists {
				chunk, err = MinChunkSpansSliceFrom(s.params, rawChunks[i])
				break
			}
			chunk = EmptyMinSpanChunksSlice(s.params)

		case slashertypes.MaxSpan:
			if chunkExists {
				chunk, err = MaxChunkSpansSliceFrom(s.params, rawChunks[i])
				break
			}
			chunk = EmptyMaxSpanChunksSlice(s.params)
		}

		if err != nil {
			return nil, errors.Wrap(err, "could not initialize chunk")
		}

		chunksByChunkIdx[chunkIndexes[i]] = chunk
	}

	return chunksByChunkIdx, nil
}

func (s *Service) saveChunksToDisk(
	ctx context.Context,
	chunkKind slashertypes.ChunkKind,
	chunkByChunkIndexByValidatorChunkIndex map[uint64]map[uint64]Chunker,
) error {
	ctx, span := trace.StartSpan(ctx, "Slasher.saveChunksToDisk")
	defer span.End()

	// Compute the total number of chunks to save.
	chunksCount := 0
	for _, chunkByChunkIndex := range chunkByChunkIndexByValidatorChunkIndex {
		chunksCount += len(chunkByChunkIndex)
	}

	// Create needed arrays.
	chunkKeys := make([][]byte, 0, chunksCount)
	chunks := make([][]uint16, 0, chunksCount)

	// Fill the arrays.
	for validatorChunkIndex, chunkByChunkIndex := range chunkByChunkIndexByValidatorChunkIndex {
		for chunkIndex, chunk := range chunkByChunkIndex {
			chunkKeys = append(chunkKeys, s.params.flatSliceID(validatorChunkIndex, chunkIndex))
			chunks = append(chunks, chunk.Chunk())
		}
	}

	// Update prometheus metrics.
	chunksSavedTotal.Add(float64(chunksCount))

	// Save the chunks to disk.
	return s.serviceCfg.Database.SaveSlasherChunks(ctx, chunkKind, chunkKeys, chunks)
}
