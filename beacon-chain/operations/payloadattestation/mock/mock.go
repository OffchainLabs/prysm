package mock

import (
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	Attestations []*ethpb.PayloadAttestation
}

// PendingPayloadAttestations --
func (m *PoolMock) PendingPayloadAttestations() []*ethpb.PayloadAttestation {
	return m.Attestations
}

// InsertPayloadAttestation --
func (m *PoolMock) InsertPayloadAttestation(msg *ethpb.PayloadAttestationMessage) error {
	m.Attestations = append(m.Attestations, &ethpb.PayloadAttestation{
		Data:      msg.Data,
		Signature: msg.Signature,
	})
	return nil
}

// MarkIncluded --
func (*PoolMock) MarkIncluded(_ *ethpb.PayloadAttestation) {
}
