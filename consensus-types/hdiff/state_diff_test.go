package hdiff

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func Test_diffToState(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 256)
	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))
	hdiff, err := diffToState(source, target)
	require.NoError(t, err)
	require.Equal(t, hdiff.slot, target.Slot())
	require.Equal(t, hdiff.targetVersion, target.Version())
}

func Test_kmpIndex(t *testing.T) {
	intSlice := make([]*int, 10)
	for i := 0; i < len(intSlice); i++ {
		intSlice[i] = new(int)
		*intSlice[i] = i
	}
	integerEquals := func(a, b *int) bool {
		if a == nil && b == nil {
			return true
		}
		if a == nil || b == nil {
			return false
		}
		return *a == *b
	}
	t.Run("integer entries match", func(t *testing.T) {
		source := []*int{intSlice[0], intSlice[1], intSlice[2], intSlice[3], intSlice[4]}
		target := []*int{intSlice[2], intSlice[3], intSlice[4], intSlice[5], intSlice[6], intSlice[7], nil}
		target = append(target, source...)
		require.Equal(t, 2, kmpIndex(len(source), target, integerEquals))
	})
	t.Run("integer entries skipped", func(t *testing.T) {
		source := []*int{intSlice[0], intSlice[1], intSlice[2], intSlice[3], intSlice[4]}
		target := []*int{intSlice[2], intSlice[3], intSlice[4], intSlice[0], intSlice[5], nil}
		target = append(target, source...)
		require.Equal(t, 2, kmpIndex(len(source), target, integerEquals))
	})
	t.Run("integer entries repetitions", func(t *testing.T) {
		source := []*int{intSlice[0], intSlice[1], intSlice[0], intSlice[0], intSlice[0]}
		target := []*int{intSlice[0], intSlice[0], intSlice[1], intSlice[2], intSlice[5], nil}
		target = append(target, source...)
		require.Equal(t, 3, kmpIndex(len(source), target, integerEquals))
	})
	t.Run("integer entries no match", func(t *testing.T) {
		source := []*int{intSlice[0], intSlice[1], intSlice[2], intSlice[3]}
		target := []*int{intSlice[4], intSlice[5], intSlice[6], nil}
		target = append(target, source...)
		require.Equal(t, len(source), kmpIndex(len(source), target, integerEquals))
	})

}

func TestApplyDiff(t *testing.T) {
	source, keys := util.DeterministicGenesisStateElectra(t, 256)
	blk, err := util.GenerateFullBlockElectra(source, keys, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	ctx := context.Background()
	target, err := transition.ExecuteStateTransition(ctx, source, wsb)
	require.NoError(t, err)

	hdiff, err := Diff(source, target)
	require.NoError(t, err)
	require.NoError(t, ApplyDiff(ctx, source, hdiff))
	require.DeepEqual(t, source, target)
}

var sourceFile = flag.String("source", "", "Path to the source file")
var targetFile = flag.String("target", "", "Path to the target file")

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func BenchmarkGetDiff(b *testing.B) {
	if *sourceFile == "" || *targetFile == "" {
		b.Skip("source and target files not provided")
	}
	sourceBytes, err := os.ReadFile(*sourceFile)
	if err != nil {
		b.Fatalf("failed to read source file: %v", err)
	}
	targetBytes, err := os.ReadFile(*targetFile)
	if err != nil {
		b.Fatalf("failed to read target file: %v", err)
	}
	sourceProto := &ethpb.BeaconStateDeneb{}
	require.NoError(b, sourceProto.UnmarshalSSZ(sourceBytes))
	source, err := state_native.InitializeFromProtoDeneb(sourceProto)
	require.NoError(b, err)
	targetProto := &ethpb.BeaconStateElectra{}
	require.NoError(b, targetProto.UnmarshalSSZ(targetBytes))
	target, err := state_native.InitializeFromProtoElectra(targetProto)
	require.NoError(b, err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hdiff, err := Diff(source, target)
		b.Log("Diff size:", len(hdiff.StateDiff)+len(hdiff.BalancesDiff)+len(hdiff.ValidatorDiffs))
		require.NoError(b, err)
	}
}

func BenchmarkApplyDiff(b *testing.B) {
	if *sourceFile == "" || *targetFile == "" {
		b.Skip("source and target files not provided")
	}
	sourceBytes, err := os.ReadFile(*sourceFile)
	if err != nil {
		b.Fatalf("failed to read source file: %v", err)
	}
	targetBytes, err := os.ReadFile(*targetFile)
	if err != nil {
		b.Fatalf("failed to read target file: %v", err)
	}
	sourceProto := &ethpb.BeaconStateDeneb{}
	require.NoError(b, sourceProto.UnmarshalSSZ(sourceBytes))
	source, err := state_native.InitializeFromProtoDeneb(sourceProto)
	require.NoError(b, err)
	targetProto := &ethpb.BeaconStateElectra{}
	require.NoError(b, targetProto.UnmarshalSSZ(targetBytes))
	target, err := state_native.InitializeFromProtoElectra(targetProto)
	require.NoError(b, err)
	hdiff, err := Diff(source, target)
	require.NoError(b, err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := ApplyDiff(context.Background(), source, hdiff)
		require.NoError(b, err)
	}
}
