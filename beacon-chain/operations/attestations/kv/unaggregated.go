package kv

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// SaveUnaggregatedAttestation saves an unaggregated attestation in cache.
func (c *AttCaches) SaveUnaggregatedAttestation(att ethpb.Att) error {
	if att == nil || att.IsNil() {
		return nil
	}
	if att.IsAggregated() {
		return errors.New("attestation is aggregated")
	}

	seen, err := c.hasSeenBit(att)
	if err != nil {
		return err
	}
	if seen {
		return nil
	}

	id, err := attestation.NewId(att, attestation.Full)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}

	c.unAggregateAttLock.Lock()
	defer c.unAggregateAttLock.Unlock()
	c.unAggregatedAtt[id] = att

	return nil
}

// SaveUnaggregatedAttestations saves a list of unaggregated attestations in cache.
func (c *AttCaches) SaveUnaggregatedAttestations(atts []ethpb.Att) error {
	for _, att := range atts {
		if err := c.SaveUnaggregatedAttestation(att); err != nil {
			return err
		}
	}

	return nil
}

// UnaggregatedAttestations returns all the unaggregated attestations in cache.
func (c *AttCaches) UnaggregatedAttestations() ([]ethpb.Att, error) {
	c.unAggregateAttLock.RLock()
	defer c.unAggregateAttLock.RUnlock()
	unAggregatedAtts := c.unAggregatedAtt
	atts := make([]ethpb.Att, 0, len(unAggregatedAtts))
	for _, att := range unAggregatedAtts {
		seen, err := c.hasSeenBit(att)
		if err != nil {
			return nil, err
		}
		if !seen {
			atts = append(atts, att.Clone())
		}
	}
	return atts, nil
}

// UnaggregatedAttestationsBySlotIndex returns the unaggregated attestations in cache,
// filtered by committee index and slot.
func (c *AttCaches) UnaggregatedAttestationsBySlotIndex(
	ctx context.Context,
	slot primitives.Slot,
	committeeIndex primitives.CommitteeIndex,
) []*ethpb.Attestation {
	_, span := trace.StartSpan(ctx, "operations.attestations.kv.UnaggregatedAttestationsBySlotIndex")
	defer span.End()

	atts := make([]*ethpb.Attestation, 0)

	c.unAggregateAttLock.RLock()
	defer c.unAggregateAttLock.RUnlock()

	unAggregatedAtts := c.unAggregatedAtt
	for _, a := range unAggregatedAtts {
		if a.Version() == version.Phase0 && slot == a.GetData().Slot && committeeIndex == a.GetData().CommitteeIndex {
			att, ok := a.(*ethpb.Attestation)
			// This will never fail in practice because we asserted the version
			if ok {
				atts = append(atts, att)
			}
		}
	}

	return atts
}

// UnaggregatedAttestationsBySlotIndexElectra returns the unaggregated attestations in cache,
// filtered by committee index and slot.
func (c *AttCaches) UnaggregatedAttestationsBySlotIndexElectra(
	ctx context.Context,
	slot primitives.Slot,
	committeeIndex primitives.CommitteeIndex,
) []*ethpb.AttestationElectra {
	_, span := trace.StartSpan(ctx, "operations.attestations.kv.UnaggregatedAttestationsBySlotIndexElectra")
	defer span.End()

	atts := make([]*ethpb.AttestationElectra, 0)

	c.unAggregateAttLock.RLock()
	defer c.unAggregateAttLock.RUnlock()

	unAggregatedAtts := c.unAggregatedAtt
	for _, a := range unAggregatedAtts {
		if a.Version() == version.Electra && slot == a.GetData().Slot && a.CommitteeBitsVal().BitAt(uint64(committeeIndex)) {
			att, ok := a.(*ethpb.AttestationElectra)
			// This will never fail in practice because we asserted the version
			if ok {
				atts = append(atts, att)
			}
		}
	}

	return atts
}

// DeleteUnaggregatedAttestation deletes the unaggregated attestations in cache.
func (c *AttCaches) DeleteUnaggregatedAttestation(att ethpb.Att) error {
	if att == nil || att.IsNil() {
		return nil
	}
	if att.IsAggregated() {
		return errors.New("attestation is aggregated")
	}

	if err := c.insertSeenBit(att); err != nil {
		return err
	}

	id, err := attestation.NewId(att, attestation.Full)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}

	c.unAggregateAttLock.Lock()
	defer c.unAggregateAttLock.Unlock()
	delete(c.unAggregatedAtt, id)

	return nil
}

// DeleteSeenUnaggregatedAttestations deletes the unaggregated attestations in cache
// that have been already processed once. Returns number of attestations deleted.
func (c *AttCaches) DeleteSeenUnaggregatedAttestations() (int, error) {
	c.unAggregateAttLock.Lock()
	defer c.unAggregateAttLock.Unlock()

	count := 0
	for r, att := range c.unAggregatedAtt {
		if att == nil || att.IsNil() || att.IsAggregated() {
			continue
		}
		if seen, err := c.hasSeenBit(att); err == nil && seen {
			delete(c.unAggregatedAtt, r)
			count++
		}
	}
	return count, nil
}

// UnaggregatedAttestationCount returns the number of unaggregated attestations key in the pool.
func (c *AttCaches) UnaggregatedAttestationCount() int {
	c.unAggregateAttLock.RLock()
	defer c.unAggregateAttLock.RUnlock()
	return len(c.unAggregatedAtt)
}
