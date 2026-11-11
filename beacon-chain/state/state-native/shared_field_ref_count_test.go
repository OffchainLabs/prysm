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
		{"phase0", func() (state.BeaconState, error) { return NewUnsafe(&ethpb.BeaconState{}) }, phase0SharedFieldRefCount},
		{"altair", func() (state.BeaconState, error) { return NewUnsafe(&ethpb.BeaconStateAltair{}) }, altairSharedFieldRefCount},
		{"bellatrix", func() (state.BeaconState, error) {
			return NewUnsafe(&ethpb.BeaconStateBellatrix{})
		}, bellatrixSharedFieldRefCount},
		{"capella", func() (state.BeaconState, error) {
			return NewUnsafe(&ethpb.BeaconStateCapella{})
		}, capellaSharedFieldRefCount},
		{"deneb", func() (state.BeaconState, error) { return NewUnsafe(&ethpb.BeaconStateDeneb{}) }, denebSharedFieldRefCount},
		{"electra", func() (state.BeaconState, error) {
			return NewUnsafe(&ethpb.BeaconStateElectra{})
		}, electraSharedFieldRefCount},
		{"fulu", func() (state.BeaconState, error) { return NewUnsafe(&ethpb.BeaconStateFulu{}) }, fuluSharedFieldRefCount},
		{"gloas", func() (state.BeaconState, error) { return NewUnsafe(&ethpb.BeaconStateGloas{}) }, gloasSharedFieldRefCount},
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
