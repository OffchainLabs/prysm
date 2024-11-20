package slasherkv

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"

	"github.com/golang/snappy"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	slashertypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/slasher/types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
)

const (
	attestationRecordKeySize = 32 // Bytes.
	rootSize                 = 32 // Bytes.

	// For database performance reasons, database read/write operations
	// are chunked into batches of maximum `batchSize` elements.
	batchSize = 10_000
)

// LastEpochWrittenForValidators given a list of validator indices returns the latest
// epoch we have recorded the validators writing data for.
func (s *Store) LastEpochWrittenForValidators(
	ctx context.Context, validatorIndexes []primitives.ValidatorIndex,
) ([]*slashertypes.AttestedEpochForValidator, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.LastEpochWrittenForValidators")
	defer span.End()

	attestedEpochs := make([]*slashertypes.AttestedEpochForValidator, 0)

	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(attestedEpochsByValidator)

		for _, validatorIndex := range validatorIndexes {
			encodedIndex := encodeValidatorIndex(validatorIndex)

			epochBytes := bkt.Get(encodedIndex)
			if epochBytes == nil {
				// If there is no epoch for this validator, skip to the next validator.
				continue
			}

			var epoch primitives.Epoch
			if err := epoch.UnmarshalSSZ(epochBytes); err != nil {
				return err
			}

			attestedEpoch := &slashertypes.AttestedEpochForValidator{
				ValidatorIndex: validatorIndex,
				Epoch:          epoch,
			}

			attestedEpochs = append(attestedEpochs, attestedEpoch)
		}

		return nil
	})

	return attestedEpochs, err
}

// SaveLastEpochWrittenForValidators saves the latest epoch
// that each validator has attested to in the provided map.
func (s *Store) SaveLastEpochWrittenForValidators(
	ctx context.Context, epochByValIndex map[primitives.ValidatorIndex]primitives.Epoch,
) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveLastEpochWrittenForValidators")
	defer span.End()

	const batchSize = 10000

	encodedIndexes := make([][]byte, 0, len(epochByValIndex))
	encodedEpochs := make([][]byte, 0, len(epochByValIndex))

	for valIndex, epoch := range epochByValIndex {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		encodedIndex := encodeValidatorIndex(valIndex)

		encodedEpoch, err := epoch.MarshalSSZ()
		if err != nil {
			return err
		}

		encodedIndexes = append(encodedIndexes, encodedIndex)
		encodedEpochs = append(encodedEpochs, encodedEpoch)
	}

	// The list of validators might be too massive for boltdb to handle in a single transaction,
	// so instead we split it into batches and write each batch.
	for start := 0; start < len(encodedIndexes); start += batchSize {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := s.db.Update(func(tx *bolt.Tx) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			bkt := tx.Bucket(attestedEpochsByValidator)
			end := min(start+batchSize, len(encodedIndexes))

			for j, encodedIndex := range encodedIndexes[start:end] {
				if ctx.Err() != nil {
					return ctx.Err()
				}

				if err := bkt.Put(encodedIndex, encodedEpochs[j+start]); err != nil {
					return err
				}
			}

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

// CheckAttesterDoubleVotes retrieves any slashable double votes that exist
// for a series of input attestations with respect to the database.
func (s *Store) CheckAttesterDoubleVotes(
	ctx context.Context, attestations []*slashertypes.IndexedAttestationWrapper,
) ([]*slashertypes.AttesterDoubleVote, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.CheckAttesterDoubleVotes")
	defer span.End()

	doubleVotes := make([]*slashertypes.AttesterDoubleVote, 0)
	mu := sync.Mutex{}
	eg, egctx := errgroup.WithContext(ctx)

	for _, attestation := range attestations {
		// Copy the iteration instance to a local variable to give each go-routine its own copy to play with.
		// See https://golang.org/doc/faq#closures_and_goroutines for more details.
		attToProcess := attestation

		// Process each attestation in parallel.
		eg.Go(func() error {
			err := s.db.View(func(tx *bolt.Tx) error {
				signingRootsBkt := tx.Bucket(attestationDataRootsBucket)
				attRecordsBkt := tx.Bucket(attestationRecordsBucket)

				encEpoch := encodeTargetEpoch(attToProcess.IndexedAttestation.GetData().Target.Epoch)
				localDoubleVotes := make([]*slashertypes.AttesterDoubleVote, 0)

				for _, valIdx := range attToProcess.IndexedAttestation.GetAttestingIndices() {
					// Check if there is signing root in the database for this combination
					// of validator index and target epoch.
					encIdx := encodeValidatorIndex(primitives.ValidatorIndex(valIdx))
					validatorEpochKey := append(encEpoch, encIdx...)
					attRecordsKey := signingRootsBkt.Get(validatorEpochKey)

					// An attestation record key consists of a signing root (32 bytes).
					if len(attRecordsKey) < attestationRecordKeySize {
						// If there is no signing root for this combination,
						// then there is no double vote. We can continue to the next validator.
						continue
					}

					// Retrieve the attestation record corresponding to the data root
					// from the database.
					encExistingAttRecord := attRecordsBkt.Get(attRecordsKey)
					if encExistingAttRecord == nil {
						continue
					}

					existingDataRoot := bytesutil.ToBytes32(attRecordsKey[:rootSize])
					if existingDataRoot == attToProcess.DataRoot {
						continue
					}

					// There is a double vote.
					existingAttRecord, err := decodeAttestationRecord(encExistingAttRecord)
					if err != nil {
						return err
					}

					// Build the proof of double vote.
					slashAtt := &slashertypes.AttesterDoubleVote{
						ValidatorIndex: primitives.ValidatorIndex(valIdx),
						Target:         attToProcess.IndexedAttestation.GetData().Target.Epoch,
						Wrapper_1:      existingAttRecord,
						Wrapper_2:      attToProcess,
					}

					localDoubleVotes = append(localDoubleVotes, slashAtt)
				}

				// If any routine is cancelled, then cancel this routine too.
				select {
				case <-egctx.Done():
					return egctx.Err()
				default:
				}

				// If there are any double votes in this attestation, add it to the global double votes.
				if len(localDoubleVotes) > 0 {
					mu.Lock()
					defer mu.Unlock()
					doubleVotes = append(doubleVotes, localDoubleVotes...)
				}

				return nil
			})

			return err
		})
	}

	return doubleVotes, eg.Wait()
}

// AttestationRecordForValidator given a validator index and a target epoch,
// retrieves an existing attestation record we have stored in the database.
func (s *Store) AttestationRecordForValidator(
	ctx context.Context, validatorIdx primitives.ValidatorIndex, targetEpoch primitives.Epoch,
) (*slashertypes.IndexedAttestationWrapper, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.AttestationRecordForValidator")
	defer span.End()
	var record *slashertypes.IndexedAttestationWrapper
	encIdx := encodeValidatorIndex(validatorIdx)
	encEpoch := encodeTargetEpoch(targetEpoch)
	key := append(encEpoch, encIdx...)
	err := s.db.View(func(tx *bolt.Tx) error {
		signingRootsBkt := tx.Bucket(attestationDataRootsBucket)
		attRecordKey := signingRootsBkt.Get(key)
		if attRecordKey == nil {
			return nil
		}
		attRecordsBkt := tx.Bucket(attestationRecordsBucket)
		indexedAttBytes := attRecordsBkt.Get(attRecordKey)
		if indexedAttBytes == nil {
			return nil
		}
		decoded, err := decodeAttestationRecord(indexedAttBytes)
		if err != nil {
			return err
		}
		record = decoded
		return nil
	})
	return record, err
}

// SaveAttestationRecordsForValidators saves attestation records for the specified indices.
// If multiple attestations are provided for the same validator index + target epoch combination,
// then only the first one is (arbitrarily) saved in the `attestationDataRootsBucket` bucket.
func (s *Store) SaveAttestationRecordsForValidators(
	ctx context.Context,
	attWrappers []*slashertypes.IndexedAttestationWrapper,
) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveAttestationRecordsForValidators")
	defer span.End()

	attWrappersCount := len(attWrappers)

	// If no attestations are provided, skip.
	if attWrappersCount == 0 {
		return nil
	}

	// Build encoded target epochs and encoded records
	encodedTargetEpoch := make([][]byte, attWrappersCount)
	encodedRecords := make([][]byte, attWrappersCount)

	for i, attestation := range attWrappers {
		encEpoch := encodeTargetEpoch(attestation.IndexedAttestation.GetData().Target.Epoch)

		value, err := encodeAttestationRecord(attestation)
		if err != nil {
			return err
		}

		encodedTargetEpoch[i] = encEpoch
		encodedRecords[i] = value
	}

	// Save attestation records in the database by batch.
	for stop := attWrappersCount; stop >= 0; stop -= batchSize {
		start := max(0, stop-batchSize)

		attWrappersBatch := attWrappers[start:stop]
		encodedTargetEpochBatch := encodedTargetEpoch[start:stop]
		encodedRecordsBatch := encodedRecords[start:stop]

		// Perform basic check.
		if len(encodedTargetEpochBatch) != len(encodedRecordsBatch) {
			return fmt.Errorf(
				"cannot save attestation records, got %d target epochs and %d records",
				len(encodedTargetEpochBatch), len(encodedRecordsBatch),
			)
		}

		currentBatchSize := len(encodedTargetEpochBatch)

		// Save attestation records in the database.
		if err := s.db.Update(func(tx *bolt.Tx) error {
			attRecordsBkt := tx.Bucket(attestationRecordsBucket)
			dataRootsBkt := tx.Bucket(attestationDataRootsBucket)

			for i := currentBatchSize - 1; i >= 0; i-- {
				attWrapper := attWrappersBatch[i]
				dataRoot := attWrapper.DataRoot

				encodedTargetEpoch := encodedTargetEpochBatch[i]
				encodedRecord := encodedRecordsBatch[i]

				if err := attRecordsBkt.Put(dataRoot[:], encodedRecord); err != nil {
					return err
				}

				for _, validatorIndex := range attWrapper.IndexedAttestation.GetAttestingIndices() {
					encodedIndex := encodeValidatorIndex(primitives.ValidatorIndex(validatorIndex))

					key := append(encodedTargetEpoch, encodedIndex...)
					if err := dataRootsBkt.Put(key, dataRoot[:]); err != nil {
						return err
					}
				}
			}

			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to save attestation records")
		}
	}

	return nil
}

// LoadSlasherChunks given a chunk kind and a disk keys, retrieves chunks for a validator
// min or max span used by slasher from our database.
func (s *Store) LoadSlasherChunks(
	ctx context.Context, kind slashertypes.ChunkKind, chunkKeys [][]byte,
) ([][]uint16, []bool, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.LoadSlasherChunk")
	defer span.End()

	keysCount := len(chunkKeys)

	chunks := make([][]uint16, 0, keysCount)
	exists := make([]bool, 0, keysCount)
	encodedKeys := make([][]byte, 0, keysCount)

	// Encode kind.
	encodedKind := ssz.MarshalUint8(make([]byte, 0), uint8(kind))

	// Encode keys.
	for _, chunkKey := range chunkKeys {
		encodedKey := append(encodedKind, chunkKey...)
		encodedKeys = append(encodedKeys, encodedKey)
	}

	// Read chunks from the database by batch.
	for start := 0; start < keysCount; start += batchSize {
		stop := min(start+batchSize, len(encodedKeys))
		encodedKeysBatch := encodedKeys[start:stop]

		if err := s.db.View(func(tx *bolt.Tx) error {
			bkt := tx.Bucket(slasherChunksBucket)

			for _, encodedKey := range encodedKeysBatch {
				chunkBytes := bkt.Get(encodedKey)

				if chunkBytes == nil {
					chunks = append(chunks, []uint16{})
					exists = append(exists, false)
					continue
				}

				chunk, err := decodeSlasherChunk(chunkBytes)
				if err != nil {
					return err
				}

				chunks = append(chunks, chunk)
				exists = append(exists, true)
			}

			return nil
		}); err != nil {
			return nil, nil, err
		}
	}

	return chunks, exists, nil
}

// SaveSlasherChunks given a chunk kind, list of disk keys, and list of chunks,
// saves the chunks to our database for use by slasher in slashing detection.
func (s *Store) SaveSlasherChunks(
	ctx context.Context, kind slashertypes.ChunkKind, chunkKeys [][]byte, chunks [][]uint16,
) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveSlasherChunks")
	defer span.End()

	// Ensure we have the same number of keys and chunks.
	if len(chunkKeys) != len(chunks) {
		return fmt.Errorf(
			"cannot save slasher chunks, got %d keys and %d chunks",
			len(chunkKeys), len(chunks),
		)
	}

	chunksCount := len(chunks)

	// Encode kind.
	encodedKind := ssz.MarshalUint8(make([]byte, 0), uint8(kind))

	// Encode keys and chunks.
	encodedKeys := make([][]byte, chunksCount)
	encodedChunks := make([][]byte, chunksCount)

	for i := 0; i < chunksCount; i++ {
		chunkKey, chunk := chunkKeys[i], chunks[i]
		encodedKey := append(encodedKind, chunkKey...)

		encodedChunk, err := encodeSlasherChunk(chunk)
		if err != nil {
			return errors.Wrapf(err, "failed to encode slasher chunk for key %v", chunkKey)
		}

		encodedKeys[i] = encodedKey
		encodedChunks[i] = encodedChunk
	}

	// Save chunks in the database by batch.
	for start := 0; start < chunksCount; start += batchSize {
		stop := min(start+batchSize, len(encodedKeys))
		encodedKeysBatch := encodedKeys[start:stop]
		encodedChunksBatch := encodedChunks[start:stop]
		batchSize := len(encodedKeysBatch)

		if err := s.db.Update(func(tx *bolt.Tx) error {
			bkt := tx.Bucket(slasherChunksBucket)

			for i := 0; i < batchSize; i++ {
				if err := bkt.Put(encodedKeysBatch[i], encodedChunksBatch[i]); err != nil {
					return err
				}
			}

			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to save slasher chunks")
		}
	}

	return nil
}

// CheckDoubleBlockProposals takes in a list of proposals and for each,
// checks if there already exists a proposal at the same slot+validatorIndex combination.
// If so, it checks if the existing signing root is not-empty and is different than
// the incoming proposal signing root.
// If so, it returns a double block proposal object.
func (s *Store) CheckDoubleBlockProposals(
	ctx context.Context, incomingProposals []*slashertypes.SignedBlockHeaderWrapper,
) ([]*ethpb.ProposerSlashing, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.CheckDoubleBlockProposals")
	defer span.End()

	proposerSlashings := make([]*ethpb.ProposerSlashing, 0, len(incomingProposals))

	err := s.db.View(func(tx *bolt.Tx) error {
		// Retrieve the proposal records bucket
		bkt := tx.Bucket(proposalRecordsBucket)

		for _, incomingProposal := range incomingProposals {
			// Build the key corresponding to this slot + validator index combination
			key := keyForValidatorProposal(
				incomingProposal.SignedBeaconBlockHeader.Header.Slot,
				incomingProposal.SignedBeaconBlockHeader.Header.ProposerIndex,
			)

			// Retrieve the existing proposal record from the database
			encExistingProposalWrapper := bkt.Get(key)

			// If there is no existing proposal record (empty result), then there is no double proposal.
			// We can continue to the next proposal.
			if len(encExistingProposalWrapper) < rootSize {
				continue
			}

			// Compare the proposal signing root in the DB with the incoming proposal signing root.
			// If they differ, we have a double proposal.
			existingRoot := bytesutil.ToBytes32(encExistingProposalWrapper[:rootSize])
			if existingRoot != incomingProposal.HeaderRoot {
				existingProposalWrapper, err := decodeProposalRecord(encExistingProposalWrapper)
				if err != nil {
					return err
				}

				proposerSlashings = append(proposerSlashings, &ethpb.ProposerSlashing{
					Header_1: existingProposalWrapper.SignedBeaconBlockHeader,
					Header_2: incomingProposal.SignedBeaconBlockHeader,
				})
			}
		}
		return nil
	})

	return proposerSlashings, err
}

// BlockProposalForValidator given a validator index and a slot
// retrieves an existing proposal record we have stored in the database.
func (s *Store) BlockProposalForValidator(
	ctx context.Context, validatorIdx primitives.ValidatorIndex, slot primitives.Slot,
) (*slashertypes.SignedBlockHeaderWrapper, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.BlockProposalForValidator")
	defer span.End()
	var record *slashertypes.SignedBlockHeaderWrapper
	key := keyForValidatorProposal(slot, validatorIdx)

	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(proposalRecordsBucket)
		encProposal := bkt.Get(key)
		if encProposal == nil {
			return nil
		}
		decoded, err := decodeProposalRecord(encProposal)
		if err != nil {
			return err
		}
		record = decoded
		return nil
	})
	return record, err
}

// SaveBlockProposals takes in a list of block proposals and saves them to our
// proposal records bucket in the database.
// If multiple proposals are provided for the same slot + validatorIndex combination,
// then only the last one is saved in the database.
func (s *Store) SaveBlockProposals(
	ctx context.Context, proposals []*slashertypes.SignedBlockHeaderWrapper,
) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveBlockProposals")
	defer span.End()

	encodedKeys := make([][]byte, len(proposals))
	encodedProposals := make([][]byte, len(proposals))

	// Loop over all proposals to encode keys and proposals themselves.
	for i, proposal := range proposals {
		// Encode the key for this proposal.
		key := keyForValidatorProposal(
			proposal.SignedBeaconBlockHeader.Header.Slot,
			proposal.SignedBeaconBlockHeader.Header.ProposerIndex,
		)

		// Encode the proposal itself.
		enc, err := encodeProposalRecord(proposal)
		if err != nil {
			return err
		}

		encodedKeys[i] = key
		encodedProposals[i] = enc
	}

	// All proposals are saved into the DB in a single transaction.
	return s.db.Update(func(tx *bolt.Tx) error {
		// Retrieve the proposal records bucket.
		bkt := tx.Bucket(proposalRecordsBucket)

		// Save all proposals.
		for i := range proposals {
			if err := bkt.Put(encodedKeys[i], encodedProposals[i]); err != nil {
				return err
			}
		}

		return nil
	})
}

// HighestAttestations retrieves the last attestation data from the database for all indices.
func (s *Store) HighestAttestations(
	_ context.Context,
	indices []primitives.ValidatorIndex,
) ([]*ethpb.HighestAttestation, error) {
	if len(indices) == 0 {
		return nil, nil
	}
	// Sort indices to keep DB interactions short.
	sort.SliceStable(indices, func(i, j int) bool {
		return uint64(indices[i]) < uint64(indices[j])
	})

	var err error
	encodedIndices := make([][]byte, len(indices))
	for i, valIdx := range indices {
		encodedIndices[i] = encodeValidatorIndex(valIdx)
	}

	history := make([]*ethpb.HighestAttestation, 0, len(encodedIndices))
	err = s.db.View(func(tx *bolt.Tx) error {
		signingRootsBkt := tx.Bucket(attestationDataRootsBucket)
		attRecordsBkt := tx.Bucket(attestationRecordsBucket)
		for i := 0; i < len(encodedIndices); i++ {
			c := signingRootsBkt.Cursor()
			for k, v := c.Last(); k != nil; k, v = c.Prev() {
				if suffixForAttestationRecordsKey(k, encodedIndices[i]) {
					encodedAttRecord := attRecordsBkt.Get(v)
					if encodedAttRecord == nil {
						continue
					}
					attWrapper, err := decodeAttestationRecord(encodedAttRecord)
					if err != nil {
						return err
					}
					highestAtt := &ethpb.HighestAttestation{
						ValidatorIndex:     uint64(indices[i]),
						HighestSourceEpoch: attWrapper.IndexedAttestation.GetData().Source.Epoch,
						HighestTargetEpoch: attWrapper.IndexedAttestation.GetData().Target.Epoch,
					}
					history = append(history, highestAtt)
					break
				}
			}
		}
		return nil
	})
	return history, err
}

func suffixForAttestationRecordsKey(key, encodedValidatorIndex []byte) bool {
	encIdx := key[8:]
	return bytes.Equal(encIdx, encodedValidatorIndex)
}

// keyForValidatorProposal returns a disk key for a validator proposal, including a slot+validatorIndex as a byte slice.
func keyForValidatorProposal(slot primitives.Slot, proposerIndex primitives.ValidatorIndex) []byte {
	encSlot := make([]byte, 8)
	binary.BigEndian.PutUint64(encSlot, uint64(slot))
	encValidatorIdx := encodeValidatorIndex(proposerIndex)
	return append(encSlot, encValidatorIdx...)
}

func encodeSlasherChunk(chunk []uint16) ([]byte, error) {
	val := make([]byte, 0)
	for i := 0; i < len(chunk); i++ {
		val = append(val, ssz.MarshalUint16(make([]byte, 0), chunk[i])...)
	}
	if len(val) == 0 {
		return nil, errors.New("cannot encode empty chunk")
	}
	return snappy.Encode(nil, val), nil
}

func decodeSlasherChunk(enc []byte) ([]uint16, error) {
	chunkBytes, err := snappy.Decode(nil, enc)
	if err != nil {
		return nil, err
	}
	if len(chunkBytes)%2 != 0 {
		return nil, fmt.Errorf(
			"cannot decode slasher chunk with length %d, must be a multiple of 2",
			len(chunkBytes),
		)
	}
	chunk := make([]uint16, 0)
	for i := 0; i < len(chunkBytes); i += 2 {
		distance := ssz.UnmarshallUint16(chunkBytes[i : i+2])
		chunk = append(chunk, distance)
	}
	return chunk, nil
}

// Encode attestation record to bytes.
// The output encoded attestation record consists in the signing root concatenated with the compressed attestation record.
func encodeAttestationRecord(att *slashertypes.IndexedAttestationWrapper) ([]byte, error) {
	if att == nil || att.IndexedAttestation == nil || att.IndexedAttestation.IsNil() {
		return []byte{}, errors.New("nil proposal record")
	}

	// Encode attestation.
	encodedAtt, err := att.IndexedAttestation.MarshalSSZ()
	if err != nil {
		return nil, err
	}

	// Compress attestation.
	compressedAtt := snappy.Encode(nil, encodedAtt)

	return append(att.DataRoot[:], compressedAtt...), nil
}

// Decode attestation record from bytes.
// The input encoded attestation record consists in the signing root concatenated with the compressed attestation record.
func decodeAttestationRecord(encoded []byte) (*slashertypes.IndexedAttestationWrapper, error) {
	if len(encoded) < rootSize {
		return nil, fmt.Errorf("wrong length for encoded attestation record, want minimum %d, got %d", rootSize, len(encoded))
	}

	// Decompress attestation.
	decodedAttBytes, err := snappy.Decode(nil, encoded[rootSize:])
	if err != nil {
		return nil, err
	}

	// Decode attestation.
	decodedAtt := &ethpb.IndexedAttestation{}
	if err := decodedAtt.UnmarshalSSZ(decodedAttBytes); err != nil {
		return nil, err
	}

	// Decode signing root.
	dataRootBytes := encoded[:rootSize]
	dataRoot := bytesutil.ToBytes32(dataRootBytes)

	// Return decoded attestation.
	attestation := &slashertypes.IndexedAttestationWrapper{
		IndexedAttestation: decodedAtt,
		DataRoot:           dataRoot,
	}

	return attestation, nil
}

func encodeProposalRecord(blkHdr *slashertypes.SignedBlockHeaderWrapper) ([]byte, error) {
	if blkHdr == nil || blkHdr.SignedBeaconBlockHeader == nil {
		return []byte{}, errors.New("nil proposal record")
	}
	encodedHdr, err := blkHdr.SignedBeaconBlockHeader.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	compressedHdr := snappy.Encode(nil, encodedHdr)
	return append(blkHdr.HeaderRoot[:], compressedHdr...), nil
}

func decodeProposalRecord(encoded []byte) (*slashertypes.SignedBlockHeaderWrapper, error) {
	if len(encoded) < rootSize {
		return nil, fmt.Errorf(
			"wrong length for encoded proposal record, want %d, got %d", rootSize, len(encoded),
		)
	}
	dataRoot := encoded[:rootSize]
	decodedBlkHdr := &ethpb.SignedBeaconBlockHeader{}
	decodedHdrBytes, err := snappy.Decode(nil, encoded[rootSize:])
	if err != nil {
		return nil, err
	}
	if err := decodedBlkHdr.UnmarshalSSZ(decodedHdrBytes); err != nil {
		return nil, err
	}
	return &slashertypes.SignedBlockHeaderWrapper{
		SignedBeaconBlockHeader: decodedBlkHdr,
		HeaderRoot:              bytesutil.ToBytes32(dataRoot),
	}, nil
}

// Encodes an epoch into big-endian bytes.
func encodeTargetEpoch(epoch primitives.Epoch) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(epoch))
	return buf
}

// Encodes a validator index using 5 bytes instead of 8 as a
// client optimization to save space in the database. Because the max validator
// registry size is 2**40, this is a safe optimization.
func encodeValidatorIndex(index primitives.ValidatorIndex) []byte {
	buf := make([]byte, 5)
	v := uint64(index)
	buf[0] = byte(v)
	buf[1] = byte(v >> 8)
	buf[2] = byte(v >> 16)
	buf[3] = byte(v >> 24)
	buf[4] = byte(v >> 32)
	return buf
}
