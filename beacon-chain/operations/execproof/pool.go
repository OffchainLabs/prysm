package execproof

import (
	"maps"
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	doublylinkedlist "github.com/OffchainLabs/prysm/v7/container/doubly-linked-list"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// We recycle the execution proof pool to avoid the backing map growing without
	// bound. The cycling operation is expensive because it copies all elements, so
	// we only do it when the map is smaller than this upper bound.
	execProofPoolThreshold = 2000
)

var (
	execProofInPoolTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "exec_proof_pool_total",
		Help: "The number of execution proofs in the operation pool.",
	})
)

var _ PoolManager = (*Pool)(nil)

// PoolManager maintains execution proofs received via gossip.
// These proofs are used for data availability checks when importing blocks.
// Lightweight verifier nodes need a minimum number of proofs from different zkVM types
// to verify block execution correctness.
type PoolManager interface {
	// InsertExecutionProof adds a proof received from gossip to the pool
	InsertExecutionProof(executionProof *ethpb.ExecutionProof)

	// GetProofsForBlock returns all proofs for a specific block root
	GetProofsForBlock(blockRoot [32]byte) []*ethpb.ExecutionProof

	// GetProofCountForBlock returns the count of unique proof types for a block
	GetProofCountForBlock(blockRoot [32]byte) uint64

	// GetProofTypesForBlock returns the unique proof types for a block
	GetProofTypesForBlock(blockRoot [32]byte) map[primitives.ExecutionProofId]struct{}

	// ProofExists checks if a proof exists for the given slot and proof ID
	ProofExists(slot primitives.Slot, proofId primitives.ExecutionProofId) bool

	// PruneFinalizedProofs removes proofs older than the finalized slot.
	PruneFinalizedProofs(finalizedSlot primitives.Slot) uint64

	// PendingExecutionProofs returns all proofs from the pool (for debugging/monitoring)
	PendingExecutionProofs() ([]*ethpb.ExecutionProof, error)
}

// Pool is a concrete implementation of PoolManager.
type Pool struct {
	lock    sync.RWMutex
	pending doublylinkedlist.List[*ethpb.ExecutionProof]
	m       map[ProofKey]*doublylinkedlist.Node[*ethpb.ExecutionProof]
}

// NewPool returns an initialized pool.
func NewPool() *Pool {
	return &Pool{
		pending: doublylinkedlist.List[*ethpb.ExecutionProof]{},
		m:       make(map[ProofKey]*doublylinkedlist.Node[*ethpb.ExecutionProof]),
	}
}

// makeKey creates a ProofKey from an execution proof.
func makeKey(proof *ethpb.ExecutionProof) ProofKey {
	return ProofKey{
		Slot:    proof.Slot,
		ProofId: proof.ProofId,
	}
}

// Copies the internal map and returns a new one.
func (p *Pool) cycleMap() {
	newMap := make(map[ProofKey]*doublylinkedlist.Node[*ethpb.ExecutionProof])
	maps.Copy(newMap, p.m)
	p.m = newMap
}

// PendingExecutionProofs returns all proofs from the pool.
func (p *Pool) PendingExecutionProofs() ([]*ethpb.ExecutionProof, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	result := make([]*ethpb.ExecutionProof, p.pending.Len())
	node := p.pending.First()
	var err error
	for i := 0; node != nil; i++ {
		result[i], err = node.Value()
		if err != nil {
			return nil, err
		}
		node, err = node.Next()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// GetProofsForBlock returns all proofs for a specific block root.
// This is used during data availability checks to verify we have enough proofs
// from different zkVM types to trust the block's execution.
func (p *Pool) GetProofsForBlock(blockRoot [32]byte) []*ethpb.ExecutionProof {
	p.lock.RLock()
	defer p.lock.RUnlock()

	var result []*ethpb.ExecutionProof
	node := p.pending.First()

	for node != nil {
		proof, err := node.Value()
		if err != nil {
			break
		}

		var proofBlockRoot [32]byte
		copy(proofBlockRoot[:], proof.BlockRoot)
		if proofBlockRoot == blockRoot {
			result = append(result, proof)
		}

		node, _ = node.Next()
	}

	return result
}

// GetProofCountForBlock returns the count of unique proof types (ProofId) for a block.
// This is used to check if we have enough diverse proofs for data availability.
func (p *Pool) GetProofCountForBlock(blockRoot [32]byte) uint64 {
	p.lock.RLock()
	defer p.lock.RUnlock()

	set := p.getProofSet(blockRoot)
	return uint64(len(set))
}

// GetProofTypesForBlock returns the count of unique proof types (ProofId) for a block.
func (p *Pool) GetProofTypesForBlock(blockRoot [32]byte) map[primitives.ExecutionProofId]struct{} {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.getProofSet(blockRoot)
}

// getProofSet is an internal helper to retrieve the set of unique proof IDs for a block.
// Should be called with the lock held.
func (p *Pool) getProofSet(blockRoot [32]byte) map[primitives.ExecutionProofId]struct{} {
	// Track unique proof IDs
	set := make(map[primitives.ExecutionProofId]struct{})
	node := p.pending.First()

	for node != nil {
		proof, err := node.Value()
		if err != nil {
			break
		}

		var proofBlockRoot [32]byte
		copy(proofBlockRoot[:], proof.BlockRoot)
		if proofBlockRoot == blockRoot {
			set[proof.ProofId] = struct{}{}
		}

		node, _ = node.Next()
	}

	return set
}

// InsertExecutionProof inserts a proof into the pool.
// Deduplicates by (Slot, ProofId) - only keeps the first proof for each combination.
func (p *Pool) InsertExecutionProof(proof *ethpb.ExecutionProof) {
	p.lock.Lock()
	defer p.lock.Unlock()

	key := makeKey(proof)

	// Check if proof already exists
	if existingNode := p.m[key]; existingNode != nil {
		// Already have a proof for this (Slot, ProofId) - skip insertion
		return
	}

	// Insert new proof
	node := doublylinkedlist.NewNode(proof)
	p.pending.Append(node)
	p.m[key] = p.pending.Last()

	execProofInPoolTotal.Inc()
}

// PruneFinalizedProofs removes proofs older than the finalized slot.
func (p *Pool) PruneFinalizedProofs(finalizedSlot primitives.Slot) uint64 {
	p.lock.Lock()
	defer p.lock.Unlock()

	pruned := uint64(0)
	node := p.pending.First()

	for node != nil {
		proof, err := node.Value()
		if err != nil {
			break
		}

		next, _ := node.Next()

		// Remove proofs older than finalizedSlot
		if proof.Slot < finalizedSlot {
			key := makeKey(proof)
			delete(p.m, key)
			p.pending.Remove(node)
			execProofInPoolTotal.Dec()
			pruned++
		}

		node = next
	}

	// Cycle map after pruning if we're below threshold
	if pruned > 0 && p.numPending() <= execProofPoolThreshold {
		p.cycleMap()
	}

	return pruned
}

// ProofExists checks if a proof exists for the given slot and proof ID.
func (p *Pool) ProofExists(slot primitives.Slot, proofId primitives.ExecutionProofId) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	key := ProofKey{Slot: slot, ProofId: proofId}
	return p.m[key] != nil
}

// numPending returns the number of pending execution proofs in the pool.
func (p *Pool) numPending() int {
	return p.pending.Len()
}
