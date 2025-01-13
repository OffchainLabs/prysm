package cache

import (
	"errors"
	"sync"

	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

type AttestationConsensusData struct {
	Slot     primitives.Slot
	HeadRoot []byte
	Target   forkchoicetypes.Checkpoint
	Source   forkchoicetypes.Checkpoint
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
