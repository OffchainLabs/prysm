package hdiff

import (
	"bytes"
	"context"
	"flag"
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/pkg/errors"
)

var sourceFile = flag.String("source", "", "Path to the source file")
var targetFile = flag.String("target", "", "Path to the target file")

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

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
	source, err = ApplyDiff(ctx, source, hdiff)
	require.NoError(t, err)
	require.DeepEqual(t, source, target)
}

func getMainnetStates() (state.BeaconState, state.BeaconState, error) {
	sourceBytes, err := os.ReadFile(*sourceFile)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read source file")
	}
	targetBytes, err := os.ReadFile(*targetFile)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read target file")
	}
	sourceProto := &ethpb.BeaconStateDeneb{}
	if err := sourceProto.UnmarshalSSZ(sourceBytes); err != nil {
		return nil, nil, errors.Wrap(err, "failed to unmarshal source proto")
	}
	source, err := state_native.InitializeFromProtoDeneb(sourceProto)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to initialize source state")
	}
	targetProto := &ethpb.BeaconStateElectra{}
	if err := targetProto.UnmarshalSSZ(targetBytes); err != nil {
		return nil, nil, errors.Wrap(err, "failed to unmarshal target proto")
	}
	target, err := state_native.InitializeFromProtoElectra(targetProto)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to initialize target state")
	}
	return source, target, nil
}

func TestApplyDiffMainnet(t *testing.T) {
	if *sourceFile == "" || *targetFile == "" {
		t.Skip("source and target files not provided")
	}
	source, target, err := getMainnetStates()
	require.NoError(t, err)
	hdiff, err := Diff(source, target)
	require.NoError(t, err)
	source, err = ApplyDiff(context.Background(), source, hdiff)
	require.NoError(t, err)
	sourceSSZ, err := source.MarshalSSZ()
	require.NoError(t, err)
	targetSSZ, err := target.MarshalSSZ()
	require.NoError(t, err)
	sVals := source.Validators()
	tVals := target.Validators()
	require.Equal(t, len(sVals), len(tVals))
	for i, v := range sVals {
		require.Equal(t, true, bytes.Equal(v.PublicKey, tVals[i].PublicKey))
		require.Equal(t, true, bytes.Equal(v.WithdrawalCredentials, tVals[i].WithdrawalCredentials))
		require.Equal(t, v.EffectiveBalance, tVals[i].EffectiveBalance)
		require.Equal(t, v.Slashed, tVals[i].Slashed)
		require.Equal(t, v.ActivationEligibilityEpoch, tVals[i].ActivationEligibilityEpoch)
		require.Equal(t, v.ActivationEpoch, tVals[i].ActivationEpoch)
		require.Equal(t, v.ExitEpoch, tVals[i].ExitEpoch)
		require.Equal(t, v.WithdrawableEpoch, tVals[i].WithdrawableEpoch)
	}
	sBals := source.Balances()
	tBals := target.Balances()
	require.Equal(t, len(sBals), len(tBals))
	for i, v := range sBals {
		require.Equal(t, v, tBals[i], "i: %d", i)
	}

	require.Equal(t, true, bytes.Equal(sourceSSZ, targetSSZ))
}

func BenchmarkGetDiff(b *testing.B) {
	if *sourceFile == "" || *targetFile == "" {
		b.Skip("source and target files not provided")
	}
	source, target, err := getMainnetStates()
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
	source, target, err := getMainnetStates()
	require.NoError(b, err)
	hdiff, err := Diff(source, target)
	require.NoError(b, err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		source, err = ApplyDiff(context.Background(), source, hdiff)
		require.NoError(b, err)
	}
}
