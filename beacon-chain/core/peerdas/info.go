package peerdas

import (
	"encoding/binary"
	"sync"

	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/ethereum/go-ethereum/p2p/enode"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
)

// info contains all useful peerDAS related information regarding a peer.
type (
	info struct {
		CustodyGroups      map[uint64]bool
		CustodyColumns     map[uint64]bool
		DataColumnsSubnets map[uint64]bool
	}

	targetCustodyGroupCount struct {
		mut                          sync.RWMutex
		validatorsCustodyRequirement uint64
	}

	toAdverstiseCustodyGroupCount struct {
		mut   sync.RWMutex
		value uint64
	}

	CustodyInfo struct {
		// Mut is a mutex to be used by caller to ensure neither
		// TargetCustodyGroupCount nor ToAdvertiseCustodyGroupCount are being modified.
		// (This is not necessary to use this mutex for any data protection.)
		Mut sync.RWMutex

		// TargetGroupCount represents the target number of custody groups we should custody
		// regarding the validators we are tracking.
		TargetGroupCount targetCustodyGroupCount

		// ToAdvertiseGroupCount represents the number of custody groups to advertise to the network.
		ToAdvertiseGroupCount toAdverstiseCustodyGroupCount
	}
)

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

// ActualGroupCount returns the actual custody group count.
func (custodyInfo *CustodyInfo) ActualGroupCount() uint64 {
	return min(custodyInfo.TargetGroupCount.Get(), custodyInfo.ToAdvertiseGroupCount.Get())
}

// CustodyGroupCount returns the number of groups we should participate in for custody.
func (tcgc *targetCustodyGroupCount) Get() uint64 {
	// If subscribed to all subnets, return the number of custody groups.
	if flags.Get().SubscribeToAllSubnets {
		return params.BeaconConfig().NumberOfCustodyGroups
	}

	tcgc.mut.RLock()
	defer tcgc.mut.RUnlock()

	// If no validators are tracked, return the default custody requirement.
	if tcgc.validatorsCustodyRequirement == 0 {
		return params.BeaconConfig().CustodyRequirement
	}

	// Return the validators custody requirement.
	return tcgc.validatorsCustodyRequirement
}

// setValidatorsCustodyRequirement sets the validators custody requirement.
func (tcgc *targetCustodyGroupCount) SetValidatorsCustodyRequirement(value uint64) {
	tcgc.mut.Lock()
	defer tcgc.mut.Unlock()

	tcgc.validatorsCustodyRequirement = value
}

// Get returns the to advertise custody group count.
func (tacgc *toAdverstiseCustodyGroupCount) Get() uint64 {
	// If subscribed to all subnets, return the number of custody groups.
	if flags.Get().SubscribeToAllSubnets {
		return params.BeaconConfig().NumberOfCustodyGroups
	}

	custodyRequirement := params.BeaconConfig().CustodyRequirement

	tacgc.mut.RLock()
	defer tacgc.mut.RUnlock()

	return max(tacgc.value, custodyRequirement)
}

// Set sets the to advertise custody group count.
func (tacgc *toAdverstiseCustodyGroupCount) Set(value uint64) {
	tacgc.mut.Lock()
	defer tacgc.mut.Unlock()

	tacgc.value = value
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
