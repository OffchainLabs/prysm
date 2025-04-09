package state_native_test

import (
	"testing"

	"github.com/golang/snappy"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func TestExitEpochAndUpdateChurn_SpectestCase(t *testing.T) {
	// Load a serialized Electra state from disk.
	// The spec tests shows that the exit epoch is 262 for validator 0 performing a voluntary exit.
	serializedBytes, err := util.BazelFileBytes("tests/mainnet/electra/operations/voluntary_exit/pyspec_tests/exit_existing_churn_and_churn_limit_balance/pre.ssz_snappy")
	require.NoError(t, err)
	serializedSSZ, err := snappy.Decode(nil /* dst */, serializedBytes)
	require.NoError(t, err)
	pb := &eth.BeaconStateElectra{}
	require.NoError(t, pb.UnmarshalSSZ(serializedSSZ))
	s, err := state_native.InitializeFromProtoElectra(pb)
	require.NoError(t, err)

	val, err := s.ValidatorAtIndex(0)
	require.NoError(t, err)

	ee, err := s.ExitEpochAndUpdateChurn(primitives.Gwei(val.EffectiveBalance))
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(262), ee)

	p := s.ToProto()
	pb, ok := p.(*eth.BeaconStateElectra)
	if !ok {
		t.Fatal("wrong proto")
	}
	require.Equal(t, primitives.Gwei(127000000000), pb.ExitBalanceToConsume)
	require.Equal(t, primitives.Epoch(262), pb.EarliestExitEpoch)

	// Fails for versions older than electra
	s, err = state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)
	_, err = s.ExitEpochAndUpdateChurn(10)
	require.ErrorContains(t, "not supported", err)
}

func TestExitEpochAndUpdateChurn(t *testing.T) {
	slot := primitives.Slot(10_000_000)
	epoch := slots.ToEpoch(slot)
	t.Run("state earliest exit epoch is old", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{
			Slot: slot,
			Validators: []*eth.Validator{
				{
					EffectiveBalance: params.BeaconConfig().MaxEffectiveBalanceElectra,
				},
			},
			Balances:             []uint64{params.BeaconConfig().MaxEffectiveBalanceElectra},
			EarliestExitEpoch:    epoch - params.BeaconConfig().MaxSeedLookahead*2, // Old, relative to slot.
			ExitBalanceToConsume: primitives.Gwei(20_000_000),
		})
		require.NoError(t, err)
		activeBal, err := helpers.TotalActiveBalance(st)
		require.NoError(t, err)

		exitBal := primitives.Gwei(10_000_000)

		wantExitBalToConsume := helpers.ActivationExitChurnLimit(primitives.Gwei(activeBal)) - exitBal

		ee, err := st.ExitEpochAndUpdateChurn(exitBal)
		require.NoError(t, err)

		wantExitEpoch := helpers.ActivationExitEpoch(epoch)
		require.Equal(t, wantExitEpoch, ee)

		p := st.ToProto()
		pb, ok := p.(*eth.BeaconStateElectra)
		if !ok {
			t.Fatal("wrong proto")
		}
		require.Equal(t, wantExitBalToConsume, pb.ExitBalanceToConsume)
		require.Equal(t, wantExitEpoch, pb.EarliestExitEpoch)
	})

	t.Run("state exit bal to consume is less than activation exit churn limit", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{
			Slot: slot,
			Validators: []*eth.Validator{
				{
					EffectiveBalance: params.BeaconConfig().MaxEffectiveBalanceElectra,
				},
			},
			Balances:             []uint64{params.BeaconConfig().MaxEffectiveBalanceElectra},
			EarliestExitEpoch:    epoch,
			ExitBalanceToConsume: primitives.Gwei(20_000_000),
		})
		require.NoError(t, err)
		activeBal, err := helpers.TotalActiveBalance(st)
		require.NoError(t, err)

		activationExitChurnLimit := helpers.ActivationExitChurnLimit(primitives.Gwei(activeBal))
		exitBal := activationExitChurnLimit * 2

		wantExitBalToConsume := primitives.Gwei(0)

		ee, err := st.ExitEpochAndUpdateChurn(exitBal)
		require.NoError(t, err)

		wantExitEpoch := helpers.ActivationExitEpoch(epoch) + 1
		require.Equal(t, wantExitEpoch, ee)

		p := st.ToProto()
		pb, ok := p.(*eth.BeaconStateElectra)
		if !ok {
			t.Fatal("wrong proto")
		}
		require.Equal(t, wantExitBalToConsume, pb.ExitBalanceToConsume)
		require.Equal(t, wantExitEpoch, pb.EarliestExitEpoch)
	})

	t.Run("state earliest exit epoch is in the future and exit balance is less than state", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{
			Slot: slot,
			Validators: []*eth.Validator{
				{
					EffectiveBalance: params.BeaconConfig().MaxEffectiveBalanceElectra,
				},
			},
			Balances:             []uint64{params.BeaconConfig().MaxEffectiveBalanceElectra},
			EarliestExitEpoch:    epoch + 10_000,
			ExitBalanceToConsume: primitives.Gwei(20_000_000),
		})
		require.NoError(t, err)

		exitBal := primitives.Gwei(10_000_000)

		wantExitBalToConsume := primitives.Gwei(20_000_000) - exitBal

		ee, err := st.ExitEpochAndUpdateChurn(exitBal)
		require.NoError(t, err)

		wantExitEpoch := epoch + 10_000
		require.Equal(t, wantExitEpoch, ee)

		p := st.ToProto()
		pb, ok := p.(*eth.BeaconStateElectra)
		if !ok {
			t.Fatal("wrong proto")
		}
		require.Equal(t, wantExitBalToConsume, pb.ExitBalanceToConsume)
		require.Equal(t, wantExitEpoch, pb.EarliestExitEpoch)
	})

	t.Run("state earliest exit epoch is in the future and exit balance exceeds state", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{
			Slot: slot,
			Validators: []*eth.Validator{
				{
					EffectiveBalance: params.BeaconConfig().MaxEffectiveBalanceElectra,
				},
			},
			Balances:             []uint64{params.BeaconConfig().MaxEffectiveBalanceElectra},
			EarliestExitEpoch:    epoch + 10_000,
			ExitBalanceToConsume: primitives.Gwei(20_000_000),
		})
		require.NoError(t, err)

		exitBal := primitives.Gwei(40_000_000)
		activeBal, err := helpers.TotalActiveBalance(st)
		require.NoError(t, err)
		activationExitChurnLimit := helpers.ActivationExitChurnLimit(primitives.Gwei(activeBal))
		wantExitBalToConsume := activationExitChurnLimit - 20_000_000

		ee, err := st.ExitEpochAndUpdateChurn(exitBal)
		require.NoError(t, err)

		wantExitEpoch := epoch + 10_000 + 1
		require.Equal(t, wantExitEpoch, ee)

		p := st.ToProto()
		pb, ok := p.(*eth.BeaconStateElectra)
		if !ok {
			t.Fatal("wrong proto")
		}
		require.Equal(t, wantExitBalToConsume, pb.ExitBalanceToConsume)
		require.Equal(t, wantExitEpoch, pb.EarliestExitEpoch)
	})

	t.Run("earlier than electra returns error", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
		require.NoError(t, err)
		_, err = st.ExitEpochAndUpdateChurn(0)
		require.ErrorContains(t, "is not supported", err)
	})
}
