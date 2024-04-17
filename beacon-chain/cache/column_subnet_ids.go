package cache

import (
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/prysmaticlabs/prysm/v5/config/params"
)

type columnSubnetIDs struct {
	colSubCache *cache.Cache
	colSubLock  sync.RWMutex
}

// ColumnSubnetIDs for column subnet participants
var ColumnSubnetIDs = newColumnSubnetIDs()

const columnKey = "columns"

func newColumnSubnetIDs() *columnSubnetIDs {
	epochDuration := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	// Set the default duration of a column subnet subscription as the column expiry period.
	subLength := epochDuration * time.Duration(params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest)
	persistentCache := cache.New(subLength*time.Second, epochDuration*time.Second)
	return &columnSubnetIDs{colSubCache: persistentCache}
}

// GetColumnSubnets retrieves the data column subnets.
func (s *columnSubnetIDs) GetColumnSubnets() ([]uint64, bool, time.Time) {
	s.colSubLock.RLock()
	defer s.colSubLock.RUnlock()

	id, duration, ok := s.colSubCache.GetWithExpiration(columnKey)
	if !ok {
		return nil, false, time.Time{}
	}
	// Retrieve indices from the cache.
	idxs, ok := id.([]uint64)
	if !ok {
		return nil, false, time.Time{}
	}

	return idxs, ok, duration
}

// AddColumnSubnets adds the relevant data column subnets.
func (s *columnSubnetIDs) AddColumnSubnets(colIdx []uint64) {
	s.colSubLock.Lock()
	defer s.colSubLock.Unlock()

	s.colSubCache.Set(columnKey, colIdx, 0)
}

// EmptyAllCaches empties out all the related caches and flushes any stored
// entries on them. This should only ever be used for testing, in normal
// production, handling of the relevant subnets for each role is done
// separately.
func (s *columnSubnetIDs) EmptyAllCaches() {
	// Clear the cache.
	s.colSubLock.Lock()
	defer s.colSubLock.Unlock()

	s.colSubCache.Flush()
}
