package peerdas

import (
	"encoding/binary"
	"maps"
	"sync"

	"github.com/ethereum/go-ethereum/p2p/enode"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
)

// info contains all useful peerDAS related information regarding a peer.
type info struct {
	CustodyGroups      map[uint64]bool
	CustodyColumns     map[uint64]bool
	DataColumnsSubnets map[uint64]bool
}

const (
	nodeInfoCacheSize   = 200
	nodeInfoCachKeySize = 32 + 8
)

var (
	nodeInfoCacheMut sync.Mutex
	nodeInfoCache    *lru.Cache
)

// Info returns the peerDAS information for a given nodeID and custodyGroupCount.
// It returns a boolean indicating if the peer info was already in the cache and an error if any.
func Info(nodeID enode.ID, custodyGroupCount uint64) (*info, bool, error) {
	// Create a new cache if it doesn't exist.
	if err := createInfoCacheIfNeeded(); err != nil {
		return nil, false, errors.Wrap(err, "create cache if needed")
	}

	// Compute the key.
	key := computeInfoCacheKey(nodeID, custodyGroupCount)

	// If the value is already in the cache, return it.
	if value, ok := nodeInfoCache.Get(key); ok {
		peerInfo, ok := value.(*info)
		if !ok {
			return nil, false, errors.New("failed to cast peer info (should never happen)")
		}

		return peerInfo, true, nil
	}

	// The peer info is not in the cache, compute it.
	// Compute custody groups.
	custodyGroups, err := CustodyGroups(nodeID, custodyGroupCount)
	if err != nil {
		return nil, false, errors.Wrap(err, "custody groups")
	}

	// Compute custody columns.
	custodyColumns, err := CustodyColumns(custodyGroups)
	if err != nil {
		return nil, false, errors.Wrap(err, "custody columns")
	}

	// Compute data columns subnets.
	dataColumnsSubnets := DataColumnSubnets(custodyColumns)

	// Convert the custody groups to a map.
	custodyGroupsMap := make(map[uint64]bool, len(custodyGroups))
	for _, group := range custodyGroups {
		custodyGroupsMap[group] = true
	}

	result := &info{
		CustodyGroups:      custodyGroupsMap,
		CustodyColumns:     custodyColumns,
		DataColumnsSubnets: dataColumnsSubnets,
	}

	// Add the result to the cache.
	nodeInfoCache.Add(key, result)

	return result, false, nil
}

// createInfoCacheIfNeeded creates a new cache if it doesn't exist.
func createInfoCacheIfNeeded() error {
	nodeInfoCacheMut.Lock()
	defer nodeInfoCacheMut.Unlock()

	if nodeInfoCache == nil {
		c, err := lru.New(nodeInfoCacheSize)
		if err != nil {
			return errors.Wrap(err, "lru new")
		}

		nodeInfoCache = c
	}

	return nil
}

// computeInfoCacheKey returns a unique key for a node and its custodyGroupCount.
func computeInfoCacheKey(nodeID enode.ID, custodyGroupCount uint64) [nodeInfoCachKeySize]byte {
	var key [nodeInfoCachKeySize]byte

	copy(key[:32], nodeID[:])
	binary.BigEndian.PutUint64(key[32:], custodyGroupCount)

	return key
}

// ColumnIndices represents as a set of ColumnIndices. This could be the set of indices that a node is required to custody,
// the set that a peer custodies, missing indices for a given block, indices that are present on disk, etc.
type ColumnIndices map[uint64]struct{}

// Has returns true if the index is present in the ColumnIndices.
func (ci ColumnIndices) Has(index uint64) bool {
	_, ok := ci[index]
	return ok
}

// Count returns the number of indices present in the ColumnIndices.
func (ci ColumnIndices) Count() int {
	return len(ci)
}

// Set sets the index in the ColumnIndices.
func (ci ColumnIndices) Set(index uint64) {
	ci[index] = struct{}{}
}

// Unset removes the index from the ColumnIndices.
func (ci ColumnIndices) Unset(index uint64) {
	delete(ci, index)
}

// Copy creates a copy of the ColumnIndices.
func (ci ColumnIndices) Copy() ColumnIndices {
	newCi := make(ColumnIndices, len(ci))
	maps.Copy(newCi, ci)
	return newCi
}

// Intersection returns a new ColumnIndices that contains only the indices that are present in both ColumnIndices.
func (ci ColumnIndices) Intersection(other ColumnIndices) ColumnIndices {
	result := make(ColumnIndices)
	for index := range ci {
		if other.Has(index) {
			result.Set(index)
		}
	}
	return result
}

// Merge mutates the receiver so that any index that is set in either of
// the two ColumnIndices is set in the receiver after the function finishes.
// It does not mutate the other ColumnIndices given as a function argument.
func (ci ColumnIndices) Merge(other ColumnIndices) {
	for index := range other {
		ci.Set(index)
	}
}

// ToMap converts a ColumnIndices into a map[uint64]struct{}.
// In the future ColumnIndices may be changed to a bit map, so using
// ToMap will ensure forwards-compatibility.
func (ci ColumnIndices) ToMap() map[uint64]struct{} {
	return ci.Copy()
}

// ToSlice converts a ColumnIndices into a slice of uint64 indices.
func (ci ColumnIndices) ToSlice() []uint64 {
	indices := make([]uint64, 0, len(ci))
	for index := range ci {
		indices = append(indices, index)
	}
	return indices
}

// NewColumnIndicesFromSlice creates a ColumnIndices from a slice of uint64.
func NewColumnIndicesFromSlice(indices []uint64) ColumnIndices {
	ci := make(ColumnIndices, len(indices))
	for _, index := range indices {
		ci[index] = struct{}{}
	}
	return ci
}

// NewColumnIndicesFromMap creates a ColumnIndices from a map[uint64]bool. This kind of map
// is used in several places in peerdas code. Converting from this map type to ColumnIndices
// will allow us to move ColumnIndices underlying type to a bitmap in the future and avoid
// lots of loops for things like intersections/unions or copies.
func NewColumnIndicesFromMap(indices map[uint64]bool) ColumnIndices {
	ci := make(ColumnIndices, len(indices))
	for index, set := range indices {
		if !set {
			continue
		}
		ci[index] = struct{}{}
	}
	return ci
}

// NewColumnIndices creates an empty ColumnIndices.
// In the future ColumnIndices may change from a reference type to a value type,
// so using this constructor will ensure forwards-compatibility.
func NewColumnIndices() ColumnIndices {
	return make(ColumnIndices)
}
