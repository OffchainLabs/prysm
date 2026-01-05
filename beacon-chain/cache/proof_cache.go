package cache

import (
	"errors"
	"fmt"
	"sync"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	lru "github.com/hashicorp/golang-lru"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// ProofCache is a thread-safe, synchronous LRU cache for execution proofs,
// storing proofs indexed by execution block hash.
type ProofCache struct {
	cache *lru.Cache
	lock  sync.RWMutex
}


// NewProofCache creates a new proof cache with the specified capacity.
func NewProofCache(capacity int) (*ProofCache, error) {
	if capacity <= 0 {
		return nil, errors.New("cache capacity must be > 0")
	}

	l, err := lru.New(capacity)
	if err != nil {
		return nil, fmt.Errorf("failed to create LRU cache: %w", err)
	}
	return &ProofCache{
		cache: l,
	}, nil
}

// Get all proofs for a specific block hash.
// It uses Peek to avoid promoting the entry in the LRU order.
func (c *ProofCache) Get(blockHash []byte) ([]ethpb.ExecutionProof, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return nil, false
	}

	proofs := val.([]ethpb.ExecutionProof)
	// Return a deep copy to prevent concurrent modification by the caller
	proofsCopy := make([]ethpb.ExecutionProof, len(proofs))
	for i := range proofs {
		proofsCopy[i] = *proofs[i].Copy()
	}

	return proofsCopy, true
}

// Insert a proof into the cache.
// If a proof from the same subnet already exists for this block hash,
// it will be replaced.
func (c *ProofCache) Put(proof *ethpb.ExecutionProof) {
	c.lock.Lock()
	defer c.lock.Unlock()

	blockHash := proof.BlockHash
	var existingProofs []ethpb.ExecutionProof

	// Get existing proofs, if any.
	// We use Get() here to promote the entry
	if val, ok := c.cache.Get(blockHash); ok {
		existingProofs = val.([]ethpb.ExecutionProof)
	}

	// Filter out any existing proof from the same subnet
	newProofs := make([]ethpb.ExecutionProof, 0, len(existingProofs)+1)
	for i := range existingProofs {
		if existingProofs[i].ProofId != proof.ProofId {
			newProofs = append(newProofs, *existingProofs[i].Copy())
		}
	}
	// Add the new proof
	newProofs = append(newProofs, *proof.Copy())

	// Add the new slice back to the cache
	c.cache.Add(blockHash, newProofs)
}

// GetFromSubnets gets proofs for a specific block hash from specific subnets.
func (c *ProofCache) GetFromSubnets(blockHash []byte, subnetIds []primitives.ExecutionProofId) []ethpb.ExecutionProof {
	c.lock.RLock()
	defer c.lock.RUnlock()

	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return []ethpb.ExecutionProof{}
	}

	proofs := val.([]ethpb.ExecutionProof)
	// Create a map for quick lookup
	subnetSet := make(map[primitives.ExecutionProofId]struct{}, len(subnetIds))
	for _, id := range subnetIds {
		subnetSet[id] = struct{}{}
	}

	filteredProofs := make([]ethpb.ExecutionProof, 0)
	for i := range proofs {
		if _, exists := subnetSet[proofs[i].ProofId]; exists {
			filteredProofs = append(filteredProofs, *proofs[i].Copy())
		}
	}
	return filteredProofs
}

// HasRequiredProofs checks if we have the minimum required number of proofs
// from _different_ subnets.
func (c *ProofCache) HasRequiredProofs(blockHash []byte, minRequired int) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return false
	}

	return len(val.([]ethpb.ExecutionProof)) >= minRequired
}

// SubnetCount gets the number of unique subnets/proofs we have for a
// particular execution payload.
func (c *ProofCache) SubnetCount(blockHash []byte) int {
	c.lock.RLock()
	defer c.lock.RUnlock()

	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return 0
	}
	return len(val.([]ethpb.ExecutionProof))
}

// HasProofFromSubnet checks if a proof exists from a specific subnet for a block.
func (c *ProofCache) HasProofFromSubnet(blockHash []byte, subnetId primitives.ExecutionProofId) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return false
	}

	proofs := val.([]ethpb.ExecutionProof)
	for i := range proofs {
		if proofs[i].ProofId == subnetId {
			return true
		}
	}
	return false
}

// Remove all proofs for a specific block hash.
func (c *ProofCache) Remove(blockHash []byte) ([]ethpb.ExecutionProof, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// lru.Remove just returns bool, so we Peek first to get the value.
	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return nil, false
	}

	c.cache.Remove(blockHash)

	// Return a deep copy
	proofs := val.([]ethpb.ExecutionProof)
	proofsCopy := make([]ethpb.ExecutionProof, len(proofs))
	for i := range proofs {
		proofsCopy[i] = *proofs[i].Copy()
	}
	return proofsCopy, true
}

// Clear all cached proofs.
func (c *ProofCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.cache.Purge()
}

// Len gets the current number of entries (block hashes) in the cache.
func (c *ProofCache) Count() int {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.cache.Len()
}

// IsEmpty checks if the cache is empty.
func (c *ProofCache) IsEmpty() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.cache.Len() == 0
}
