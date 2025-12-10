package mock

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	ExecutionProofs []*eth.ExecutionProof
}

// InsertExecutionProof adds a proof to the mock pool.
func (m *PoolMock) InsertExecutionProof(proof *eth.ExecutionProof) {
	m.ExecutionProofs = append(m.ExecutionProofs, proof)
}

// GetProofsForBlock returns all proofs for a specific block root.
func (m *PoolMock) GetProofsForBlock(blockRoot [32]byte) []*eth.ExecutionProof {
	var result []*eth.ExecutionProof
	for _, proof := range m.ExecutionProofs {
		var proofBlockRoot [32]byte
		copy(proofBlockRoot[:], proof.BlockRoot)
		if proofBlockRoot == blockRoot {
			result = append(result, proof)
		}
	}
	return result
}

// GetProofCountForBlock returns the count of unique proof types for a block.
func (m *PoolMock) GetProofCountForBlock(blockRoot [32]byte) uint64 {
	uniqueProofIds := make(map[primitives.ExecutionProofId]bool)
	for _, proof := range m.ExecutionProofs {
		var proofBlockRoot [32]byte
		copy(proofBlockRoot[:], proof.BlockRoot)
		if proofBlockRoot == blockRoot {
			uniqueProofIds[proof.ProofId] = true
		}
	}
	return uint64(len(uniqueProofIds))
}

// ProofExists checks if a proof exists for the given slot and proof ID.
func (m *PoolMock) ProofExists(slot primitives.Slot, proofId primitives.ExecutionProofId) bool {
	for _, proof := range m.ExecutionProofs {
		if proof.Slot == slot && proof.ProofId == proofId {
			return true
		}
	}
	return false
}

// PruneFinalizedProofs removes proofs older than the finalized slot.
func (m *PoolMock) PruneFinalizedProofs(finalizedSlot primitives.Slot) uint64 {
	var kept []*eth.ExecutionProof
	pruned := uint64(0)
	for _, proof := range m.ExecutionProofs {
		if proof.Slot >= finalizedSlot {
			kept = append(kept, proof)
		} else {
			pruned++
		}
	}
	m.ExecutionProofs = kept
	return pruned
}

// PendingExecutionProofs returns all proofs from the pool.
func (m *PoolMock) PendingExecutionProofs() ([]*eth.ExecutionProof, error) {
	return m.ExecutionProofs, nil
}
