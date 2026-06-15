package state_native

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/fieldtrie"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

const (
	finalizedRootIndex = uint64(105) // Precomputed value.
)

// FinalizedRootGeneralizedIndex for the beacon state.
func FinalizedRootGeneralizedIndex() uint64 {
	return finalizedRootIndex
}

// CurrentSyncCommitteeGeneralizedIndex for the beacon state.
func (b *BeaconState) CurrentSyncCommitteeGeneralizedIndex() (uint64, error) {
	if b.version == version.Phase0 {
		return 0, errNotSupported("CurrentSyncCommitteeGeneralizedIndex", b.version)
	}

	return uint64(types.CurrentSyncCommittee.RealPosition()), nil
}

// NextSyncCommitteeGeneralizedIndex for the beacon state.
func (b *BeaconState) NextSyncCommitteeGeneralizedIndex() (uint64, error) {
	if b.version == version.Phase0 {
		return 0, errNotSupported("NextSyncCommitteeGeneralizedIndex", b.version)
	}

	return uint64(types.NextSyncCommittee.RealPosition()), nil
}

// CurrentSyncCommitteeProof from the state's Merkle trie representation.
func (b *BeaconState) CurrentSyncCommitteeProof(ctx context.Context) ([][]byte, error) {
	_, proof, err := b.ProofByFieldPosition(ctx, types.CurrentSyncCommittee.RealPosition())
	return proof, err
}

// NextSyncCommitteeProof from the state's Merkle trie representation.
func (b *BeaconState) NextSyncCommitteeProof(ctx context.Context) ([][]byte, error) {
	_, proof, err := b.ProofByFieldPosition(ctx, types.NextSyncCommittee.RealPosition())
	return proof, err
}

// FinalizedRootProof crafts a Merkle proof for the finalized root
// contained within the finalized checkpoint of a beacon state.
func (b *BeaconState) FinalizedRootProof(ctx context.Context) ([][]byte, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	_, branchProof, err := b.proofByFieldPosition(ctx, types.FinalizedCheckpoint.RealPosition())
	if err != nil {
		return nil, err
	}

	// The epoch field of a finalized checkpoint is the neighbor
	// index of the finalized root field in its Merkle tree representation
	// of the checkpoint. This neighbor is the first element added to the proof.
	epochBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(epochBuf, uint64(b.finalizedCheckpointVal().Epoch))
	epochRoot := bytesutil.ToBytes32(epochBuf)
	proof := make([][]byte, 0)
	proof = append(proof, epochRoot[:])
	proof = append(proof, branchProof...)
	return proof, nil
}

// ProofByFieldPosition constructs proofs for given field index with lock acquisition.
// Returns the field root (leaf) and the proof hashes.
func (b *BeaconState) ProofByFieldPosition(ctx context.Context, pos int) ([]byte, [][]byte, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	return b.proofByFieldPosition(ctx, pos)
}

// proofByFieldPosition constructs proofs for given field position.
// Important: it is assumed that beacon state mutex is locked when calling this method.
// Returns the field root (leaf) and the proof hashes.
func (b *BeaconState) proofByFieldPosition(ctx context.Context, pos int) ([]byte, [][]byte, error) {
	err := b.validateFieldPosition(pos)
	if err != nil {
		return nil, nil, err
	}

	if err := b.initializeMerkleLayers(ctx); err != nil {
		return nil, nil, err
	}
	if err := b.recomputeDirtyFields(ctx); err != nil {
		return nil, nil, err
	}

	if pos < 0 || pos >= len(b.merkleLayers[0]) {
		return nil, nil, fmt.Errorf("field position %d out of bounds (state has %d fields)", pos, len(b.merkleLayers[0]))
	}

	leaf := b.merkleLayers[0][pos]
	proof := trie.ProofFromMerkleLayers(b.merkleLayers, pos)
	return leaf, proof, nil
}

// ProofForFieldElement returns the leaf and proof for an element within a list/vector field
// (e.g., validators[0]), reaching up to the BeaconState root.
func (b *BeaconState) ProofForFieldElement(ctx context.Context, pos int, index uint64) ([]byte, [][]byte, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if err := b.validateFieldPosition(pos); err != nil {
		return nil, nil, err
	}

	// Resolve the field trie before any merkleization.
	// Bail out early if this field is not backed by a native field trie.
	var (
		f         types.FieldIndex
		fieldTrie *fieldtrie.FieldTrie
	)

	for idx := range b.stateFieldLeaves {
		if idx.RealPosition() == pos {
			f = idx
			fieldTrie = b.stateFieldLeaves[f]
			break
		}
	}
	if fieldTrie == nil {
		return nil, nil, errors.Wrapf(state.ErrFieldElementProofUnsupported, "no field trie for field position %d", pos)
	}

	if err := b.initializeMerkleLayers(ctx); err != nil {
		return nil, nil, err
	}
	if err := b.recomputeDirtyFields(ctx); err != nil {
		return nil, nil, err
	}

	// If the field trie is empty, initialize it by calling rootSelector.
	// This happens when the state is first loaded and the field hasn't been modified yet.
	if fieldTrie.Empty() {
		if _, err := b.rootSelector(ctx, f); err != nil {
			return nil, nil, err
		}

		// Re-fetch the field trie after initialization
		fieldTrie = b.stateFieldLeaves[f]
		if fieldTrie.Empty() {
			return nil, nil, fmt.Errorf("field trie is still empty after initialization for field %s", f.String())
		}
	}

	// For packed arrays (e.g., balances), convert element index to chunk index.
	// In SSZ, basic types like uint64 are packed into 32-byte chunks.
	// For example, balances packs 4 uint64 values (4 * 8 = 32 bytes) per chunk.
	chunkIndex := index
	if elemsInChunk, err := f.ElemsInChunk(); err == nil && elemsInChunk > 0 {
		chunkIndex = index / elemsInChunk
	}

	leaf, proof, err := fieldTrie.ProveField(chunkIndex)
	if err != nil {
		return nil, nil, errors.Wrap(err, fmt.Sprintf("failed to prove field element at index %d in field %s", index, f.String()))
	}

	// Append the field's proof (field root -> state root) so it reaches the state root.
	combinedProof := make([][]byte, 0, len(proof)+len(b.merkleLayers))
	for _, p := range proof {
		combinedProof = append(combinedProof, p[:])
	}
	combinedProof = append(combinedProof, trie.ProofFromMerkleLayers(b.merkleLayers, pos)...)

	return leaf[:], combinedProof, nil
}

func (b *BeaconState) validateFieldPosition(pos int) error {
	errFunc := func(ver int) error {
		return fmt.Errorf("field position %d is out of bounds (not supported) for version %s", pos, version.String(ver))
	}

	switch b.version {
	case version.Phase0:
		if pos > params.BeaconConfig().BeaconStateFieldCount-1 {
			return errFunc(version.Phase0)
		}
	case version.Altair:
		if pos > params.BeaconConfig().BeaconStateAltairFieldCount-1 {
			return errFunc(version.Altair)
		}
	case version.Bellatrix:
		if pos > params.BeaconConfig().BeaconStateBellatrixFieldCount-1 {
			return errFunc(version.Bellatrix)
		}
	case version.Capella:
		if pos > params.BeaconConfig().BeaconStateCapellaFieldCount-1 {
			return errFunc(version.Capella)
		}
	case version.Deneb:
		if pos > params.BeaconConfig().BeaconStateDenebFieldCount-1 {
			return errFunc(version.Deneb)
		}
	case version.Electra:
		if pos > params.BeaconConfig().BeaconStateElectraFieldCount-1 {
			return errFunc(version.Electra)
		}
	case version.Fulu:
		if pos > params.BeaconConfig().BeaconStateFuluFieldCount-1 {
			return errFunc(version.Fulu)
		}
	case version.Gloas:
		if pos > params.BeaconConfig().BeaconStateGloasFieldCount-1 {
			return errFunc(version.Gloas)
		}
	default:
		return fmt.Errorf("unsupported version %s for field position validation", version.String(b.version))
	}

	return nil
}
