package mock

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	ExecutionProofs []*eth.ExecutionProof
}

// PendingExecProofChanges --
func (m *PoolMock) PendingExecProofChanges() ([]*eth.ExecutionProof, error) {
	return m.ExecutionProofs, nil
}

// ExecProofForInclusion --
func (m *PoolMock) ExecProofForInclusion(_ state.ReadOnlyBeaconState) ([]*eth.ExecutionProof, error) {
	return m.ExecutionProofs, nil
}

// InsertExecProofChange --
func (m *PoolMock) InsertExecProofChange(change *eth.ExecutionProof) {
	m.ExecutionProofs = append(m.ExecutionProofs, change)
}

// MarkIncluded --
func (*PoolMock) MarkIncluded(_ *eth.ExecutionProof) {
	panic("implement me") // lint:nopanic -- mock / test code.
}

// ValidatorExists --
func (*PoolMock) ValidatorExists(_ primitives.ValidatorIndex) bool {
	panic("implement me") // lint:nopanic -- mock / test code.
}
