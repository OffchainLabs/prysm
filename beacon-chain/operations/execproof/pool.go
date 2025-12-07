package execproof

import (
	"maps"
	"sync"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	doublylinkedlist "github.com/OffchainLabs/prysm/v7/container/doubly-linked-list"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// We recycle the BLS changes pool to avoid the backing map growing without
// bound. The cycling operation is expensive because it copies all elements, so
// we only do it when the map is smaller than this upper bound.
const execProofPoolThreshold = 2000

var (
	blsToExecMessageInPoolTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "exec_proof_message_pool_total",
		Help: "The number of saved bls to exec messages in the operation pool.",
	})
)

// PoolManager maintains pending and seen Execution-proofs objects.
// This pool is used by proposers to insert Execution-proofs objects into new blocks.
type PoolManager interface {
	PendingExecutionProof() ([]*ethpb.ExecutionProof, error)
	ExecutionForInclusion(beaconState state.ReadOnlyBeaconState) ([]*ethpb.ExecutionProof, error)
	InsertExecutionProof(executionProof *ethpb.ExecutionProof)
	MarkIncluded(executionProof *ethpb.ExecutionProof)
	ValidatorExists(idx primitives.ValidatorIndex) bool
}

// Pool is a concrete implementation of PoolManager.
type Pool struct {
	lock    sync.RWMutex
	pending doublylinkedlist.List[*ethpb.ExecutionProof]
	m       map[primitives.ValidatorIndex]*doublylinkedlist.Node[*ethpb.ExecutionProof]
}

// NewPool returns an initialized pool.
func NewPool() *Pool {
	return &Pool{
		pending: doublylinkedlist.List[*ethpb.ExecutionProof]{},
		m:       make(map[primitives.ValidatorIndex]*doublylinkedlist.Node[*ethpb.ExecutionProof]),
	}
}

// Copies the internal map and returns a new one.
func (p *Pool) cycleMap() {
	newMap := make(map[primitives.ValidatorIndex]*doublylinkedlist.Node[*ethpb.ExecutionProof])
	maps.Copy(newMap, p.m)
	p.m = newMap
}

// PendingExecProofChanges returns all objects from the pool.
func (p *Pool) PendingExecProofChanges() ([]*ethpb.ExecutionProof, error) {
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


// InsertExecutionProof inserts an object into the pool.
func (p *Pool) InsertExecutionProof(change *ethpb.ExecutionProof) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.pending.Append(doublylinkedlist.NewNode(change))

	blsToExecMessageInPoolTotal.Inc()
}


// ValidatorExists checks if the bls to execution change object exists
// for that particular validator.
func (p *Pool) ValidatorExists(idx primitives.ValidatorIndex) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node := p.m[idx]

	return node != nil
}

// numPending returns the number of pending bls to execution changes in the pool
func (p *Pool) numPending() int {
	return p.pending.Len()
}
