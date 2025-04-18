package mock

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	Exits []*eth.SignedVoluntaryExit
}

// PendingExits --
func (m *PoolMock) PendingExits() ([]*eth.SignedVoluntaryExit, error) {
	return m.Exits, nil
}

// ExitsForInclusion --
func (m *PoolMock) ExitsForInclusion(_ state.ReadOnlyBeaconState, _ primitives.Slot) ([]*eth.SignedVoluntaryExit, error) {
	return m.Exits, nil
}

// InsertVoluntaryExit --
func (m *PoolMock) InsertVoluntaryExit(exit *eth.SignedVoluntaryExit) {
	m.Exits = append(m.Exits, exit)
}

// MarkIncluded --
func (*PoolMock) MarkIncluded(_ *eth.SignedVoluntaryExit) {
	panic("implement me") // lint:nopanic -- Mock / test code.
}
