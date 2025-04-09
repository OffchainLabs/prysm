//go:build !fuzz

package cache

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

var (
	// ProposerIndicesCacheMiss tracks the number of proposerIndices requests that aren't present in the cache.
	ProposerIndicesCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "proposer_indices_cache_miss",
		Help: "The number of proposer indices requests that aren't present in the cache.",
	})
	// ProposerIndicesCacheHit tracks the number of proposerIndices requests that are in the cache.
	ProposerIndicesCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "proposer_indices_cache_hit",
		Help: "The number of proposer indices requests that are present in the cache.",
	})
)

// ProposerIndicesCache keeps track of the proposer indices in the next two
// epochs. It is keyed by the state root of the last epoch before. That is, for
// blocks during epoch 2, for example slot 65, it will be keyed by the state
// root of slot 63 (last slot in epoch 1).
// The cache keeps two sets of indices computed, the "safe" set is computed
// right before the epoch transition into the current epoch. For example for
// epoch 2 we will compute this list after importing block 63. The "unsafe"
// version is computed an epoch in advance, for example for epoch 3, it will be
// computed after importing block 63.
//
// The cache also keeps a map from checkpoints to state roots so that one is
// able to access the proposer indices list from a checkpoint instead. The
// checkpoint is the checkpoint for the epoch previous to the requested
// proposer indices. That is, for a slot in epoch 2 (eg. 65), the checkpoint
// root would be for slot 32 if present.
type ProposerIndicesCache struct {
	sync.Mutex
	indices map[primitives.Epoch]map[[32]byte][fieldparams.SlotsPerEpoch]primitives.ValidatorIndex
	rootMap map[forkchoicetypes.Checkpoint][32]byte // A map from checkpoint root to state root
}

// NewProposerIndicesCache returns a newly created cache
func NewProposerIndicesCache() *ProposerIndicesCache {
	return &ProposerIndicesCache{
		indices: make(map[primitives.Epoch]map[[32]byte][fieldparams.SlotsPerEpoch]primitives.ValidatorIndex),
		rootMap: make(map[forkchoicetypes.Checkpoint][32]byte),
	}
}

// ProposerIndices returns the proposer indices (safe) for the given root
func (p *ProposerIndicesCache) ProposerIndices(epoch primitives.Epoch, root [32]byte) ([fieldparams.SlotsPerEpoch]primitives.ValidatorIndex, bool) {
	p.Lock()
	defer p.Unlock()
	inner, ok := p.indices[epoch]
	if !ok {
		ProposerIndicesCacheMiss.Inc()
		return [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex{}, false
	}
	indices, exists := inner[root]
	if exists {
		ProposerIndicesCacheHit.Inc()
	} else {
		ProposerIndicesCacheMiss.Inc()
	}
	return indices, exists
}

// Prune resets the ProposerIndicesCache to its initial state
func (p *ProposerIndicesCache) Prune(epoch primitives.Epoch) {
	p.Lock()
	defer p.Unlock()
	for key := range p.indices {
		if key < epoch {
			delete(p.indices, key)
		}
	}
	for key := range p.rootMap {
		if key.Epoch+1 < epoch {
			delete(p.rootMap, key)
		}
	}
}

// Set sets the proposer indices for the given root as key
func (p *ProposerIndicesCache) Set(epoch primitives.Epoch, root [32]byte, indices [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex) {
	p.Lock()
	defer p.Unlock()

	inner, ok := p.indices[epoch]
	if !ok {
		inner = make(map[[32]byte][fieldparams.SlotsPerEpoch]primitives.ValidatorIndex)
		p.indices[epoch] = inner
	}
	inner[root] = indices
}

// SetCheckpoint updates the map from checkpoints to state roots
func (p *ProposerIndicesCache) SetCheckpoint(c forkchoicetypes.Checkpoint, root [32]byte) {
	p.Lock()
	defer p.Unlock()
	p.rootMap[c] = root
}

// IndicesFromCheckpoint returns the proposer indices from a checkpoint rather than the state root
func (p *ProposerIndicesCache) IndicesFromCheckpoint(c forkchoicetypes.Checkpoint) ([fieldparams.SlotsPerEpoch]primitives.ValidatorIndex, bool) {
	p.Lock()
	emptyIndices := [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex{}
	root, ok := p.rootMap[c]
	p.Unlock()
	if !ok {
		ProposerIndicesCacheMiss.Inc()
		return emptyIndices, ok
	}
	return p.ProposerIndices(c.Epoch+1, root)
}
