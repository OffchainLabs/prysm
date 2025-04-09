//go:build fuzz

// This file is used in fuzzer builds to bypass proposer indices caches.
package cache

import (
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

// FakeProposerIndicesCache is a struct with 1 queue for looking up proposer indices by root.
type FakeProposerIndicesCache struct {
}

// NewProposerIndicesCache creates a new proposer indices cache for storing/accessing proposer index assignments of an epoch.
func NewProposerIndicesCache() *FakeProposerIndicesCache {
	return &FakeProposerIndicesCache{}
}

// ProposerIndices is a stub.
func (c *FakeProposerIndicesCache) ProposerIndices(_ primitives.Epoch, _ [32]byte) ([fieldparams.SlotsPerEpoch]primitives.ValidatorIndex, bool) {
	return [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex{}, false
}

// UnsafeProposerIndices is a stub.
func (c *FakeProposerIndicesCache) UnsafeProposerIndices(_ primitives.Epoch, _ [32]byte) ([fieldparams.SlotsPerEpoch]primitives.ValidatorIndex, bool) {
	return [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex{}, false
}

// Prune is a stub.
func (p *FakeProposerIndicesCache) Prune(epoch primitives.Epoch) {}

// Set is a stub.
func (p *FakeProposerIndicesCache) Set(epoch primitives.Epoch, root [32]byte, indices [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex) {
}

// SetUnsafe is a stub.
func (p *FakeProposerIndicesCache) SetUnsafe(epoch primitives.Epoch, root [32]byte, indices [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex) {
}

// SetCheckpoint is a stub.
func (p *FakeProposerIndicesCache) SetCheckpoint(c forkchoicetypes.Checkpoint, root [32]byte) {}

// IndicesFromCheckpoint is a stub.
func (p *FakeProposerIndicesCache) IndicesFromCheckpoint(_ forkchoicetypes.Checkpoint) ([fieldparams.SlotsPerEpoch]primitives.ValidatorIndex, bool) {
	return [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex{}, false
}
