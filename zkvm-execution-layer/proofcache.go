package zkvmexecutionlayer

import (
	"errors"
	"fmt"
	"sync"
	"github.com/ethereum/go-ethereum/common"
	executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"
	lru "github.com/hashicorp/golang-lru"
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

// Insert a proof into the cache.
// If a proof from the same subnet already exists for this block hash,
// it will be replaced.
func (c *ProofCache) Insert(proof executionproof.ExecutionProof) {
	c.lock.Lock()
	defer c.lock.Unlock()

	blockHash := proof.BlockHash
	var existingProofs []executionproof.ExecutionProof

	// Get existing proofs, if any.
	// We use Get() here to promote the entry, similar to Rust's get_or_insert_mut.
	if val, ok := c.cache.Get(blockHash); ok {
		existingProofs = val.([]executionproof.ExecutionProof)
	}

	// Filter out any existing proof from the same subnet (simulates Rust's .retain())
	newProofs := make([]executionproof.ExecutionProof, 0, len(existingProofs)+1)
	for _, p := range existingProofs {
		if p.SubnetId != proof.SubnetId {
			newProofs = append(newProofs, p)
		}
	}

	// Add the new proof
	newProofs = append(newProofs, proof)

	// Add the new slice back to the cache
	c.cache.Add(blockHash, newProofs)
}

// Get all proofs for a specific block hash.
// It uses Peek to avoid promoting the entry in the LRU order.
func (c *ProofCache) Get(blockHash common.Hash) ([]executionproof.ExecutionProof, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return nil, false
	}

	proofs := val.([]executionproof.ExecutionProof)
	// Return a copy to prevent concurrent modification by the caller
	proofsCopy := make([]executionproof.ExecutionProof, len(proofs))
	copy(proofsCopy, proofs)

	return proofsCopy, true
}

// GetFromSubnets gets proofs for a specific block hash from specific subnets.
func (c *ProofCache) GetFromSubnets(blockHash common.Hash, subnetIds []executionproof.ExecutionProofSubnetId) []executionproof.ExecutionProof {
	c.lock.RLock()
	defer c.lock.RUnlock()

	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return []executionproof.ExecutionProof{}
	}

	proofs := val.([]executionproof.ExecutionProof)
	// Create a map for quick lookup
	subnetSet := make(map[executionproof.ExecutionProofSubnetId]struct{}, len(subnetIds))
	for _, id := range subnetIds {
		subnetSet[id] = struct{}{}
	}

	filteredProofs := make([]executionproof.ExecutionProof, 0)
	for _, p := range proofs {
		if _, exists := subnetSet[p.SubnetId]; exists {
			filteredProofs = append(filteredProofs, p) // appends a copy
		}
	}
	return filteredProofs
}

// HasRequiredProofs checks if we have the minimum required number of proofs
// from _different_ subnets.
func (c *ProofCache) HasRequiredProofs(blockHash common.Hash, minRequired int) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return false
	}

	return len(val.([]executionproof.ExecutionProof)) >= minRequired
}

// SubnetCount gets the number of unique subnets/proofs we have for a
// particular execution payload.
func (c *ProofCache) SubnetCount(blockHash common.Hash) int {
	c.lock.RLock()
	defer c.lock.RUnlock()

	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return 0
	}
	return len(val.([]executionproof.ExecutionProof))
}

// HasProofFromSubnet checks if a proof exists from a specific subnet for a block.
func (c *ProofCache) HasProofFromSubnet(blockHash common.Hash, subnetId executionproof.ExecutionProofSubnetId) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return false
	}

	proofs := val.([]executionproof.ExecutionProof)
	for _, p := range proofs {
		if p.SubnetId == subnetId {
			return true
		}
	}
	return false
}

// Remove all proofs for a specific block hash.
func (c *ProofCache) Remove(blockHash common.Hash) ([]executionproof.ExecutionProof, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// lru.Remove just returns bool, so we Peek first to get the value.
	val, ok := c.cache.Peek(blockHash)
	if !ok {
		return nil, false
	}

	c.cache.Remove(blockHash)

	// Return a copy
	proofs := val.([]executionproof.ExecutionProof)
	proofsCopy := make([]executionproof.ExecutionProof, len(proofs))
	copy(proofsCopy, proofs)
	return proofsCopy, true
}

// Clear all cached proofs.
func (c *ProofCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.cache.Purge() // Purge is the equivalent of Rust's clear
}

// Len gets the current number of entries (block hashes) in the cache.
func (c *ProofCache) Len() int {
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