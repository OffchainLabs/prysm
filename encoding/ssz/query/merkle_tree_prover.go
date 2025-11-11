package query

import (
	"errors"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz/query/proof"
)

// GetTree + prove:
// GenerateMerkleProof generates a merkle proof for a given SSZ object and generalized index.
// It works with any SSZ type (BeaconState, BeaconBlock, Attestation, etc.) by using
// dynamic reflection-based type analysis guided by pre-computed SszInfo.
//
// Parameters:
//   - generalizedIndex: The index in the merkle tree to generate proof for
//   - info: Pre-analyzed SSZ type information (use AnalyzeObject if not available)
//
// The approach:
// 1. Create a Wrapper that implements HashWalker to build the merkle tree
// 2. Call HashTreeRootWith to walk the object structure and build the tree
// 3. Extract the proof from the resulting tree at the specified generalized index
func GenerateMerkleProof(generalizedIndex uint64, info *SszInfo) (*proof.Proof, error) {
	// GetTree step:
	w := &proof.Wrapper{}

	if err := HashTreeRootWith(info, w); err != nil {
		return nil, err
	}

	// Prove step:
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

	return merkleProof, nil
}
