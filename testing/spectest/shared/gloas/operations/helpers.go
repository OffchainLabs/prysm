package operations

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func sszToState(b []byte) (state.BeaconState, error) {
	base := &ethpb.BeaconStateGloas{}
	if err := base.UnmarshalSSZ(b); err != nil {
		return nil, err
	}
	return state_native.InitializeFromProtoGloas(base)
}
