package state_native

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSharedFieldRefCount(t *testing.T) {
	tests := []struct {
		name string
		init func() (state.BeaconState, error)
		want int
	}{
		{"phase0", func() (state.BeaconState, error) { return InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{}) }, phase0SharedFieldRefCount},
		{"altair", func() (state.BeaconState, error) { return InitializeFromProtoUnsafeAltair(&ethpb.BeaconStateAltair{}) }, altairSharedFieldRefCount},
		{"bellatrix", func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeBellatrix(&ethpb.BeaconStateBellatrix{})
		}, bellatrixSharedFieldRefCount},
		{"capella", func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeCapella(&ethpb.BeaconStateCapella{})
		}, capellaSharedFieldRefCount},
		{"deneb", func() (state.BeaconState, error) { return InitializeFromProtoUnsafeDeneb(&ethpb.BeaconStateDeneb{}) }, denebSharedFieldRefCount},
		{"electra", func() (state.BeaconState, error) {
			return InitializeFromProtoUnsafeElectra(&ethpb.BeaconStateElectra{})
		}, electraSharedFieldRefCount},
		{"fulu", func() (state.BeaconState, error) { return InitializeFromProtoUnsafeFulu(&ethpb.BeaconStateFulu{}) }, fuluSharedFieldRefCount},
		{"gloas", func() (state.BeaconState, error) { return InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{}) }, gloasSharedFieldRefCount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := tt.init()
			require.NoError(t, err)
			b, ok := st.(*BeaconState)
			require.Equal(t, true, ok)
			require.Equal(t, tt.want, len(b.sharedFieldReferences))

			cp, ok := b.Copy().(*BeaconState)
			require.Equal(t, true, ok)
			require.Equal(t, tt.want, len(cp.sharedFieldReferences))
		})
	}
}
