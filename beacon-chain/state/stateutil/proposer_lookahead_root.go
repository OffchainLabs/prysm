package stateutil

import (
	"encoding/binary"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz"
)

// ProposerLookaheadRoot computes the hash tree root of the proposer lookahead
func ProposerLookaheadRoot(lookahead []primitives.ValidatorIndex) ([32]byte, error) {
	chunks := make([][32]byte, (len(lookahead)*8+31)/32)
	for i, idx := range lookahead {
		j := i / 4
		binary.LittleEndian.PutUint64(chunks[j][(i%4)*8:], uint64(idx))
	}
	return ssz.MerkleizeVector(chunks, uint64(len(chunks))), nil
}
