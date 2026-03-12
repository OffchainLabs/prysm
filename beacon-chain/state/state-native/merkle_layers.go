package state_native

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
)

// sharedMerkleLayers wraps the beacon state's top-level Merkle tree layers with
// reference counting so that Copy() can share them instead of deep-copying.
// All access is protected by the owning BeaconState's lock; this struct does
// not carry its own mutex.
type sharedMerkleLayers struct {
	layers [][][]byte
	ref    *stateutil.Reference
}

// newSharedMerkleLayers wraps existing layers in a ref-counted container.
func newSharedMerkleLayers(layers [][][]byte) *sharedMerkleLayers {
	return &sharedMerkleLayers{
		layers: layers,
		ref:    stateutil.NewRef(1),
	}
}

// copy increments the reference count and returns the same pointer, making
// BeaconState.Copy() O(1) for this field. The caller must call ensureUnique()
// before mutating the layers.
func (s *sharedMerkleLayers) copy() *sharedMerkleLayers {
	s.ref.AddRef()
	return s
}

// ensureUnique deep-copies the layers if this instance is shared (refs > 1)
// and returns the (possibly new) sharedMerkleLayers to use. The caller must
// replace its field with the returned value:
//
//	b.merkleLayers = b.merkleLayers.ensureUnique()
func (s *sharedMerkleLayers) ensureUnique() *sharedMerkleLayers {
	if s.ref.Refs() == 1 {
		return s
	}
	// Shared — deep-copy and detach.
	s.ref.MinusRef()
	newLayers := make([][][]byte, len(s.layers))
	for i, layer := range s.layers {
		newLayers[i] = make([][]byte, len(layer))
		for j, content := range layer {
			newLayers[i][j] = make([]byte, len(content))
			copy(newLayers[i][j], content)
		}
	}
	return newSharedMerkleLayers(newLayers)
}

// release decrements the reference count. Called during finalizer cleanup.
func (s *sharedMerkleLayers) release() {
	s.ref.MinusRef()
}
