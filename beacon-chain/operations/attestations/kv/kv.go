// Package kv includes a key-value store implementation
// of an attestation cache used to satisfy important use-cases
// such as aggregation in a beacon node runtime.
package kv

import (
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/operations/attestations/attmap"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
)

// AttCaches defines the caches used to satisfy attestation pool interface.
// These caches are KV store for various attestations
// such are unaggregated, aggregated or attestations within a block.
type AttCaches struct {
	aggregatedAttLock  sync.RWMutex
	aggregatedAtt      map[attestation.Id][]ethpb.Att
	unAggregateAttLock sync.RWMutex
	unAggregatedAtt    map[attestation.Id]ethpb.Att
	forkchoiceAtt      *attmap.Attestations
	blockAttLock       sync.RWMutex
	blockAtt           map[attestation.Id][]ethpb.Att
	seenAtt            *cache.Cache
}

// NewAttCaches initializes a new attestation pool consists of multiple KV store in cache for
// various kind of attestations.
func NewAttCaches() *AttCaches {
	secsInEpoch := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	c := cache.New(2*secsInEpoch*time.Second, 2*secsInEpoch*time.Second)
	pool := &AttCaches{
		unAggregatedAtt: make(map[attestation.Id]ethpb.Att),
		aggregatedAtt:   make(map[attestation.Id][]ethpb.Att),
		forkchoiceAtt:   attmap.New(),
		blockAtt:        make(map[attestation.Id][]ethpb.Att),
		seenAtt:         c,
	}

	return pool
}

// saveForkchoiceAttestation saves a forkchoice attestation.
func (c *AttCaches) saveForkchoiceAttestation(att ethpb.Att) error {
	return c.forkchoiceAtt.Save(att)
}

// SaveForkchoiceAttestations saves forkchoice attestations.
func (c *AttCaches) SaveForkchoiceAttestations(att []ethpb.Att) error {
	return c.forkchoiceAtt.SaveMany(att)
}

// ForkchoiceAttestations returns all forkchoice attestations.
func (c *AttCaches) ForkchoiceAttestations() []ethpb.Att {
	return c.forkchoiceAtt.GetAll()
}

// DeleteForkchoiceAttestation deletes a forkchoice attestation.
func (c *AttCaches) DeleteForkchoiceAttestation(att ethpb.Att) error {
	return c.forkchoiceAtt.Delete(att)
}

// ForkchoiceAttestationCount returns the number of forkchoice attestation keys.
func (c *AttCaches) ForkchoiceAttestationCount() int {
	return c.forkchoiceAtt.Count()
}
