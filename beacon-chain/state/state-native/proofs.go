package state_native

import (
	"context"
	"encoding/binary"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
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

	if err := b.initializeMerkleLayers(ctx); err != nil {
		return nil, err
	}
	if err := b.recomputeDirtyFields(ctx); err != nil {
		return nil, err
	}
	return trie.ProofFromMerkleLayers(b.merkleLayers, f.RealPosition()), nil
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
	}

	return nil
}

// func (b *BeaconState) ProofByGeneralizedIndex(ctx context.Context, generalizedIndices []uint64) ([][]byte, error) {
// 	b.lock.Lock()
// 	defer b.lock.Unlock()

// 	return b.proofByGeneralizedIndex(ctx, generalizedIndices)
// }

// func (b *BeaconState) proofByGeneralizedIndex(ctx context.Context, generalizedIndices []uint64) ([][]byte, error) {
// 	// Validate generalized index
// 	if len(generalizedIndices) == 0 {
// 		return nil, errors.New("no generalized indices provided for proof generation")
// 	}

// 	parentField, err := b.getParentField(generalizedIndices[0])
// 	if err != nil {
// 		return nil, err
// 	}

// 	proofs, err := b.proofByFieldIndex(ctx, parentField)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// At this point, proofs correspond to the parent field.
// 	// If the generalized index does not correspond to a top-level field,
// 	// we need to drill down into the field's Merkle tree representation.
// 	fieldCount, err := b.getFieldCount()
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Compute depths
// 	fieldsDepth := getDepth(fieldCount)
// 	currentDepth := uint64(bits.Len64(generalizedIndices[0]) - 1)
// 	if uint64(fieldsDepth) == currentDepth {
// 		// Top-level leaf (no inner path)
// 		return proofs, nil
// 	}

// 	fieldLayers := b.stateFieldLeaves[parentField].GetFieldLayers()
// 	startingLeafIndex := generalizedIndices[0] - (1 << currentDepth)

// 	innerProof := trie.ProofFromMerkleLayers(convertFieldLayers(fieldLayers), int(startingLeafIndex))
// 	combined := make([][]byte, 0, len(innerProof)+len(proofs))
// 	combined = append(combined, innerProof...)
// 	combined = append(combined, proofs...)

// 	// For other field types, return the top-level proof for now.
// 	return combined, nil
// }

// func (b *BeaconState) getFieldCount() (uint64, error) {
// 	var fieldCount int
// 	switch b.version {
// 	case version.Phase0:
// 		fieldCount = params.BeaconConfig().BeaconStateFieldCount
// 	case version.Altair:
// 		fieldCount = params.BeaconConfig().BeaconStateAltairFieldCount
// 	case version.Bellatrix:
// 		fieldCount = params.BeaconConfig().BeaconStateBellatrixFieldCount
// 	case version.Capella:
// 		fieldCount = params.BeaconConfig().BeaconStateCapellaFieldCount
// 	case version.Deneb:
// 		fieldCount = params.BeaconConfig().BeaconStateDenebFieldCount
// 	case version.Electra:
// 		fieldCount = params.BeaconConfig().BeaconStateElectraFieldCount
// 	case version.Fulu:
// 		fieldCount = params.BeaconConfig().BeaconStateFuluFieldCount
// 	default:
// 		return 0, errNotSupported("getParentField", b.version)
// 	}
// 	return uint64(fieldCount), nil
// }

// func (b *BeaconState) getParentField(gIndex uint64) (types.FieldIndex, error) {
// 	fieldCount, err := b.getFieldCount()
// 	if err != nil {
// 		return -1, err
// 	}

// 	topLevelFieldsDepth := getDepth(fieldCount)
// 	parentField, err := getAncestorFieldAtDepth(gIndex, uint64(topLevelFieldsDepth))
// 	if err != nil {
// 		return -1, err
// 	}

// 	return types.FieldIndex(parentField), nil
// }

// func nextPowerOfTwo(v uint64) uint {
// 	v--
// 	v |= v >> 1
// 	v |= v >> 2
// 	v |= v >> 4
// 	v |= v >> 8
// 	v |= v >> 16
// 	v++
// 	return uint(v)
// }

// // getDepth computes the depth of a Merkle tree given the number of leaves.
// //
// //	      1           --depth = 0  2**0 + 0 = 1
// //	  2       3       --depth = 1  2**1 + 0 = 2, 2**1+1 = 3
// //	4   5   6   7     --depth = 2  2**2 + 0 = 4, 2**2 + 1 = 5...
// func getDepth(d uint64) uint8 {
// 	if d <= 1 {
// 		return 0
// 	}
// 	i := nextPowerOfTwo(d)
// 	leadingZeros := bits.LeadingZeros(i)
// 	return 64 - uint8(leadingZeros) - 1
// }

// func getAncestorFieldAtDepth(generalizedIndex uint64, depth uint64) (uint64, error) {
// 	if generalizedIndex == 0 {
// 		return 0, errors.New("generalized index cannot be zero")
// 	}

// 	currentDepth := uint64(bits.Len64(generalizedIndex) - 1)

// 	if depth > currentDepth {
// 		return 0, errors.New("depth exceeds current depth")
// 	}

// 	shift := currentDepth - depth
// 	ancestorIndex := generalizedIndex >> shift
// 	ancestorField := ancestorIndex - 1<<depth
// 	return ancestorField, nil
// }

// func convertFieldLayers(layers [][]*[32]byte) [][][]byte {
// 	converted := make([][][]byte, len(layers))
// 	for i, layer := range layers {
// 		converted[i] = make([][]byte, len(layer))
// 		for j, node := range layer {
// 			if node == nil {
// 				converted[i][j] = make([]byte, 32)
// 				continue
// 			}
// 			leaf := make([]byte, 32)
// 			copy(leaf, node[:])
// 			converted[i][j] = leaf
// 		}
// 	}
// 	return converted
// }
