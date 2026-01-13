package testutil

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// MockStater is a fake implementation of lookup.Stater.
type MockStater struct {
	BeaconState       state.BeaconState
	StateProviderFunc func(ctx context.Context, stateId []byte) (state.BeaconState, error)
	BeaconStateRoot   []byte
	StatesBySlot      map[primitives.Slot]state.BeaconState
	StatesByEpoch     map[primitives.Epoch]state.BeaconState
	StatesByRoot      map[[32]byte]state.BeaconState
	CustomError       error
}

// State --
func (m *MockStater) State(ctx context.Context, id []byte) (state.BeaconState, error) {
	if m.CustomError != nil {
		return nil, m.CustomError
	}
	if m.StateProviderFunc != nil {
		return m.StateProviderFunc(ctx, id)
	}

	if m.BeaconState != nil {
		return m.BeaconState, nil
	}

	return m.StatesByRoot[bytesutil.ToBytes32(id)], nil
}

// StateRoot --
func (m *MockStater) StateRoot(context.Context, []byte) ([]byte, error) {
	return m.BeaconStateRoot, nil
}

// StateBySlot --
func (m *MockStater) StateBySlot(_ context.Context, s primitives.Slot) (state.BeaconState, error) {
	return m.StatesBySlot[s], nil
}

// StateByEpoch --
func (m *MockStater) StateByEpoch(_ context.Context, e primitives.Epoch) (state.BeaconState, error) {
	if m.CustomError != nil {
		return nil, m.CustomError
	}
	if m.StatesByEpoch != nil {
		return m.StatesByEpoch[e], nil
	}
	// Fall back to StatesBySlot if StatesByEpoch is not set
	slot, err := slots.EpochStart(e)
	if err != nil {
		return nil, err
	}
	if m.StatesBySlot != nil {
		return m.StatesBySlot[slot], nil
	}
	return m.BeaconState, nil
}
