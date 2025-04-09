//go:build fuzz

// This file is used in fuzzer builds to bypass global committee caches.
package cache

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

// FakeCommitteeCache is a struct with 1 queue for looking up shuffled indices list by seed.
type FakeCommitteeCache struct {
}

// NewCommitteesCache creates a new committee cache for storing/accessing shuffled indices of a committee.
func NewCommitteesCache() *FakeCommitteeCache {
	return &FakeCommitteeCache{}
}

// Committee fetches the shuffled indices by slot and committee index. Every list of indices
// represent one committee. Returns true if the list exists with slot and committee index. Otherwise returns false, nil.
func (c *FakeCommitteeCache) Committee(ctx context.Context, slot primitives.Slot, seed [32]byte, index primitives.CommitteeIndex) ([]primitives.ValidatorIndex, error) {
	return nil, nil
}

// AddCommitteeShuffledList adds Committee shuffled list object to the cache. T
// his method also trims the least recently list if the cache size has ready the max cache size limit.
func (c *FakeCommitteeCache) AddCommitteeShuffledList(ctx context.Context, committees *Committees) error {
	return nil
}

// AddProposerIndicesList updates the committee shuffled list with proposer indices.
func (c *FakeCommitteeCache) AddProposerIndicesList(seed [32]byte, indices []primitives.ValidatorIndex) error {
	return nil
}

// ActiveIndices returns the active indices of a given seed stored in cache.
func (c *FakeCommitteeCache) ActiveIndices(ctx context.Context, seed [32]byte) ([]primitives.ValidatorIndex, error) {
	return nil, nil
}

// ActiveIndicesCount returns the active indices count of a given seed stored in cache.
func (c *FakeCommitteeCache) ActiveIndicesCount(ctx context.Context, seed [32]byte) (int, error) {
	return 0, nil
}

// ActiveBalance returns the active balance of a given seed stored in cache.
func (c *FakeCommitteeCache) ActiveBalance(seed [32]byte) (uint64, error) {
	return 0, nil
}

// ProposerIndices returns the proposer indices of a given seed.
func (c *FakeCommitteeCache) ProposerIndices(seed [32]byte) ([]primitives.ValidatorIndex, error) {
	return nil, nil
}

// HasEntry returns true if the committee cache has a value.
func (c *FakeCommitteeCache) HasEntry(string) bool {
	return false
}

// MarkInProgress is a stub.
func (c *FakeCommitteeCache) MarkInProgress(seed [32]byte) error {
	return nil
}

// MarkNotInProgress is a stub.
func (c *FakeCommitteeCache) MarkNotInProgress(seed [32]byte) error {
	return nil
}

// Clear is a stub.
func (c *FakeCommitteeCache) Clear() {
	return
}

func (c *FakeCommitteeCache) ExpandCommitteeCache() {
	return
}

func (c *FakeCommitteeCache) CompressCommitteeCache() {
	return
}
