package mock

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	PendingAttSlashings  []ethpb.AttSlashing
	PendingPropSlashings []*ethpb.ProposerSlashing
}

// PendingAttesterSlashings --
func (m *PoolMock) PendingAttesterSlashings(_ context.Context, _ state.ReadOnlyBeaconState, _ bool) []ethpb.AttSlashing {
	return m.PendingAttSlashings
}

// PendingProposerSlashings --
func (m *PoolMock) PendingProposerSlashings(_ context.Context, _ state.ReadOnlyBeaconState, _ bool) []*ethpb.ProposerSlashing {
	return m.PendingPropSlashings
}

// InsertAttesterSlashing --
func (m *PoolMock) InsertAttesterSlashing(_ context.Context, _ state.ReadOnlyBeaconState, slashing ethpb.AttSlashing) error {
	m.PendingAttSlashings = append(m.PendingAttSlashings, slashing)
	return nil
}

// InsertProposerSlashing --
func (m *PoolMock) InsertProposerSlashing(_ context.Context, _ state.ReadOnlyBeaconState, slashing *ethpb.ProposerSlashing) error {
	m.PendingPropSlashings = append(m.PendingPropSlashings, slashing)
	return nil
}

// ConvertToElectra --
func (*PoolMock) ConvertToElectra() {}

// MarkIncludedAttesterSlashing --
func (*PoolMock) MarkIncludedAttesterSlashing(_ ethpb.AttSlashing) {
	panic("implement me")
}

// MarkIncludedProposerSlashing --
func (*PoolMock) MarkIncludedProposerSlashing(_ *ethpb.ProposerSlashing) {
	panic("implement me")
}
