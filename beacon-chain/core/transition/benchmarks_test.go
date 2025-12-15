package transition_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	coreState "github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/benchmark"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/protobuf/proto"
)

var runAmount = 25

func BenchmarkExecuteStateTransition_FullBlock(b *testing.B) {
	undo, err := benchmark.SetBenchmarkConfig()
	require.NoError(b, err)
	defer undo()
	beaconState, err := benchmark.PreGenState1Epoch()
	require.NoError(b, err)
	cleanStates := clonedStates(beaconState)
	block, err := benchmark.PreGenFullBlock()
	require.NoError(b, err)

	for i := 0; b.Loop(); i++ {
		wsb, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(b, err)
		_, err = coreState.ExecuteStateTransition(b.Context(), cleanStates[i], wsb)
		require.NoError(b, err)
	}
}

func BenchmarkExecuteStateTransition_WithCache(b *testing.B) {
	undo, err := benchmark.SetBenchmarkConfig()
	require.NoError(b, err)
	defer undo()

	beaconState, err := benchmark.PreGenState1Epoch()
	require.NoError(b, err)
	cleanStates := clonedStates(beaconState)
	block, err := benchmark.PreGenFullBlock()
	require.NoError(b, err)

	// We have to reset slot back to last epoch to hydrate cache. Since
	// some attestations in block are from previous epoch
	currentSlot := beaconState.Slot()
	require.NoError(b, beaconState.SetSlot(beaconState.Slot()-params.BeaconConfig().SlotsPerEpoch))
	require.NoError(b, helpers.UpdateCommitteeCache(b.Context(), beaconState, time.CurrentEpoch(beaconState)))
	require.NoError(b, beaconState.SetSlot(currentSlot))
	// Run the state transition once to populate the cache.
	wsb, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(b, err)
	_, err = coreState.ExecuteStateTransition(b.Context(), beaconState, wsb)
	require.NoError(b, err, "Failed to process block, benchmarks will fail")

	for i := 0; b.Loop(); i++ {
		wsb, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(b, err)
		_, err = coreState.ExecuteStateTransition(b.Context(), cleanStates[i], wsb)
		require.NoError(b, err, "Failed to process block, benchmarks will fail")
	}
}

func BenchmarkProcessEpoch_2FullEpochs(b *testing.B) {
	undo, err := benchmark.SetBenchmarkConfig()
	require.NoError(b, err)
	defer undo()
	beaconState, err := benchmark.PreGenstateFullEpochs()
	require.NoError(b, err)

	// We have to reset slot back to last epoch to hydrate cache. Since
	// some attestations in block are from previous epoch
	currentSlot := beaconState.Slot()
	require.NoError(b, beaconState.SetSlot(beaconState.Slot()-params.BeaconConfig().SlotsPerEpoch))
	require.NoError(b, helpers.UpdateCommitteeCache(b.Context(), beaconState, time.CurrentEpoch(beaconState)))
	require.NoError(b, beaconState.SetSlot(currentSlot))

	for b.Loop() {
		// ProcessEpochPrecompute is the optimized version of process epoch. It's enabled by default
		// at run time.
		_, err := coreState.ProcessEpochPrecompute(b.Context(), beaconState.Copy())
		require.NoError(b, err)
	}
}

func BenchmarkHashTreeRoot_FullState(b *testing.B) {
	beaconState, err := benchmark.PreGenstateFullEpochs()
	require.NoError(b, err)

	for b.Loop() {
		_, err := beaconState.HashTreeRoot(b.Context())
		require.NoError(b, err)
	}
}

func BenchmarkHashTreeRootState_FullState(b *testing.B) {
	beaconState, err := benchmark.PreGenstateFullEpochs()
	require.NoError(b, err)

	ctx := b.Context()

	// Hydrate the HashTreeRootState cache.
	_, err = beaconState.HashTreeRoot(ctx)
	require.NoError(b, err)

	for b.Loop() {
		_, err := beaconState.HashTreeRoot(ctx)
		require.NoError(b, err)
	}
}

func BenchmarkMarshalState_FullState(b *testing.B) {
	beaconState, err := benchmark.PreGenstateFullEpochs()
	require.NoError(b, err)
	natState, err := state_native.ProtobufBeaconStatePhase0(beaconState.ToProtoUnsafe())
	require.NoError(b, err)
	b.Run("Proto_Marshal", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			_, err := proto.Marshal(natState)
			require.NoError(b, err)
		}
	})

	b.Run("Fast_SSZ_Marshal", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			_, err := natState.MarshalSSZ()
			require.NoError(b, err)
		}
	})
}

func BenchmarkUnmarshalState_FullState(b *testing.B) {
	beaconState, err := benchmark.PreGenstateFullEpochs()
	require.NoError(b, err)
	natState, err := state_native.ProtobufBeaconStatePhase0(beaconState.ToProtoUnsafe())
	require.NoError(b, err)
	protoObject, err := proto.Marshal(natState)
	require.NoError(b, err)
	sszObject, err := natState.MarshalSSZ()
	require.NoError(b, err)

	b.Run("Proto_Unmarshal", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			require.NoError(b, proto.Unmarshal(protoObject, &ethpb.BeaconState{}))
		}
	})

	b.Run("Fast_SSZ_Unmarshal", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			sszState := &ethpb.BeaconState{}
			require.NoError(b, sszState.UnmarshalSSZ(sszObject))
		}
	})
}

func clonedStates(beaconState state.BeaconState) []state.BeaconState {
	clonedStates := make([]state.BeaconState, runAmount)
	for i := range runAmount {
		clonedStates[i] = beaconState.Copy()
	}
	return clonedStates
}
