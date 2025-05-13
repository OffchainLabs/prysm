package das

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
)

// Bisector describes a type that takes a set of RODataColumns via the Bisect method
// and implements an iterator pattern to return subsets of those columns via Next().
// It is up to the bisector implementation to decide how to chunk up the columns,
// whether by block, by peer, or any other strategy. For example, backfill implements
// a bisector that keeps track of the source of each sidecar by peer, and groups
// sidecars by peer in the Next method, enabling it to track which peers, out of all
// the peers contributing to a batch, gave us bad data.
// When a batch fails, the OnError method should be used so that the bisector can
// keep track of the failed groups of columns and eg apply that knowledge in peer scoring.
type Bisector interface {
	// Bisect prepares to break up a set of columns into groups for verification. It must be called
	// to initialize the Bisector before Next() is called.
	Bisect([]blocks.RODataColumn) error
	// Next returns the next group of columns to verify.
	// When the iteration is complete, Next should return (nil, io.EOF).
	Next() ([]blocks.RODataColumn, error)
	// OnError should be called when verification of a group of columns obtained via next fails.
	OnError(error)
	// Error can be used at the end of the iteration to get a single error result. It will return
	// nil if OnError was never called, or an error of the implementers choosing representing the set
	// of errors seen during iteration.
	Error() error
}
