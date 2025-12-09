package query

import (
	ssz "github.com/prysmaticlabs/fastssz"
)

// MerkleTree builds and returns the full SSZ Merkle tree for the analyzed object.
// The returned Node can be used to compute the hash tree root via Node.Hash(),
// or to generate inclusion proofs via Node.Prove(gindex).
func (info *SszInfo) MerkleTree() (*ssz.Node, error) {
	return info.toMerkleTree(&ssz.Wrapper{})
}
