package util_test

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestLightClientUtils(t *testing.T) {

	t.Run("Altair", func(t *testing.T) {
		t.Run("WithNoFinalizedBlock", func(t *testing.T) {
			l := util.NewTestLightClient(t, 1, util.WithNoFinalizedCheckpoint())
			require.IsNil(t, l.FinalizedBlock)
		})

	})

	t.Run("Bellatrix", func(t *testing.T) {
		t.Run("WithNoFinalizedBlock", func(t *testing.T) {
			l := util.NewTestLightClient(t, 2, util.WithNoFinalizedCheckpoint())
			require.IsNil(t, l.FinalizedBlock)
		})
		t.Run("WithFinalizedBlockInPrevFork", func(t *testing.T) {
			l := util.NewTestLightClient(t, 2, util.WithFinalizedCheckpointInPrevFork())
			require.Equal(t, l.FinalizedBlock.Version(), 1)
		})

	})

	t.Run("Capella", func(t *testing.T) {
		t.Run("WithNoFinalizedBlock", func(t *testing.T) {
			l := util.NewTestLightClient(t, 3, util.WithNoFinalizedCheckpoint())
			require.IsNil(t, l.FinalizedBlock)
		})
		t.Run("WithFinalizedBlockInPrevFork", func(t *testing.T) {
			l := util.NewTestLightClient(t, 3, util.WithFinalizedCheckpointInPrevFork())
			require.Equal(t, l.FinalizedBlock.Version(), 2)
		})
	})

	t.Run("Deneb", func(t *testing.T) {
		t.Run("WithNoFinalizedBlock", func(t *testing.T) {
			l := util.NewTestLightClient(t, 4, util.WithNoFinalizedCheckpoint())
			require.IsNil(t, l.FinalizedBlock)
		})
		t.Run("WithFinalizedBlockInPrevFork", func(t *testing.T) {
			l := util.NewTestLightClient(t, 4, util.WithFinalizedCheckpointInPrevFork())
			require.Equal(t, l.FinalizedBlock.Version(), 3)
		})
	})

	t.Run("Electra", func(t *testing.T) {
		t.Run("WithNoFinalizedBlock", func(t *testing.T) {
			l := util.NewTestLightClient(t, 5, util.WithNoFinalizedCheckpoint())
			require.IsNil(t, l.FinalizedBlock)
		})
		t.Run("WithFinalizedBlockInPrevFork", func(t *testing.T) {
			l := util.NewTestLightClient(t, 5, util.WithFinalizedCheckpointInPrevFork())
			require.Equal(t, l.FinalizedBlock.Version(), 4)
		})
	})

}
