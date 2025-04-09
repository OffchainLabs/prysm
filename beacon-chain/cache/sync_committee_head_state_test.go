package cache_test

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestSyncCommitteeHeadState(t *testing.T) {
	beaconState, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Fork: &ethpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		},
	})
	require.NoError(t, err)
	phase0State, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Fork: &ethpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		},
	})
	require.NoError(t, err)
	bellatrixState, err := state_native.InitializeFromProtoBellatrix(&ethpb.BeaconStateBellatrix{
		Fork: &ethpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		},
	})
	require.NoError(t, err)
	capellaState, err := state_native.InitializeFromProtoCapella(&ethpb.BeaconStateCapella{
		Fork: &ethpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		},
	})
	require.NoError(t, err)
	type put struct {
		slot  primitives.Slot
		state state.BeaconState
	}
	tests := []struct {
		name       string
		key        primitives.Slot
		put        *put
		want       state.BeaconState
		wantErr    bool
		wantPutErr bool
	}{
		{
			name: "putting error in",
			key:  primitives.Slot(1),
			put: &put{
				slot:  primitives.Slot(1),
				state: nil,
			},
			wantPutErr: true,
			wantErr:    true,
		},
		{
			name: "putting invalid state in",
			key:  primitives.Slot(1),
			put: &put{
				slot:  primitives.Slot(1),
				state: phase0State,
			},
			wantPutErr: true,
			wantErr:    true,
		},
		{
			name:    "not found when empty cache",
			key:     primitives.Slot(1),
			wantErr: true,
		},
		{
			name: "not found when non-existent key in non-empty cache",
			key:  primitives.Slot(2),
			put: &put{
				slot:  primitives.Slot(1),
				state: beaconState,
			},
			wantErr: true,
		},
		{
			name: "found with key",
			key:  primitives.Slot(1),
			put: &put{
				slot:  primitives.Slot(1),
				state: beaconState,
			},
			want: beaconState,
		},
		{
			name: "not found when non-existent key in non-empty cache (bellatrix state)",
			key:  primitives.Slot(2),
			put: &put{
				slot:  primitives.Slot(1),
				state: bellatrixState,
			},
			wantErr: true,
		},
		{
			name: "found with key (bellatrix state)",
			key:  primitives.Slot(100),
			put: &put{
				slot:  primitives.Slot(100),
				state: bellatrixState,
			},
			want: bellatrixState,
		},
		{
			name: "found with key (capella state)",
			key:  primitives.Slot(200),
			put: &put{
				slot:  primitives.Slot(200),
				state: capellaState,
			},
			want: capellaState,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cache.NewSyncCommitteeHeadState()
			if tt.put != nil {
				err := c.Put(tt.put.slot, tt.put.state)
				if (err != nil) != tt.wantPutErr {
					t.Fatalf("Put() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
			got, err := c.Get(tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			require.DeepEqual(t, tt.want, got)
		})
	}
}
