package cache

import (
	"maps"
	"sync"

	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	lru "github.com/hashicorp/golang-lru"
)

// executionProofRetentionEpochs sizes the caches below, in epochs' worth of
// beacon blocks.
//
// EIP-8025 defines no retention period: its fork-choice Store simply
// accumulates verified proofs. A bounded cache is enough in practice because
// proofs are recursive, so a proof for block n already attests to the validity
// of the chain prefix ending at n and supersedes its predecessors.
const executionProofRetentionEpochs = 2

// ExecutionProofCache holds the recent per-block state of EIP-8025: the
// identifier of each revealed payload, and the verified execution proof
// envelopes for it, at most one per proof type.
type ExecutionProofCache struct {
	// maps a beacon block root to its verified proofs, keyed by proof type
	proofs *lru.Cache
	mu     sync.Mutex

	// maps a beacon block root to the hash tree root of the new payload request
	// for the payload revealed for it.
	payloadRoots *lru.Cache
}

// NewExecutionProofCache creates an empty execution proof cache.
func NewExecutionProofCache() *ExecutionProofCache {
	// lint:ignore uintcast -- preset value that would panic on startup if negative.
	size := int(params.BeaconConfig().SlotsPerEpoch) * executionProofRetentionEpochs
	return &ExecutionProofCache{
		proofs:       lruwrpr.New(size),
		payloadRoots: lruwrpr.New(size),
	}
}

// SetNewPayloadRequestRoot records the new payload request root of the payload
// revealed for a beacon block, marking the payload as available to prove.
func (c *ExecutionProofCache) SetNewPayloadRequestRoot(
	blockRoot [fieldparams.RootLength]byte,
	newPayloadRequestRoot [fieldparams.RootLength]byte,
) {
	if c == nil {
		return
	}

	c.payloadRoots.Add(blockRoot, newPayloadRequestRoot)
}

// NewPayloadRequestRoot returns the new payload request root recorded for a
// beacon block.
func (c *ExecutionProofCache) NewPayloadRequestRoot(blockRoot [fieldparams.RootLength]byte) ([fieldparams.RootLength]byte, bool) {
	if c == nil {
		return [fieldparams.RootLength]byte{}, false
	}

	value, ok := c.payloadRoots.Get(blockRoot)
	if !ok {
		return [fieldparams.RootLength]byte{}, false
	}

	root, ok := value.([fieldparams.RootLength]byte)
	return root, ok
}

// Has reports whether a verified proof of this type is already known for the
// beacon block.
func (c *ExecutionProofCache) Has(blockRoot [fieldparams.RootLength]byte, proofType ethpb.ProofType) bool {
	if c == nil {
		return false
	}

	_, ok := c.byType(blockRoot)[proofType]
	return ok
}

// Save stores a verified proof envelope. It reports whether the proof was
// stored. A proof of a type already known for this beacon block is dropped, so
// that the first verified proof of each type wins.
func (c *ExecutionProofCache) Save(proof blocks.VerifiedROSignedExecutionProofEnvelope) bool {
	if c == nil {
		return false
	}

	blockRoot := proof.BeaconBlockRoot()
	proofType := proof.ProofType()

	c.mu.Lock()
	defer c.mu.Unlock()

	byType := c.byType(blockRoot)
	if _, ok := byType[proofType]; ok {
		return false
	}

	// Replace the whole map rather than mutating it in place, so that a reader
	// holding the previous value is unaffected. maps.Clone is not usable here:
	// byType is nil on the first proof for a block, and Clone preserves nil.
	updated := make(map[ethpb.ProofType]blocks.VerifiedROSignedExecutionProofEnvelope, len(byType)+1)
	maps.Copy(updated, byType)

	updated[proofType] = proof
	c.proofs.Add(blockRoot, updated)

	return true
}

// Get returns the verified proofs known for a beacon block, keyed by proof type.
func (c *ExecutionProofCache) Get(blockRoot [fieldparams.RootLength]byte) map[ethpb.ProofType]blocks.VerifiedROSignedExecutionProofEnvelope {
	if c == nil {
		return nil
	}

	byType := c.byType(blockRoot)
	if len(byType) == 0 {
		return nil
	}

	return maps.Clone(byType)
}

// byType returns the proofs stored for a beacon block. The returned map must be
// treated as read-only: Save replaces it rather than mutating it.
func (c *ExecutionProofCache) byType(blockRoot [fieldparams.RootLength]byte) map[ethpb.ProofType]blocks.VerifiedROSignedExecutionProofEnvelope {
	value, ok := c.proofs.Get(blockRoot)
	if !ok {
		return nil
	}

	byType, ok := value.(map[ethpb.ProofType]blocks.VerifiedROSignedExecutionProofEnvelope)
	if !ok {
		return nil
	}

	return byType
}
