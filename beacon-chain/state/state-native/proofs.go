package state_native

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

const (
	finalizedRootIndex = uint64(105) // Precomputed pre-Electra value.
)

type lightClientGeneralizedIndices struct {
	finalizedRoot        uint64
	currentSyncCommittee uint64
	nextSyncCommittee    uint64
}

// FinalizedRootGeneralizedIndex for the beacon state.
func FinalizedRootGeneralizedIndex() uint64 {
	return finalizedRootIndex
}

// FinalizedRootGeneralizedIndexForVersion returns the finalized checkpoint root
// generalized index for the given state version and SSZ mode.
func FinalizedRootGeneralizedIndexForVersion(stateVersion int) (uint64, error) {
	indices, err := lightClientGeneralizedIndicesForVersion(stateVersion)
	if err != nil {
		return 0, err
	}
	return indices.finalizedRoot, nil
}

// CurrentSyncCommitteeGeneralizedIndexForVersion returns the current sync
// committee generalized index for the given state version and SSZ mode.
func CurrentSyncCommitteeGeneralizedIndexForVersion(stateVersion int) (uint64, error) {
	if stateVersion == version.Phase0 {
		return 0, errNotSupported("CurrentSyncCommitteeGeneralizedIndex", stateVersion)
	}
	indices, err := lightClientGeneralizedIndicesForVersion(stateVersion)
	if err != nil {
		return 0, err
	}
	return indices.currentSyncCommittee, nil
}

// NextSyncCommitteeGeneralizedIndexForVersion returns the next sync committee
// generalized index for the given state version and SSZ mode.
func NextSyncCommitteeGeneralizedIndexForVersion(stateVersion int) (uint64, error) {
	if stateVersion == version.Phase0 {
		return 0, errNotSupported("NextSyncCommitteeGeneralizedIndex", stateVersion)
	}
	indices, err := lightClientGeneralizedIndicesForVersion(stateVersion)
	if err != nil {
		return 0, err
	}
	return indices.nextSyncCommittee, nil
}

func lightClientGeneralizedIndicesForVersion(stateVersion int) (lightClientGeneralizedIndices, error) {
	switch stateVersion {
	case version.Phase0, version.Altair, version.Bellatrix, version.Capella, version.Deneb:
		return lightClientGeneralizedIndices{finalizedRoot: 105, currentSyncCommittee: 54, nextSyncCommittee: 55}, nil
	case version.Electra, version.Fulu:
		return lightClientGeneralizedIndices{finalizedRoot: 169, currentSyncCommittee: 86, nextSyncCommittee: 87}, nil
	case version.Gloas:
		if features.ProgressiveSSZEnabled(stateVersion) {
			return lightClientGeneralizedIndices{finalizedRoot: 735, currentSyncCommittee: 2945, nextSyncCommittee: 2946}, nil
		}
		return lightClientGeneralizedIndices{finalizedRoot: 169, currentSyncCommittee: 86, nextSyncCommittee: 87}, nil
	default:
		return lightClientGeneralizedIndices{}, errNotSupported("light client generalized indices", stateVersion)
	}
}

// CurrentSyncCommitteeGeneralizedIndex for the beacon state.
func (b *BeaconState) CurrentSyncCommitteeGeneralizedIndex() (uint64, error) {
	return CurrentSyncCommitteeGeneralizedIndexForVersion(b.version)
}

// NextSyncCommitteeGeneralizedIndex for the beacon state.
func (b *BeaconState) NextSyncCommitteeGeneralizedIndex() (uint64, error) {
	return NextSyncCommitteeGeneralizedIndexForVersion(b.version)
}

// CurrentSyncCommitteeProof from the state's Merkle trie representation.
func (b *BeaconState) CurrentSyncCommitteeProof(ctx context.Context) ([][]byte, error) {
	return b.ProofByFieldIndex(ctx, types.CurrentSyncCommittee)
}

// NextSyncCommitteeProof from the state's Merkle trie representation.
func (b *BeaconState) NextSyncCommitteeProof(ctx context.Context) ([][]byte, error) {
	return b.ProofByFieldIndex(ctx, types.NextSyncCommittee)
}

// FinalizedRootProof crafts a Merkle proof for the finalized root
// contained within the finalized checkpoint of a beacon state.
func (b *BeaconState) FinalizedRootProof(ctx context.Context) ([][]byte, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	branchProof, err := b.proofByFieldIndex(ctx, types.FinalizedCheckpoint)
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

// ProofByFieldIndex constructs proofs for given field index with lock acquisition.
func (b *BeaconState) ProofByFieldIndex(ctx context.Context, f types.FieldIndex) ([][]byte, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	return b.proofByFieldIndex(ctx, f)
}

// proofByFieldIndex constructs proofs for given field index.
// Important: it is assumed that beacon state mutex is locked when calling this method.
func (b *BeaconState) proofByFieldIndex(ctx context.Context, f types.FieldIndex) ([][]byte, error) {
	err := b.validateFieldIndex(f)
	if err != nil {
		return nil, err
	}

	if features.ProgressiveSSZEnabled(b.version) {
		return b.progressiveProofByFieldIndex(ctx, f)
	}

	if err := b.initializeMerkleLayers(ctx); err != nil {
		return nil, err
	}
	if err := b.recomputeDirtyFields(ctx); err != nil {
		return nil, err
	}
	b.progressiveMerkleTree = nil
	return trie.ProofFromMerkleLayers(b.merkleLayers, f.RealPosition()), nil
}

func (b *BeaconState) progressiveProofByFieldIndex(ctx context.Context, f types.FieldIndex) ([][]byte, error) {
	if err := b.initializeProgressiveMerkleTree(ctx); err != nil {
		return nil, err
	}
	if err := b.recomputeProgressiveDirtyFields(ctx); err != nil {
		return nil, err
	}

	schema, ok := ProgressiveStateSchemaForVersion(b.version)
	if !ok {
		return nil, fmt.Errorf("progressive state schema not found for version %v", b.version)
	}
	fieldPos, ok := schema.GetFieldIndex(f)
	if !ok {
		return nil, fmt.Errorf("field index %s not found for version %v", f.String(), b.version)
	}

	branch, err := b.progressiveMerkleTree.Proof(fieldPos)
	if err != nil {
		return nil, err
	}
	activeFieldsRoot, err := ssz.PackActiveFields(schema.ActiveFields())
	if err != nil {
		return nil, err
	}
	branch = append(branch, activeFieldsRoot)

	proof := make([][]byte, len(branch))
	for i := range branch {
		proof[i] = branch[i][:]
	}
	b.merkleLayers = nil
	return proof, nil
}

func (b *BeaconState) validateFieldIndex(f types.FieldIndex) error {
	switch b.version {
	case version.Phase0:
		if f.RealPosition() > params.BeaconConfig().BeaconStateFieldCount-1 {
			return errNotSupported(f.String(), b.version)
		}
	case version.Altair:
		if f.RealPosition() > params.BeaconConfig().BeaconStateAltairFieldCount-1 {
			return errNotSupported(f.String(), b.version)
		}
	case version.Bellatrix:
		if f.RealPosition() > params.BeaconConfig().BeaconStateBellatrixFieldCount-1 {
			return errNotSupported(f.String(), b.version)
		}
	case version.Capella:
		if f.RealPosition() > params.BeaconConfig().BeaconStateCapellaFieldCount-1 {
			return errNotSupported(f.String(), b.version)
		}
	case version.Deneb:
		if f.RealPosition() > params.BeaconConfig().BeaconStateDenebFieldCount-1 {
			return errNotSupported(f.String(), b.version)
		}
	case version.Electra:
		if f.RealPosition() > params.BeaconConfig().BeaconStateElectraFieldCount-1 {
			return errNotSupported(f.String(), b.version)
		}
	case version.Fulu:
		if f.RealPosition() > params.BeaconConfig().BeaconStateFuluFieldCount-1 {
			return errNotSupported(f.String(), b.version)
		}
	case version.Gloas:
		schema, ok := ProgressiveStateSchemaForVersion(version.Gloas)
		if !ok {
			return fmt.Errorf("progressive state schema not found for version %v", b.version)
		}
		_, ok = schema.GetFieldIndex(f)
		if !ok {
			return errNotSupported(f.String(), b.version)
		}
	}

	return nil
}
