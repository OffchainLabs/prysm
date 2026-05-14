package stateutil

import (
	"maps"
	"sync"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// BuilderMapHandler holds a pubkey -> builder index lookup map alongside a
// reference counter so the underlying map can be shared across state copies.
//
// Unlike validators, builder indices are reusable: when a builder exits and
// its balance reaches zero, its slot can be reassigned to a new builder with
// a different pubkey. Callers that mutate state.builders must update this
// handler accordingly (Remove the displaced pubkey, Set the new one).
type BuilderMapHandler struct {
	idxMap map[[fieldparams.BLSPubkeyLength]byte]uint64
	mapRef *Reference
	*sync.RWMutex
}

// NewBuilderMapHandler builds a handler from a builders slice.
func NewBuilderMapHandler(builders []*ethpb.Builder) *BuilderMapHandler {
	m := make(map[[fieldparams.BLSPubkeyLength]byte]uint64, len(builders))
	for idx, builder := range builders {
		if builder == nil || len(builder.Pubkey) != fieldparams.BLSPubkeyLength {
			continue
		}
		m[bytesutil.ToBytes48(builder.Pubkey)] = uint64(idx)
	}
	return &BuilderMapHandler{
		idxMap:  m,
		mapRef:  &Reference{refs: 1},
		RWMutex: new(sync.RWMutex),
	}
}

// AddRef bumps the reference counter so the map can be shared by a state copy
// without an immediate clone.
func (h *BuilderMapHandler) AddRef() {
	h.mapRef.AddRef()
}

// IsNil returns true if the underlying map is uninitialized.
func (h *BuilderMapHandler) IsNil() bool {
	return h == nil || h.mapRef == nil || h.idxMap == nil
}

// Get returns the index for the given pubkey if present.
func (h *BuilderMapHandler) Get(key [fieldparams.BLSPubkeyLength]byte) (uint64, bool) {
	h.RLock()
	defer h.RUnlock()
	idx, ok := h.idxMap[key]
	return idx, ok
}

// Set inserts pubkey -> index. Caller must hold the state lock.
func (h *BuilderMapHandler) Set(key [fieldparams.BLSPubkeyLength]byte, index uint64) {
	h.Lock()
	defer h.Unlock()
	h.idxMap[key] = index
}

// Remove deletes the entry for pubkey. No-op if missing.
func (h *BuilderMapHandler) Remove(key [fieldparams.BLSPubkeyLength]byte) {
	h.Lock()
	defer h.Unlock()
	delete(h.idxMap, key)
}

// Copy returns a fresh handler with a cloned underlying map. Use when the
// caller intends to mutate state.builders and the current handler is shared
// (mapRef.Refs() > 1).
func (h *BuilderMapHandler) Copy() *BuilderMapHandler {
	h.RLock()
	defer h.RUnlock()
	m := make(map[[fieldparams.BLSPubkeyLength]byte]uint64, len(h.idxMap))
	maps.Copy(m, h.idxMap)
	return &BuilderMapHandler{
		idxMap:  m,
		mapRef:  &Reference{refs: 1},
		RWMutex: new(sync.RWMutex),
	}
}

// Refs returns the current reference count.
func (h *BuilderMapHandler) Refs() uint {
	return h.mapRef.Refs()
}

// MinusRef decrements the reference count.
func (h *BuilderMapHandler) MinusRef() {
	h.mapRef.MinusRef()
}
