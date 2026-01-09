package das

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// AvailabilityChecker is the minimum interface needed to check if data is available for a block.
// By convention there is a concept of an AvailabilityStore that implements a method to persist
// blobs or data columns to prepare for Availability checking, but since those methods are different
// for different forms of blob data, they are not included in the interface.
type AvailabilityChecker interface {
	IsDataAvailable(ctx context.Context, current primitives.Slot, b ...blocks.ROBlock) error
}

// RetentionChecker is a callback that determines whether blobs at the given slot are within the retention period.
type RetentionChecker func(primitives.Slot) bool
