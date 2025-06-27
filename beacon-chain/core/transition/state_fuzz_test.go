package transition

import (
	"testing"

	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	fuzz "github.com/google/gofuzz"
)

func TestGenesisBeaconState_1000(t *testing.T) {
	SkipSlotCache.Disable()
	defer SkipSlotCache.Enable()
	fuzzer := fuzz.NewWithSeed(0)
	fuzzer.NilChance(0.1)
	deposits := make([]*ethpb.Deposit, 300000)
	var genesisTime uint64
	eth1Data := &ethpb.Eth1Data{}
	for i := 0; i < 1000; i++ {
		fuzzer.Fuzz(&deposits)
		fuzzer.Fuzz(&genesisTime)
		fuzzer.Fuzz(eth1Data)
		gs, err := GenesisBeaconState(t.Context(), deposits, genesisTime, eth1Data)
		if err != nil {
			if gs != nil {
				t.Fatalf("Genesis state should be nil on err. found: %v on error: %v for inputs deposit: %v "+
					"genesis time: %v eth1data: %v", gs, err, deposits, genesisTime, eth1Data)
			}
		}
	}
}

func TestOptimizedGenesisBeaconState_1000(t *testing.T) {
	SkipSlotCache.Disable()
	defer SkipSlotCache.Enable()
	fuzzer := fuzz.NewWithSeed(0)
	fuzzer.NilChance(0.1)
	var genesisTime uint64
	preState, err := state_native.InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{})
	require.NoError(t, err)
	eth1Data := &ethpb.Eth1Data{}
	for i := 0; i < 1000; i++ {
		fuzzer.Fuzz(&genesisTime)
		fuzzer.Fuzz(eth1Data)
		fuzzer.Fuzz(preState)
		gs, err := OptimizedGenesisBeaconState(genesisTime, preState, eth1Data)
		if err != nil {
			if gs != nil {
				t.Fatalf("Genesis state should be nil on err. found: %v on error: %v for inputs genesis time: %v "+
					"pre state: %v eth1data: %v", gs, err, genesisTime, preState, eth1Data)
			}
		}
	}
}

func TestIsValidGenesisState_100000(_ *testing.T) {
	SkipSlotCache.Disable()
	defer SkipSlotCache.Enable()
	fuzzer := fuzz.NewWithSeed(0)
	fuzzer.NilChance(0.1)
	var chainStartDepositCount, currentTime uint64
	for i := 0; i < 100000; i++ {
		fuzzer.Fuzz(&chainStartDepositCount)
		fuzzer.Fuzz(&currentTime)
		IsValidGenesisState(chainStartDepositCount, currentTime)
	}
}
