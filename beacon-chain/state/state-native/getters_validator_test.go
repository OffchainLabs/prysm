package state_native_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	statenative "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	testtmpl "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/testing"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestBeaconState_ValidatorAtIndexReadOnly_HandlesNilSlice_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t, func() (state.BeaconState, error) {
		return statenative.InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{
			Validators: nil,
		})
	})
}

func TestBeaconState_ValidatorAtIndexReadOnly_HandlesNilSlice_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t, func() (state.BeaconState, error) {
		return statenative.InitializeFromProtoUnsafeAltair(&ethpb.BeaconStateAltair{
			Validators: nil,
		})
	})
}

func TestBeaconState_ValidatorAtIndexReadOnly_HandlesNilSlice_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t, func() (state.BeaconState, error) {
		return statenative.InitializeFromProtoUnsafeBellatrix(&ethpb.BeaconStateBellatrix{
			Validators: nil,
		})
	})
}

func TestBeaconState_ValidatorAtIndexReadOnly_HandlesNilSlice_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t, func() (state.BeaconState, error) {
		return statenative.InitializeFromProtoUnsafeCapella(&ethpb.BeaconStateCapella{
			Validators: nil,
		})
	})
}

func TestBeaconState_ValidatorAtIndexReadOnly_HandlesNilSlice_Deneb(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t, func() (state.BeaconState, error) {
		return statenative.InitializeFromProtoUnsafeDeneb(&ethpb.BeaconStateDeneb{
			Validators: nil,
		})
	})
}

func TestValidatorIndexes(t *testing.T) {
	dState, _ := util.DeterministicGenesisState(t, 10)
	byteValue := dState.PubkeyAtIndex(1)
	t.Run("ValidatorIndexByPubkey", func(t *testing.T) {
		require.Equal(t, hexutil.Encode(byteValue[:]), "0xb89bebc699769726a318c8e9971bd3171297c61aea4a6578a7a4f94b547dcba5bac16a89108b6b6a1fe3695d1a874a0b")
	})
	t.Run("ValidatorAtIndexReadOnly", func(t *testing.T) {
		readOnlyState, err := dState.ValidatorAtIndexReadOnly(1)
		require.NoError(t, err)
		readOnlyBytes := readOnlyState.PublicKey()
		require.NotEmpty(t, readOnlyBytes)
		require.Equal(t, hexutil.Encode(readOnlyBytes[:]), hexutil.Encode(byteValue[:]))
	})
}

func TestPendingBalanceToWithdraw(t *testing.T) {
	pb := &ethpb.BeaconStateElectra{
		PendingPartialWithdrawals: []*ethpb.PendingPartialWithdrawal{
			{
				Amount: 100,
			},
			{
				Amount: 200,
			},
			{
				Amount: 300,
			},
		},
	}
	state, err := statenative.InitializeFromProtoUnsafeElectra(pb)
	require.NoError(t, err)

	ab, err := state.PendingBalanceToWithdraw(0)
	require.NoError(t, err)
	require.Equal(t, uint64(600), ab)
}

func TestAggregateKeyFromIndices(t *testing.T) {
	dState, _ := util.DeterministicGenesisState(t, 10)
	pKey1 := dState.PubkeyAtIndex(3)
	pKey2 := dState.PubkeyAtIndex(7)
	pKey3 := dState.PubkeyAtIndex(9)

	aggKey, err := bls.AggregatePublicKeys([][]byte{pKey1[:], pKey2[:], pKey3[:]})
	require.NoError(t, err)

	retKey, err := dState.AggregateKeyFromIndices([]uint64{3, 7, 9})
	require.NoError(t, err)

	assert.Equal(t, true, aggKey.Equals(retKey), "unequal aggregated keys")
}

func TestHasPendingBalanceToWithdraw(t *testing.T) {
	pb := &ethpb.BeaconStateElectra{
		PendingPartialWithdrawals: []*ethpb.PendingPartialWithdrawal{
			{
				Amount: 100,
				Index:  1,
			},
			{
				Amount: 200,
				Index:  2,
			},
			{
				Amount: 300,
				Index:  3,
			},
		},
	}
	state, err := statenative.InitializeFromProtoUnsafeElectra(pb)
	require.NoError(t, err)

	ok, err := state.HasPendingBalanceToWithdraw(1)
	require.NoError(t, err)
	require.Equal(t, true, ok)

	ok, err = state.HasPendingBalanceToWithdraw(5)
	require.NoError(t, err)
	require.Equal(t, false, ok)
}
