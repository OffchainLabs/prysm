package query

import (
	"errors"
	"reflect"

	fastssz "github.com/prysmaticlabs/fastssz"
)

// Prove is the entrypoint to generate an SSZ Merkle proof for the given generalized index.
// Parameters:
// - gindex: the generalized index of the node to prove inclusion for.
// Returns:
// - fastssz.Proof: the Merkle proof containing the leaf, index, and sibling hashes.
// - error: any error encountered during proof generation.
func (info *SszInfo) Prove(gindex uint64) (*fastssz.Proof, error) {
	if info == nil {
		return nil, errors.New("nil SszInfo")
	}
	if info.source == nil {
		return nil, errors.New("SszInfo.source is nil")
	}

	v := reflect.ValueOf(info.source)
	if !v.IsValid() {
		return nil, errors.New("proof value is invalid")
	}

	v = dereferencePointer(v)

	collector := newProofCollector()
	collector.addTarget(gindex)

	// Start the merkleization and proof collection process.
	// In SSZ generalized indices, the root is always at index 1.
	if _, err := collector.merkleize(info, v, 1); err != nil {
		return nil, err
	}

	return collector.toProof()
}
