package stateutil

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
)

// ExecutionPayloadAvailabilityRoot computes the merkle root of an execution payload availability bitvector.
func ExecutionPayloadAvailabilityRoot(bitvector []byte) ([32]byte, error) {
	chunkCount := (len(bitvector) + 31) / 32
	chunks := make([][32]byte, chunkCount)

	for i := range chunks {
		start := i * 32
		end := min(start+32, len(bitvector))
		copy(chunks[i][:], bitvector[start:end])
	}

	root, err := ssz.BitwiseMerkleize(chunks, uint64(len(chunks)), uint64(len(chunks)))
	if err != nil {
		return [32]byte{}, fmt.Errorf("could not merkleize execution payload availability: %w", err)
	}
	return root, nil
}
