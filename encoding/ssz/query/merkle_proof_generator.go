package query

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz/query/proof"
)

// GenerateMerkleProof generates a merkle proof for a given SSZ object and generalized index.
// It works with any SSZ type (BeaconState, BeaconBlock, Attestation, etc.) by using
// dynamic reflection-based type analysis guided by pre-computed SszInfo.
//
// Parameters:
//   - sszObject: Any SSZ-serializable object (BeaconState, BeaconBlock, etc.)
//   - generalizedIndex: The index in the merkle tree to generate proof for
//   - info: Pre-analyzed SSZ type information (use AnalyzeObject if not available)
//
// The approach:
// 1. Create a Wrapper that implements HashWalker to build the merkle tree
// 2. Call HashTreeRootWith to walk the object structure and build the tree
// 3. Extract the proof from the resulting tree at the specified generalized index
// 4. Convert to protobuf format
func GenerateMerkleProof(sszObject any, generalizedIndex uint64, info *SszInfo) (*proof.Proof, error) {
	w := &proof.Wrapper{}

	if err := HashTreeRootWith(sszObject, info, w); err != nil {
		return nil, err
	}

	// Get the root node of the tree
	rootNode := w.Node()
	if rootNode == nil {
		return nil, errors.New("failed to build merkle tree: root node is nil")
	}

	// Generate the proof for the given generalized index
	merkleProof, err := rootNode.Prove(int(generalizedIndex))
	if err != nil {
		return nil, err
	}

	// Verify the proof against the root hash we built (not the proto-generated one)
	rootHash := rootNode.Hash()
	valid, err := proof.VerifyProof(rootHash, merkleProof)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, errors.New("failed to verify merkle proof")
	}

	// Also verify that our built tree root matches the proto-generated hash tree root
	// This is a sanity check to ensure our reflection-based tree building is correct
	if obj, ok := sszObject.(SSZObject); ok {
		protoHTR, err := obj.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("failed to get proto HashTreeRoot: %w", err)
		}
		if !bytes.Equal(rootHash, protoHTR[:]) {
			return nil, fmt.Errorf("root hash mismatch: built tree=%x, proto=%x", rootHash, protoHTR[:])
		}
	}

	return merkleProof, nil
}
