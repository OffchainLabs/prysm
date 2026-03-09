package mock

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	Attestations []*ethpb.PayloadAttestation
}

// PendingPayloadAttestations --
func (m *PoolMock) PendingPayloadAttestations(_ primitives.Slot) []*ethpb.PayloadAttestation {
	return m.Attestations
}

// InsertPayloadAttestation --
func (m *PoolMock) InsertPayloadAttestation(msg *ethpb.PayloadAttestationMessage, _ uint64) error {
	m.Attestations = append(m.Attestations, &ethpb.PayloadAttestation{
		Data:      msg.Data,
		Signature: msg.Signature,
	})
	return nil
}

// Seen --
func (*PoolMock) Seen(_ *ethpb.PayloadAttestationData, _ uint64) bool {
	return false
}

// MarkIncluded --
func (*PoolMock) MarkIncluded(_ *ethpb.PayloadAttestation) {
}
