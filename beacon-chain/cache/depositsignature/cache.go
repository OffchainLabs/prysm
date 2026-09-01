// Package depositsignature caches the pure result of deposit proof-of-possession checks.
package depositsignature

import (
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Cache stores signature validity by the complete signed deposit data root.
type Cache struct {
	mu             sync.RWMutex
	validity       map[[32]byte]bool
	builderPubkeys map[[48]byte]bool
}

// New returns an empty deposit signature cache.
func New() *Cache {
	return &Cache{
		validity:       make(map[[32]byte]bool),
		builderPubkeys: make(map[[48]byte]bool),
	}
}

// TrackBuilderPubkey records a pubkey whose builder deposit was seen by the scanner.
func (c *Cache) TrackBuilderPubkey(pubkey []byte) {
	if c == nil {
		return
	}
	key := pubkeyKey(pubkey)
	c.mu.Lock()
	if c.builderPubkeys == nil {
		c.builderPubkeys = make(map[[48]byte]bool)
	}
	if _, ok := c.builderPubkeys[key]; !ok {
		c.builderPubkeys[key] = false
	}
	c.mu.Unlock()
}

// ShouldVerifyValidator reports whether validator deposits for a builder pubkey are still needed.
func (c *Cache) ShouldVerifyValidator(pubkey []byte) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	validValidator, tracked := c.builderPubkeys[pubkeyKey(pubkey)]
	c.mu.RUnlock()
	return tracked && !validValidator
}

// MarkValidValidator records that the fork transition can stop at a valid validator deposit.
func (c *Cache) MarkValidValidator(pubkey []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.builderPubkeys == nil {
		c.builderPubkeys = make(map[[48]byte]bool)
	}
	c.builderPubkeys[pubkeyKey(pubkey)] = true
	c.mu.Unlock()
}

// BuilderPubkeyLen returns the number of builder pubkeys discovered by the scanner.
func (c *Cache) BuilderPubkeyLen() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.builderPubkeys)
}

// Get returns a cached signature result when one exists.
func (c *Cache) Get(deposit *ethpb.PendingDeposit) (bool, bool) {
	if c == nil {
		return false, false
	}
	key, err := cacheKey(deposit)
	if err != nil {
		return false, false
	}
	c.mu.RLock()
	valid, ok := c.validity[key]
	c.mu.RUnlock()
	return valid, ok
}

// Verify verifies one pending deposit, returning a cached result on a hit.
func (c *Cache) Verify(deposit *ethpb.PendingDeposit) (bool, error) {
	if valid, ok := c.Get(deposit); ok {
		return valid, nil
	}
	valid, err := helpers.IsValidDepositSignature(depositData(deposit))
	if err != nil {
		return false, err
	}
	c.store(deposit, valid)
	return valid, nil
}

// VerifyBatch verifies pending deposits together and caches both valid and invalid results.
func (c *Cache) VerifyBatch(ctx context.Context, deposits []*ethpb.PendingDeposit) ([]bool, error) {
	validity := make([]bool, len(deposits))
	misses := make([]*ethpb.PendingDeposit, 0, len(deposits))
	missIndices := make([]int, 0, len(deposits))
	for i, deposit := range deposits {
		if valid, ok := c.Get(deposit); ok {
			validity[i] = valid
			continue
		}
		misses = append(misses, deposit)
		missIndices = append(missIndices, i)
	}
	if len(misses) == 0 {
		return validity, nil
	}

	allValid, err := helpers.BatchVerifyPendingDepositsSignatures(ctx, misses)
	if err != nil {
		return nil, err
	}
	if allValid {
		for i, deposit := range misses {
			validity[missIndices[i]] = true
			c.store(deposit, true)
		}
		return validity, nil
	}

	for i, deposit := range misses {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		valid, err := c.Verify(deposit)
		if err != nil {
			return nil, err
		}
		validity[missIndices[i]] = valid
	}
	return validity, nil
}

// CopyFrom copies all verified facts from another cache.
func (c *Cache) CopyFrom(other *Cache) {
	if c == nil || other == nil || c == other {
		return
	}
	other.mu.RLock()
	values := make(map[[32]byte]bool, len(other.validity))
	for key, valid := range other.validity {
		values[key] = valid
	}
	builderPubkeys := make(map[[48]byte]bool, len(other.builderPubkeys))
	for key, validValidator := range other.builderPubkeys {
		builderPubkeys[key] = validValidator
	}
	other.mu.RUnlock()

	c.mu.Lock()
	if c.validity == nil {
		c.validity = make(map[[32]byte]bool)
	}
	for key, valid := range values {
		c.validity[key] = valid
	}
	if c.builderPubkeys == nil {
		c.builderPubkeys = make(map[[48]byte]bool)
	}
	for key, validValidator := range builderPubkeys {
		if current, ok := c.builderPubkeys[key]; !ok || !current {
			c.builderPubkeys[key] = validValidator
		}
	}
	c.mu.Unlock()
}

// Len returns the number of cached signature results.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.validity)
}

// Clear releases all cached signature results.
func (c *Cache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.validity = make(map[[32]byte]bool)
	c.builderPubkeys = make(map[[48]byte]bool)
	c.mu.Unlock()
}

func (c *Cache) store(deposit *ethpb.PendingDeposit, valid bool) {
	if c == nil {
		return
	}
	key, err := cacheKey(deposit)
	if err != nil {
		return
	}
	c.mu.Lock()
	if c.validity == nil {
		c.validity = make(map[[32]byte]bool)
	}
	c.validity[key] = valid
	c.mu.Unlock()
}

func pubkeyKey(pubkey []byte) [48]byte {
	var key [48]byte
	copy(key[:], pubkey)
	return key
}

func cacheKey(deposit *ethpb.PendingDeposit) ([32]byte, error) {
	return depositData(deposit).HashTreeRoot()
}

func depositData(deposit *ethpb.PendingDeposit) *ethpb.Deposit_Data {
	if deposit == nil {
		return &ethpb.Deposit_Data{}
	}
	return &ethpb.Deposit_Data{
		PublicKey:             deposit.PublicKey,
		WithdrawalCredentials: deposit.WithdrawalCredentials,
		Amount:                deposit.Amount,
		Signature:             deposit.Signature,
	}
}
