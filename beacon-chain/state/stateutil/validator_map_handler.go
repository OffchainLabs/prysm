package stateutil

import (
	"sync"

	coreutils "github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition/stateutils"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// ValidatorMapHandler is a container to hold the map and a reference tracker for how many
// states shared this.
type ValidatorMapHandler struct {
	valIdxMap map[[fieldparams.BLSPubkeyLength]byte]primitives.ValidatorIndex
	mapRef    *Reference
	*sync.RWMutex
}

// NewValMapHandler returns a new validator map handler.
func NewValMapHandler(vals []*ethpb.Validator) *ValidatorMapHandler {
	return &ValidatorMapHandler{
		valIdxMap: coreutils.ValidatorIndexMap(vals),
		mapRef:    &Reference{refs: 1},
		RWMutex:   new(sync.RWMutex),
	}
}

// AddRef copies the whole map and returns a map handler with the copied map.
func (v *ValidatorMapHandler) AddRef() {
	v.mapRef.AddRef()
}

// IsNil returns true if the underlying validator index map is nil.
func (v *ValidatorMapHandler) IsNil() bool {
	return v.mapRef == nil || v.valIdxMap == nil
}

// Len returns the number of entries in the validator index map.
func (v *ValidatorMapHandler) Len() int {
	v.RLock()
	defer v.RUnlock()
	return len(v.valIdxMap)
}

// Get the validator index using the corresponding public key.
func (v *ValidatorMapHandler) Get(key [fieldparams.BLSPubkeyLength]byte) (primitives.ValidatorIndex, bool) {
	v.RLock()
	defer v.RUnlock()
	idx, ok := v.valIdxMap[key]
	if !ok {
		return 0, false
	}
	return idx, true
}

// Set the validator index using the corresponding public key.
func (v *ValidatorMapHandler) Set(key [fieldparams.BLSPubkeyLength]byte, index primitives.ValidatorIndex) {
	v.Lock()
	defer v.Unlock()
	v.valIdxMap[key] = index
}

var (
	globalHandler   *ValidatorMapHandler
	globalHandlerMu sync.Mutex
)

// GlobalValMapHandler returns the global ValidatorMapHandler, extending it
// if the provided validator list has more entries than currently cached.
// Each state filters lookups via a bounds check (index < validator count),
// so sharing a superset map across all states is safe.
func GlobalValMapHandler(vals []*ethpb.Validator) *ValidatorMapHandler {
	if len(vals) == 0 {
		return NewValMapHandler(vals)
	}

	globalHandlerMu.Lock()
	defer globalHandlerMu.Unlock()

	if globalHandler == nil {
		globalHandler = NewValMapHandler(vals)
		return globalHandler
	}

	cachedLen := globalHandler.Len()
	numVals := len(vals)

	if numVals > cachedLen {
		for i := cachedLen; i < numVals; i++ {
			if vals[i] == nil {
				continue
			}
			globalHandler.Set(
				bytesutil.ToBytes48(vals[i].PublicKey),
				primitives.ValidatorIndex(i),
			)
		}
	}

	return globalHandler
}

// ResetGlobalValMapHandler clears the global validator map handler.
// This is intended for use in tests only.
func ResetGlobalValMapHandler() {
	globalHandlerMu.Lock()
	defer globalHandlerMu.Unlock()
	globalHandler = nil
}
