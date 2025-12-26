package execproof

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestInsertExecutionProof(t *testing.T) {
	t.Run("empty pool", func(t *testing.T) {
		pool := NewPool()
		proof := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		pool.InsertExecutionProof(proof)
		require.Equal(t, 1, pool.pending.Len())
		require.Equal(t, 1, len(pool.m))

		key := ProofKey{Slot: proof.Slot, ProofId: proof.ProofId}
		n, ok := pool.m[key]
		require.Equal(t, true, ok)
		v, err := n.Value()
		require.NoError(t, err)
		assert.DeepEqual(t, proof, v)
	})

	t.Run("item in pool", func(t *testing.T) {
		pool := NewPool()
		first := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		second := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(101),
			ProofId:   primitives.ExecutionProofId(2),
			BlockRoot: make([]byte, 32),
		}
		pool.InsertExecutionProof(first)
		pool.InsertExecutionProof(second)
		require.Equal(t, 2, pool.pending.Len())
		require.Equal(t, 2, len(pool.m))

		key1 := ProofKey{Slot: first.Slot, ProofId: first.ProofId}
		n, ok := pool.m[key1]
		require.Equal(t, true, ok)
		v, err := n.Value()
		require.NoError(t, err)
		assert.DeepEqual(t, first, v)

		key2 := ProofKey{Slot: second.Slot, ProofId: second.ProofId}
		n, ok = pool.m[key2]
		require.Equal(t, true, ok)
		v, err = n.Value()
		require.NoError(t, err)
		assert.DeepEqual(t, second, v)
	})

	t.Run("duplicate (slot, proofId) already exists", func(t *testing.T) {
		pool := NewPool()
		first := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: []byte("first_block_root_____________"),
		}
		duplicate := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: []byte("second_block_root____________"),
		}
		pool.InsertExecutionProof(first)
		pool.InsertExecutionProof(duplicate)

		// Should still only have 1 entry - the first one
		assert.Equal(t, 1, pool.pending.Len())
		require.Equal(t, 1, len(pool.m))

		key := ProofKey{Slot: first.Slot, ProofId: first.ProofId}
		n, ok := pool.m[key]
		require.Equal(t, true, ok)
		v, err := n.Value()
		require.NoError(t, err)
		// Should keep the first proof, not the duplicate
		assert.DeepEqual(t, first, v)
	})

	t.Run("different proofId same slot", func(t *testing.T) {
		pool := NewPool()
		proof1 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		proof2 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(2),
			BlockRoot: make([]byte, 32),
		}
		pool.InsertExecutionProof(proof1)
		pool.InsertExecutionProof(proof2)

		// Should have both - different proof IDs
		require.Equal(t, 2, pool.pending.Len())
		require.Equal(t, 2, len(pool.m))
	})
}

func TestPendingExecutionProofs(t *testing.T) {
	t.Run("empty pool", func(t *testing.T) {
		pool := NewPool()
		proofs, err := pool.PendingExecutionProofs()
		require.NoError(t, err)
		assert.Equal(t, 0, len(proofs))
	})

	t.Run("non-empty pool", func(t *testing.T) {
		pool := NewPool()
		proof1 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		proof2 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(101),
			ProofId:   primitives.ExecutionProofId(2),
			BlockRoot: make([]byte, 32),
		}
		pool.InsertExecutionProof(proof1)
		pool.InsertExecutionProof(proof2)

		proofs, err := pool.PendingExecutionProofs()
		require.NoError(t, err)
		assert.Equal(t, 2, len(proofs))
		assert.DeepEqual(t, proof1, proofs[0])
		assert.DeepEqual(t, proof2, proofs[1])
	})

	t.Run("multiple proofs maintain order", func(t *testing.T) {
		pool := NewPool()
		var expectedProofs []*ethpb.ExecutionProof

		// Insert 5 proofs in sequence
		for i := range 5 {
			proof := &ethpb.ExecutionProof{
				Slot:      primitives.Slot(100 + i),
				ProofId:   primitives.ExecutionProofId(i),
				BlockRoot: make([]byte, 32),
			}
			expectedProofs = append(expectedProofs, proof)
			pool.InsertExecutionProof(proof)
		}

		proofs, err := pool.PendingExecutionProofs()
		require.NoError(t, err)
		assert.Equal(t, 5, len(proofs))

		// Verify order is maintained (FIFO)
		for i := range 5 {
			assert.DeepEqual(t, expectedProofs[i], proofs[i])
		}
	})
}

func TestGetProofsForBlock(t *testing.T) {
	blockRoot1 := [32]byte{0x01}
	blockRoot2 := [32]byte{0x02}

	t.Run("empty pool", func(t *testing.T) {
		pool := NewPool()
		proofs := pool.GetProofsForBlock(blockRoot1)
		assert.Equal(t, 0, len(proofs))
	})

	t.Run("single proof for block", func(t *testing.T) {
		pool := NewPool()
		proof := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: blockRoot1[:],
		}
		pool.InsertExecutionProof(proof)

		proofs := pool.GetProofsForBlock(blockRoot1)
		require.Equal(t, 1, len(proofs))
		assert.DeepEqual(t, proof, proofs[0])
	})

	t.Run("multiple proofs for same block", func(t *testing.T) {
		pool := NewPool()
		proof1 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: blockRoot1[:],
		}
		proof2 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(2),
			BlockRoot: blockRoot1[:],
		}
		pool.InsertExecutionProof(proof1)
		pool.InsertExecutionProof(proof2)

		proofs := pool.GetProofsForBlock(blockRoot1)
		require.Equal(t, 2, len(proofs))
		assert.DeepEqual(t, proof1, proofs[0])
		assert.DeepEqual(t, proof2, proofs[1])
	})

	t.Run("filter by block root", func(t *testing.T) {
		pool := NewPool()
		// Proofs for blockRoot1
		proof1 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: blockRoot1[:],
		}
		proof2 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(2),
			BlockRoot: blockRoot1[:],
		}
		// Proofs for blockRoot2
		proof3 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(101),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: blockRoot2[:],
		}

		pool.InsertExecutionProof(proof1)
		pool.InsertExecutionProof(proof2)
		pool.InsertExecutionProof(proof3)

		// Get proofs for blockRoot1
		proofs1 := pool.GetProofsForBlock(blockRoot1)
		require.Equal(t, 2, len(proofs1))
		assert.DeepEqual(t, proof1, proofs1[0])
		assert.DeepEqual(t, proof2, proofs1[1])

		// Get proofs for blockRoot2
		proofs2 := pool.GetProofsForBlock(blockRoot2)
		require.Equal(t, 1, len(proofs2))
		assert.DeepEqual(t, proof3, proofs2[0])
	})

	t.Run("block not in pool", func(t *testing.T) {
		pool := NewPool()
		proof := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: blockRoot1[:],
		}
		pool.InsertExecutionProof(proof)

		// Query for different block root
		proofs := pool.GetProofsForBlock(blockRoot2)
		assert.Equal(t, 0, len(proofs))
	})
}

func TestPruneFinalizedProofs(t *testing.T) {
	t.Run("empty pool", func(t *testing.T) {
		pool := NewPool()
		pruned := pool.PruneFinalizedProofs(primitives.Slot(100))
		assert.Equal(t, uint64(0), pruned)
		assert.Equal(t, 0, pool.pending.Len())
	})

	t.Run("prune none - all proofs newer than finalized", func(t *testing.T) {
		pool := NewPool()
		proof1 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		proof2 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(101),
			ProofId:   primitives.ExecutionProofId(2),
			BlockRoot: make([]byte, 32),
		}
		pool.InsertExecutionProof(proof1)
		pool.InsertExecutionProof(proof2)

		// Finalize at slot 99 - nothing should be pruned
		pruned := pool.PruneFinalizedProofs(primitives.Slot(99))
		assert.Equal(t, uint64(0), pruned)
		assert.Equal(t, 2, pool.pending.Len())
	})

	t.Run("prune all - all proofs older than finalized", func(t *testing.T) {
		pool := NewPool()
		proof1 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		proof2 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(101),
			ProofId:   primitives.ExecutionProofId(2),
			BlockRoot: make([]byte, 32),
		}
		pool.InsertExecutionProof(proof1)
		pool.InsertExecutionProof(proof2)

		// Finalize at slot 200 - all should be pruned
		pruned := pool.PruneFinalizedProofs(primitives.Slot(200))
		assert.Equal(t, uint64(2), pruned)
		assert.Equal(t, 0, pool.pending.Len())
		assert.Equal(t, 0, len(pool.m))
	})

	t.Run("prune some - mixed old and new proofs", func(t *testing.T) {
		pool := NewPool()
		oldProof1 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(95),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		oldProof2 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(98),
			ProofId:   primitives.ExecutionProofId(2),
			BlockRoot: make([]byte, 32),
		}
		newProof1 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(3),
			BlockRoot: make([]byte, 32),
		}
		newProof2 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(105),
			ProofId:   primitives.ExecutionProofId(4),
			BlockRoot: make([]byte, 32),
		}

		pool.InsertExecutionProof(oldProof1)
		pool.InsertExecutionProof(oldProof2)
		pool.InsertExecutionProof(newProof1)
		pool.InsertExecutionProof(newProof2)

		// Finalize at slot 100 - should prune slots < 100
		pruned := pool.PruneFinalizedProofs(primitives.Slot(100))
		assert.Equal(t, uint64(2), pruned)
		assert.Equal(t, 2, pool.pending.Len())

		// Verify the correct proofs remain
		proofs, err := pool.PendingExecutionProofs()
		require.NoError(t, err)
		assert.Equal(t, 2, len(proofs))
		assert.DeepEqual(t, newProof1, proofs[0])
		assert.DeepEqual(t, newProof2, proofs[1])
	})

	t.Run("prune at boundary - slot equals finalized", func(t *testing.T) {
		pool := NewPool()
		boundaryProof := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		newerProof := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(101),
			ProofId:   primitives.ExecutionProofId(2),
			BlockRoot: make([]byte, 32),
		}
		pool.InsertExecutionProof(boundaryProof)
		pool.InsertExecutionProof(newerProof)

		// Finalize at slot 100 - proof at slot 100 should NOT be pruned (only < finalized)
		pruned := pool.PruneFinalizedProofs(primitives.Slot(100))
		assert.Equal(t, uint64(0), pruned)
		assert.Equal(t, 2, pool.pending.Len())
	})

	t.Run("multiple prune calls", func(t *testing.T) {
		pool := NewPool()
		for i := range 10 {
			proof := &ethpb.ExecutionProof{
				Slot:      primitives.Slot(100 + i),
				ProofId:   primitives.ExecutionProofId(i),
				BlockRoot: make([]byte, 32),
			}
			pool.InsertExecutionProof(proof)
		}

		// First prune at slot 105
		pruned1 := pool.PruneFinalizedProofs(primitives.Slot(105))
		assert.Equal(t, uint64(5), pruned1) // Slots 100-104
		assert.Equal(t, 5, pool.pending.Len())

		// Second prune at slot 108
		pruned2 := pool.PruneFinalizedProofs(primitives.Slot(108))
		assert.Equal(t, uint64(3), pruned2) // Slots 105-107
		assert.Equal(t, 2, pool.pending.Len())

		// Verify remaining proofs
		proofs, err := pool.PendingExecutionProofs()
		require.NoError(t, err)
		assert.Equal(t, primitives.Slot(108), proofs[0].Slot)
		assert.Equal(t, primitives.Slot(109), proofs[1].Slot)
	})
}

func TestProofExists(t *testing.T) {
	t.Run("empty pool", func(t *testing.T) {
		pool := NewPool()
		exists := pool.ProofExists(primitives.Slot(100), primitives.ExecutionProofId(1))
		assert.Equal(t, false, exists)
	})

	t.Run("proof exists", func(t *testing.T) {
		pool := NewPool()
		proof := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		pool.InsertExecutionProof(proof)

		exists := pool.ProofExists(primitives.Slot(100), primitives.ExecutionProofId(1))
		assert.Equal(t, true, exists)
	})

	t.Run("proof does not exist - different slot", func(t *testing.T) {
		pool := NewPool()
		proof := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		pool.InsertExecutionProof(proof)

		exists := pool.ProofExists(primitives.Slot(101), primitives.ExecutionProofId(1))
		assert.Equal(t, false, exists)
	})

	t.Run("proof does not exist - different proofId", func(t *testing.T) {
		pool := NewPool()
		proof := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		pool.InsertExecutionProof(proof)

		exists := pool.ProofExists(primitives.Slot(100), primitives.ExecutionProofId(2))
		assert.Equal(t, false, exists)
	})

	t.Run("multiple proofs - check each", func(t *testing.T) {
		pool := NewPool()
		proof1 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		proof2 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(100),
			ProofId:   primitives.ExecutionProofId(2),
			BlockRoot: make([]byte, 32),
		}
		proof3 := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(101),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}

		pool.InsertExecutionProof(proof1)
		pool.InsertExecutionProof(proof2)
		pool.InsertExecutionProof(proof3)

		assert.Equal(t, true, pool.ProofExists(primitives.Slot(100), primitives.ExecutionProofId(1)))
		assert.Equal(t, true, pool.ProofExists(primitives.Slot(100), primitives.ExecutionProofId(2)))
		assert.Equal(t, true, pool.ProofExists(primitives.Slot(101), primitives.ExecutionProofId(1)))
		assert.Equal(t, false, pool.ProofExists(primitives.Slot(101), primitives.ExecutionProofId(2)))
	})

	t.Run("proof exists then pruned", func(t *testing.T) {
		pool := NewPool()
		proof := &ethpb.ExecutionProof{
			Slot:      primitives.Slot(99),
			ProofId:   primitives.ExecutionProofId(1),
			BlockRoot: make([]byte, 32),
		}
		pool.InsertExecutionProof(proof)

		// Verify it exists
		exists := pool.ProofExists(primitives.Slot(99), primitives.ExecutionProofId(1))
		assert.Equal(t, true, exists)

		// Prune it
		pool.PruneFinalizedProofs(primitives.Slot(100))

		// Should no longer exist
		exists = pool.ProofExists(primitives.Slot(99), primitives.ExecutionProofId(1))
		assert.Equal(t, false, exists)
	})
}
