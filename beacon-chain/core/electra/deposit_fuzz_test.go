package electra_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/electra"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/fuzz"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	gofuzz "github.com/google/gofuzz"
)

func TestFuzzProcessDeposits_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconStateElectra{}
	deposits := make([]*ethpb.Deposit, 100)
	ctx := t.Context()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		for i := range deposits {
			fuzzer.Fuzz(deposits[i])
		}
		s, err := state_native.InitializeFromProtoUnsafeElectra(state)
		require.NoError(t, err)
		r, err := electra.ProcessDeposits(ctx, s, deposits)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, deposits)
		}
		fuzz.FreeMemory(i)
	}
}

func TestFuzzProcessDeposit_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconStateElectra{}
	deposit := &ethpb.Deposit{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(deposit)
		s, err := state_native.InitializeFromProtoUnsafeElectra(state)
		require.NoError(t, err)
		r, err := electra.ProcessDeposit(s, deposit, true)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, deposit)
		}
		fuzz.FreeMemory(i)
	}
}
