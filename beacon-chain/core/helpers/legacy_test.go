package helpers_test

import (
	"math"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestIsLegacyDepositProcessPeriod(t *testing.T) {
	tests := []struct {
		name              string
		state             state.BeaconState
		canonicalEth1Data *ethpb.Eth1Data
		want              bool
	}{
		{
			name: "pre-electra",
			state: func() state.BeaconState {
				st, err := state_native.InitializeFromProtoDeneb(&ethpb.BeaconStateDeneb{
					Eth1Data: &ethpb.Eth1Data{
						BlockHash:    []byte("0x0"),
						DepositRoot:  make([]byte, 32),
						DepositCount: 5,
					},
					Eth1DepositIndex: 1,
				})
				require.NoError(t, err)
				return st
			}(),
			canonicalEth1Data: &ethpb.Eth1Data{
				BlockHash:    []byte("0x0"),
				DepositRoot:  make([]byte, 32),
				DepositCount: 5,
			},
			want: true,
		},
		{
			name: "post-electra, pending deposits from pre-electra",
			state: func() state.BeaconState {
				st, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{
					Eth1Data: &ethpb.Eth1Data{
						BlockHash:    []byte("0x0"),
						DepositRoot:  make([]byte, 32),
						DepositCount: 5,
					},
					DepositRequestsStartIndex: math.MaxUint64,
					Eth1DepositIndex:          1,
				})
				require.NoError(t, err)
				return st
			}(),
			canonicalEth1Data: &ethpb.Eth1Data{
				BlockHash:    []byte("0x0"),
				DepositRoot:  make([]byte, 32),
				DepositCount: 5,
			},
			want: true,
		},
		{
			name: "post-electra, no pending deposits from pre-alpaca",
			state: func() state.BeaconState {
				st, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{
					Eth1Data: &ethpb.Eth1Data{
						BlockHash:    []byte("0x0"),
						DepositRoot:  make([]byte, 32),
						DepositCount: 5,
					},
					DepositRequestsStartIndex: 1,
					Eth1DepositIndex:          5,
				})
				require.NoError(t, err)
				return st
			}(),
			canonicalEth1Data: &ethpb.Eth1Data{
				BlockHash:    []byte("0x0"),
				DepositRoot:  make([]byte, 32),
				DepositCount: 5,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := helpers.IsLegacyDepositProcessPeriod(tt.state, tt.canonicalEth1Data); got != tt.want {
				t.Errorf("isLegacyDepositProcessPeriod() = %v, want %v", got, tt.want)
			}
		})
	}
}
