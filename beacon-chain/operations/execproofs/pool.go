package execproofs

import (
	"fmt"
	"sync"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ProofKey uniquely identifies an execution proof by block root and proof type.
type ProofKey struct {
	Root    [fieldparams.RootLength]byte
	ProofId primitives.ExecutionProofId
}

// String returns a string representation for logging.
func (k ProofKey) String() string {
	return fmt.Sprintf("root=%#x,proofId=%d", k.Root, k.ProofId)
}

var (
	execProofInPoolTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "exec_proof_pool_total",
		Help: "The number of execution proofs in the operation pool.",
	})
)

var _ PoolManager = (*ExecProofPool)(nil)

// PoolManager maintains execution proofs received via gossip.
// These proofs are used for data availability checks when importing blocks.
// Lightweight verifier nodes need a minimum number of proofs from different zkVM types
// to verify block execution correctness.
type PoolManager interface {
	// Insert inserts a proof into the pool.
	// If a proof with the same block root and proof ID already exists, it is not added again.
	Insert(executionProof *ethpb.ExecutionProof)

	// Get returns a copy of all proofs for a specific block root
	Get(blockRoot [fieldparams.RootLength]byte) []*ethpb.ExecutionProof

	// Ids returns the list of (unique) proof types available for a specific block root
	Ids(blockRoot [fieldparams.RootLength]byte) []primitives.ExecutionProofId

	// Count counts the number of proofs for a specific block root
	Count(blockRoot [fieldparams.RootLength]byte) uint64

	// Exists checks if a proof exists for the given block root and proof ID
	Exists(blockRoot [fieldparams.RootLength]byte, proofId primitives.ExecutionProofId) bool

	// PruneUpTo removes proofs older than the target slot
	PruneUpTo(targetSlot primitives.Slot) int
}

// ExecProofPool is a concrete implementation of type ExecProofPoolManager.
type ExecProofPool struct {
	lock sync.RWMutex
	m    map[[fieldparams.RootLength]byte]map[primitives.ExecutionProofId]*ethpb.ExecutionProof
}

// NewPool returns an initialized pool.
func NewPool() *ExecProofPool {
	return &ExecProofPool{
		m: make(map[[fieldparams.RootLength]byte]map[primitives.ExecutionProofId]*ethpb.ExecutionProof),
	}
}

// Insert inserts a proof into the pool.
// If a proof with the same block root and proof ID already exists, it is not added again.
func (p *ExecProofPool) Insert(proof *ethpb.ExecutionProof) {
	p.lock.Lock()
	defer p.lock.Unlock()

	blockRoot := bytesutil.ToBytes32(proof.BlockRoot)

	// Create the inner map if it doesn't exist
	if p.m[blockRoot] == nil {
		p.m[blockRoot] = make(map[primitives.ExecutionProofId]*ethpb.ExecutionProof)
	}

	// Check if proof already exists
	if _, exists := p.m[blockRoot][proof.ProofId]; exists {
		return
	}

	// Insert new proof
	p.m[blockRoot][proof.ProofId] = proof
	execProofInPoolTotal.Inc()
}

// Get returns a copy of all proofs for a specific block root
func (p *ExecProofPool) Get(blockRoot [fieldparams.RootLength]byte) []*ethpb.ExecutionProof {
	p.lock.RLock()
	defer p.lock.RUnlock()

	proofsByType, exists := p.m[blockRoot]
	if !exists {
		return nil
	}

	result := make([]*ethpb.ExecutionProof, 0, len(proofsByType))
	for _, proof := range proofsByType {
		result = append(result, proof.Copy())
	}
	return result
}

func (p *ExecProofPool) Ids(blockRoot [fieldparams.RootLength]byte) []primitives.ExecutionProofId {
	p.lock.RLock()
	defer p.lock.RUnlock()

	proofById, exists := p.m[blockRoot]
	if !exists {
		return nil
	}

	ids := make([]primitives.ExecutionProofId, 0, len(proofById))
	for id := range proofById {
		ids = append(ids, id)
	}

	return ids
}

// Count counts the number of proofs for a specific block root
func (p *ExecProofPool) Count(blockRoot [fieldparams.RootLength]byte) uint64 {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return uint64(len(p.m[blockRoot]))
}

// Exists checks if a proof exists for the given block root and proof ID
func (p *ExecProofPool) Exists(blockRoot [fieldparams.RootLength]byte, proofId primitives.ExecutionProofId) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	proofsByType, exists := p.m[blockRoot]
	if !exists {
		return false
	}

	_, exists = proofsByType[proofId]
	return exists
}

// PruneUpTo removes proofs older than the given slot
func (p *ExecProofPool) PruneUpTo(targetSlot primitives.Slot) int {
	p.lock.Lock()
	defer p.lock.Unlock()

	pruned := 0
	for blockRoot, proofsByType := range p.m {
		for proofId, proof := range proofsByType {
			if proof.Slot < targetSlot {
				delete(proofsByType, proofId)
				execProofInPoolTotal.Dec()
				pruned++
			}
		}

		// Clean up empty inner maps
		if len(proofsByType) == 0 {
			delete(p.m, blockRoot)
		}
	}

	return pruned
}
