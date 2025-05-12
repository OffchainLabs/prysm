package hdiff

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
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
