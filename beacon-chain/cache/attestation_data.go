package cache

import (
	"errors"
	"sync"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

type AttestationConsensusData struct {
	Slot          primitives.Slot
	HeadRoot      []byte
	Target        forkchoicetypes.Checkpoint
	Source        forkchoicetypes.Checkpoint
	HeadFull      bool // payload status of the head this data was produced from, part of the freshness key
	IsPayloadFull bool // payload status the attestation votes for
}

// IsFreshFor reports whether a holds attestation data produced for slot from the
// head identified by headRoot and headFull.
func (a *AttestationConsensusData) IsFreshFor(slot primitives.Slot, headRoot [32]byte, headFull bool) bool {
	return a != nil && a.Slot == slot && a.HeadFull == headFull && bytesutil.ToBytes32(a.HeadRoot) == headRoot
}

// AttestationDataCache stores cached results of AttestationData requests.
type AttestationDataCache struct {
	a *AttestationConsensusData
	sync.RWMutex
}

// NewAttestationDataCache creates a new instance of AttestationDataCache.
func NewAttestationDataCache() *AttestationDataCache {
	return &AttestationDataCache{}
}

// Get retrieves cached attestation data, recording a cache hit or miss. This method is lock free.
func (c *AttestationDataCache) Get() *AttestationConsensusData {
	return c.a
}

// Put adds a response to the cache. This method is lock free.
func (c *AttestationDataCache) Put(a *AttestationConsensusData) error {
	if a == nil {
		return errors.New("attestation cannot be nil")
	}
	c.a = a
	return nil
}
