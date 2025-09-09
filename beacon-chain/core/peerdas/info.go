package peerdas

import (
	"encoding/binary"
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
