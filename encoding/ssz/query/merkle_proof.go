package query

import (
	"fmt"
	"reflect"

	fastssz "github.com/prysmaticlabs/fastssz"
)

// Prove is the entrypoint to generate an SSZ Merkle proof for the given generalized index.
// Parameters:
// - gindex: the generalized index of the node to prove inclusion for.
// Returns:
// - fastssz.Proof: the Merkle proof containing the leaf, index, and sibling hashes.
// - [32]byte: the Merkle root of the entire SSZ object.
// - error: any error encountered during proof generation.
func (info *SszInfo) Prove(gindex uint64) (*fastssz.Proof, error) {
	if info == nil {
		return nil, fmt.Errorf("nil SszInfo")
	}

	collector := NewProofCollector()
	collector.RegisterRequiredSiblings(gindex)

	// info.source is guaranteed to be valid and dereferenced by AnalyzeObject
	v := reflect.ValueOf(info.source).Elem()

	if _, err := collector.merkleize(info, v, 1); err != nil {
		return nil, err
	}

	return collector.toProof(), nil
}
