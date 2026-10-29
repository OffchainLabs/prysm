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

// Committees defines the shuffled committees seed.
type Committees struct {
	CommitteeCount   uint64
	Seed             [32]byte
	ShuffledIndices  []primitives.ValidatorIndex
	SortedIndices    []primitives.ValidatorIndex
	aggregatePubkeys atomic.Pointer[[]bls.PublicKey]
}

// SetAggregatePubkeys stores the aggregate public key of each committee, in committee offset order.
func (c *Committees) SetAggregatePubkeys(keys []bls.PublicKey) {
	c.aggregatePubkeys.Store(&keys)
}
