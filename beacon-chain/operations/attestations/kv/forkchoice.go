package kv

import (
	"github.com/pkg/errors"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
)

// SaveForkchoiceAttestation saves an forkchoice attestation in cache.
func (c *AttCaches) SaveForkchoiceAttestation(att ethpb.Att) error {
	if att == nil || att.IsNil() {
		return nil
	}

	id, err := attestation.NewId(att, attestation.Full)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}

	c.forkchoiceAttLock.Lock()
	defer c.forkchoiceAttLock.Unlock()
	c.forkchoiceAtt[id] = att

	return nil
}

// SaveForkchoiceAttestations saves a list of forkchoice attestations in cache.
func (c *AttCaches) SaveForkchoiceAttestations(atts []ethpb.Att) error {
	for _, att := range atts {
		if err := c.SaveForkchoiceAttestation(att); err != nil {
			return err
		}
	}

	return nil
}

// ForkchoiceAttestations returns the forkchoice attestations in cache.
func (c *AttCaches) ForkchoiceAttestations() []ethpb.Att {
	c.forkchoiceAttLock.RLock()
	defer c.forkchoiceAttLock.RUnlock()

	atts := make([]ethpb.Att, 0, len(c.forkchoiceAtt))
	for _, att := range c.forkchoiceAtt {
		atts = append(atts, att.Clone())
	}

	return atts
}

// DeleteForkchoiceAttestation deletes a forkchoice attestation in cache.
func (c *AttCaches) DeleteForkchoiceAttestation(att ethpb.Att) error {
	if att == nil || att.IsNil() {
		return nil
	}

	id, err := attestation.NewId(att, attestation.Full)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}

	c.forkchoiceAttLock.Lock()
	defer c.forkchoiceAttLock.Unlock()
	delete(c.forkchoiceAtt, id)

	return nil
}

// ForkchoiceAttestationCount returns the number of fork choice attestations key in the pool.
func (c *AttCaches) ForkchoiceAttestationCount() int {
	c.forkchoiceAttLock.RLock()
	defer c.forkchoiceAttLock.RUnlock()
	return len(c.forkchoiceAtt)
}
