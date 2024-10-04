package das

import (
	"context"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

// AvailabilityStore describes a component that can verify and save sidecars for a given block, and confirm previously
// verified and saved sidecars.
// Persist guarantees that the sidecar will be available to perform a DA check
// for the life of the beacon node process.
// IsDataAvailable guarantees that all blobs committed to in the block have been
// durably persisted before returning a non-error value.
type AvailabilityStore interface {
	IsDataAvailable(ctx context.Context, nodeID enode.ID, current primitives.Slot, b blocks.ROBlock) error
	Persist(current primitives.Slot, sc ...blocks.ROBlob) error
}
