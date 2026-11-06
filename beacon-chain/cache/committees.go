package cache

import (
	"errors"
	"sync/atomic"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
)

// ErrNotCommittee will be returned when a cache object is not a pointer to
// a Committee struct.
var ErrNotCommittee = errors.New("object is not a committee struct")

// committeeAggregates holds the aggregate public key of each committee and whether its computation started.
// started is never reset, so a failed computation is not retried.
type committeeAggregates struct {
	keys    atomic.Pointer[[]bls.PublicKey]
	started atomic.Bool
}

// Committees defines the shuffled committees seed.
type Committees struct {
	CommitteeCount  uint64
	Seed            [32]byte
	ShuffledIndices []primitives.ValidatorIndex
	SortedIndices   []primitives.ValidatorIndex
	aggregates      committeeAggregates
}

// SetAggregatePubkeys stores the aggregate public key of each committee, in committee offset order.
func (c *Committees) SetAggregatePubkeys(keys []bls.PublicKey) {
	c.aggregates.keys.Store(&keys)
}

// TryStartAggregating reports whether the caller is the first to compute the committee aggregates.
func (c *Committees) TryStartAggregating() bool {
	return c.aggregates.started.CompareAndSwap(false, true)
}
