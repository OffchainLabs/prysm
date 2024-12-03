package mock

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/operations/attestations"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

var _ attestations.Pool = &PoolMock{}

// PoolMock --
type PoolMock struct {
	AggregatedAtts   []ethpb.Att
	UnaggregatedAtts []ethpb.Att
}

// AggregateUnaggregatedAttestations --
func (*PoolMock) AggregateUnaggregatedAttestations(_ context.Context) error {
	panic("implement me")
}

// AggregateUnaggregatedAttestationsBySlotIndex --
func (*PoolMock) AggregateUnaggregatedAttestationsBySlotIndex(_ context.Context, _ primitives.Slot, _ primitives.CommitteeIndex) error {
	panic("implement me")
}

// SaveAggregatedAttestation --
func (*PoolMock) SaveAggregatedAttestation(_ ethpb.Att) error {
	panic("implement me")
}

// SaveAggregatedAttestations --
func (m *PoolMock) SaveAggregatedAttestations(atts []ethpb.Att) error {
	m.AggregatedAtts = append(m.AggregatedAtts, atts...)
	return nil
}

// AggregatedAttestations --
func (m *PoolMock) AggregatedAttestations() []ethpb.Att {
	return m.AggregatedAtts
}

// AggregatedAttestationsBySlotIndex --
func (*PoolMock) AggregatedAttestationsBySlotIndex(_ context.Context, _ primitives.Slot, _ primitives.CommitteeIndex) []*ethpb.Attestation {
	panic("implement me")
}

// AggregatedAttestationsBySlotIndexElectra --
func (*PoolMock) AggregatedAttestationsBySlotIndexElectra(_ context.Context, _ primitives.Slot, _ primitives.CommitteeIndex) []*ethpb.AttestationElectra {
	panic("implement me")
}

// DeleteAggregatedAttestation --
func (*PoolMock) DeleteAggregatedAttestation(_ ethpb.Att) error {
	panic("implement me")
}

// HasAggregatedAttestation --
func (*PoolMock) HasAggregatedAttestation(_ ethpb.Att) (bool, error) {
	panic("implement me")
}

// AggregatedAttestationCount --
func (*PoolMock) AggregatedAttestationCount() int {
	panic("implement me")
}

// SaveUnaggregatedAttestation --
func (*PoolMock) SaveUnaggregatedAttestation(_ ethpb.Att) error {
	panic("implement me")
}

// SaveUnaggregatedAttestations --
func (m *PoolMock) SaveUnaggregatedAttestations(atts []ethpb.Att) error {
	m.UnaggregatedAtts = append(m.UnaggregatedAtts, atts...)
	return nil
}

// UnaggregatedAttestations --
func (m *PoolMock) UnaggregatedAttestations() ([]ethpb.Att, error) {
	return m.UnaggregatedAtts, nil
}

// UnaggregatedAttestationsBySlotIndex --
func (*PoolMock) UnaggregatedAttestationsBySlotIndex(_ context.Context, _ primitives.Slot, _ primitives.CommitteeIndex) []*ethpb.Attestation {
	panic("implement me")
}

// UnaggregatedAttestationsBySlotIndexElectra --
func (*PoolMock) UnaggregatedAttestationsBySlotIndexElectra(_ context.Context, _ primitives.Slot, _ primitives.CommitteeIndex) []*ethpb.AttestationElectra {
	panic("implement me")
}

// DeleteUnaggregatedAttestation --
func (*PoolMock) DeleteUnaggregatedAttestation(_ ethpb.Att) error {
	panic("implement me")
}

// DeleteSeenUnaggregatedAttestations --
func (*PoolMock) DeleteSeenUnaggregatedAttestations() (int, error) {
	panic("implement me")
}

// UnaggregatedAttestationCount --
func (*PoolMock) UnaggregatedAttestationCount() int {
	panic("implement me")
}

// SaveBlockAttestation --
func (*PoolMock) SaveBlockAttestation(_ ethpb.Att) error {
	panic("implement me")
}

// SaveBlockAttestations --
func (*PoolMock) SaveBlockAttestations(_ []ethpb.Att) error {
	panic("implement me")
}

// BlockAttestations --
func (*PoolMock) BlockAttestations() []ethpb.Att {
	panic("implement me")
}

// DeleteBlockAttestation --
func (*PoolMock) DeleteBlockAttestation(_ ethpb.Att) error {
	panic("implement me")
}

// SaveForkchoiceAttestation --
func (*PoolMock) SaveForkchoiceAttestation(_ ethpb.Att) error {
	panic("implement me")
}

// SaveForkchoiceAttestations --
func (*PoolMock) SaveForkchoiceAttestations(_ []ethpb.Att) error {
	panic("implement me")
}

// ForkchoiceAttestations --
func (*PoolMock) ForkchoiceAttestations() []ethpb.Att {
	panic("implement me")
}

// DeleteForkchoiceAttestation --
func (*PoolMock) DeleteForkchoiceAttestation(_ ethpb.Att) error {
	panic("implement me")
}

// ForkchoiceAttestationCount --
func (*PoolMock) ForkchoiceAttestationCount() int {
	panic("implement me")
}
