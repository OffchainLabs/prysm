package kv

import (
	"github.com/pkg/errors"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
)

// SaveBlockAttestation saves an block attestation in cache.
func (c *AttCaches) SaveBlockAttestation(att ethpb.Att) error {
	if att == nil || att.IsNil() {
		return nil
	}

	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}

	c.blockAttLock.Lock()
	defer c.blockAttLock.Unlock()
	atts, ok := c.blockAtt[id]
	if !ok {
		atts = make([]ethpb.Att, 0, 1)
	}

	// Ensure that this attestation is not already fully contained in an existing attestation.
	for _, a := range atts {
		if c, err := a.GetAggregationBits().Contains(att.GetAggregationBits()); err != nil {
			return err
		} else if c {
			return nil
		}
	}

	c.blockAtt[id] = append(atts, att.Clone())

	return nil
}

// BlockAttestations returns the block attestations in cache.
func (c *AttCaches) BlockAttestations() []ethpb.Att {
	atts := make([]ethpb.Att, 0)

	c.blockAttLock.RLock()
	defer c.blockAttLock.RUnlock()
	for _, att := range c.blockAtt {
		atts = append(atts, att...)
	}

	return atts
}

// DeleteBlockAttestation deletes a block attestation in cache.
func (c *AttCaches) DeleteBlockAttestation(att ethpb.Att) error {
	if att == nil || att.IsNil() {
		return nil
	}
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}

	c.blockAttLock.Lock()
	defer c.blockAttLock.Unlock()
	delete(c.blockAtt, id)

	return nil
}
