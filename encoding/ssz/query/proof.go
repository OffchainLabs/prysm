package query

import "github.com/OffchainLabs/prysm/v7/container/trie"

// Proof represents a merkle proof against a general index.
type Proof struct {
	Index  uint64
	Leaf   []byte
	Hashes [][]byte
}

// Verify unrolls the proof and verifies it against the given root.
func (p *Proof) Verify(root []byte) bool {
	if p == nil {
		return false
	}

	return trie.VerifyMerkleProof(root, p.Leaf, p.Index, p.Hashes)
}
