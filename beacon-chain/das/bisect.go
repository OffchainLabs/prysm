package das

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
)

// Bisector describes a type that takes a set of RODataColumns via the Bisect method
// and returns a BisectionIterator that returns batches of those columns to be
// verified together.
type Bisector interface {
	// Bisect initializes the BisectionIterator and returns the result.
	Bisect([]blocks.RODataColumn) (BisectionIterator, error)
}

// BisectionIterator describes an iterator that returns groups of columns to verify.
// It is up to the bisector implementation to decide how to chunk up the columns,
// whether by block, by peer, or any other strategy. For example, backfill implements
// a bisector that keeps track of the source of each sidecar by peer, and groups
// sidecars by peer in the Next method, enabling it to track which peers, out of all
// the peers contributing to a batch, gave us bad data.
// When a batch fails, the OnError method should be used so that the bisector can
// keep track of the failed groups of columns and eg apply that knowledge in peer scoring.
// The same column will be returned multiple times by Next; first as part of a larger batch,
// and again as part of a more fine grained batch if there was an error in the large batch.
// For example, first as part of a batch of all columns spanning peers, and then again
// as part of a batch of columns from a single peer if some column in the larger batch
// failed verification.
type BisectionIterator interface {
	// Next returns the next group of columns to verify.
	// When the iteration is complete, Next should return (nil, io.EOF).
	Next() ([]blocks.RODataColumn, error)
	// OnError should be called when verification of a group of columns obtained via Next() fails.
	OnError(error)
	// Error can be used at the end of the iteration to get a single error result. It will return
	// nil if OnError was never called, or an error of the implementers choosing representing the set
	// of errors seen during iteration. For instance when bisecting from columns spanning peers to columns
	// from a single peer, the broader error could be dropped, and then the more specific error
	// (for a single peer's response) returned after bisecting to it.
	Error() error
}
