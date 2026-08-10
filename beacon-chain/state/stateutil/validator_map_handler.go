package stateutil

import (
	"sync"

	coreutils "github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition/stateutils"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// ValidatorMapHandler holds the pubkey to index map shared by every state copied from a common ancestor.
type ValidatorMapHandler struct {
	valIdxMap map[[fieldparams.BLSPubkeyLength]byte]primitives.ValidatorIndex
	*sync.RWMutex
}

// NewValMapHandler returns a new validator map handler.
func NewValMapHandler(vals []*ethpb.Validator) *ValidatorMapHandler {
	return &ValidatorMapHandler{
		valIdxMap: coreutils.ValidatorIndexMap(vals),
		RWMutex:   new(sync.RWMutex),
	}
}

// IsNil returns true if the underlying validator index map is nil.
func (v *ValidatorMapHandler) IsNil() bool {
	return v == nil || v.valIdxMap == nil
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
