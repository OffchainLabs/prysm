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
	cacheSize = 200
	keySize   = 32 + 8
)

var (
	mut   sync.Mutex
	cache *lru.Cache
)

// Info returns the peerDAS information for a given nodeID and custodyGroupCount.
// It returns a boolean indicating if the peer info was already in the cache and an error if any.
func Info(nodeID enode.ID, custodyGroupCount uint64) (*info, bool, error) {
	// Create a new cache if it doesn't exist.
	if err := createCacheIfNeeded(); err != nil {
		return nil, false, errors.Wrap(err, "create cache if needed")
	}

	// Compute the key.
	key := computeKey(nodeID, custodyGroupCount)

	// If the value is already in the cache, return it.
	if value, ok := cache.Get(key); ok {
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

	result := &info{
		CustodyGroups:      custodyGroups,
		CustodyColumns:     custodyColumns,
		DataColumnsSubnets: dataColumnsSubnets,
	}

	// Add the result to the cache.
	cache.Add(key, result)

	return result, false, nil
}

// createCacheIfNeeded creates a new cache if it doesn't exist.
func createCacheIfNeeded() error {
	mut.Lock()
	defer mut.Unlock()

	if cache == nil {
		c, err := lru.New(cacheSize)
		if err != nil {
			return errors.Wrap(err, "lru new")
		}

		cache = c
	}

	return nil
}

// computeKey returns a unique key for a node and its custodyGroupCount.
func computeKey(nodeID enode.ID, custodyGroupCount uint64) [keySize]byte {
	var key [keySize]byte

	copy(key[:32], nodeID[:])
	binary.BigEndian.PutUint64(key[32:], custodyGroupCount)

	return key
}
