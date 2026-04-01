//go:build fuzz

// This file is used in fuzzer builds to bypass the payload committee cache.
package cache

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// FakePayloadCommitteeCache is a no-op implementation of the payload committee cache for fuzz builds.
type FakePayloadCommitteeCache struct{}

// NewPayloadCommitteeCache creates a new fake cache.
func NewPayloadCommitteeCache() *FakePayloadCommitteeCache {
	return &FakePayloadCommitteeCache{}
}

// Get is a stub.
func (c *FakePayloadCommitteeCache) Get(_ context.Context, _ [32]byte) ([]primitives.ValidatorIndex, error) {
	return nil, nil
}

// Add is a stub.
func (c *FakePayloadCommitteeCache) Add(_ [32]byte, _ []primitives.ValidatorIndex) {}

// MarkInProgress is a stub.
func (c *FakePayloadCommitteeCache) MarkInProgress(_ [32]byte) error {
	return nil
}

// MarkNotInProgress is a stub.
func (c *FakePayloadCommitteeCache) MarkNotInProgress(_ [32]byte) error {
	return nil
}

// Clear is a stub.
func (c *FakePayloadCommitteeCache) Clear() {}
