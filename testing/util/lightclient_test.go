package util_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestLightClientUtils(t *testing.T) {

	t.Run("WithNoFinalizedBlock", func(t *testing.T) {
		for i := 1; i < 6; i++ {
			t.Run(version.String(i), func(t *testing.T) {
				l := util.NewTestLightClient(t, i, util.WithNoFinalizedCheckpoint())
				require.IsNil(t, l.FinalizedBlock)
			})
		}
	})

	t.Run("WithFinalizedBlockInPrevFork", func(t *testing.T) {
		for i := 2; i < 6; i++ {
			t.Run(version.String(i), func(t *testing.T) {
				l := util.NewTestLightClient(t, i, util.WithFinalizedCheckpointInPrevFork())
				require.Equal(t, l.FinalizedBlock.Version(), i-1)
			})
		}
	})

	t.Run("WithIncreasedAttestedSlot", func(t *testing.T) {
		for i := 1; i < 6; i++ {
			t.Run(version.String(i), func(t *testing.T) {
				l1 := util.NewTestLightClient(t, i)
				l2 := util.NewTestLightClient(t, i, util.WithIncreasedAttestedSlot(1))
				require.Equal(t, l1.AttestedBlock.Block().Slot()+1, l2.AttestedBlock.Block().Slot())
			})
		}
	})

	t.Run("WithIncreasedFinalizedSlot", func(t *testing.T) {
		for i := 1; i < 6; i++ {
			t.Run(version.String(i), func(t *testing.T) {
				l1 := util.NewTestLightClient(t, i)
				l2 := util.NewTestLightClient(t, i, util.WithIncreasedFinalizedSlot(1))
				require.Equal(t, l1.FinalizedBlock.Block().Slot()+1, l2.FinalizedBlock.Block().Slot())
			})
		}
	})

	t.Run("WithSupermajority", func(t *testing.T) {
		for i := 1; i < 6; i++ {
			t.Run(version.String(i), func(t *testing.T) {
				l1 := util.NewTestLightClient(t, i)
				l2 := util.NewTestLightClient(t, i, util.WithSupermajority(0))
				l1SyncAgg, err := l1.Block.Block().Body().SyncAggregate()
				require.NoError(t, err)
				l1Bits := l1SyncAgg.SyncCommitteeBits.Count()
				l2SyncAgg, err := l2.Block.Block().Body().SyncAggregate()
				require.NoError(t, err)
				l2Bits := l2SyncAgg.SyncCommitteeBits.Count()
				supermajorityCount := uint64(float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0)

				require.Equal(t, true, l1Bits < supermajorityCount)
				require.Equal(t, true, l2Bits >= supermajorityCount)
			})
		}
	})

}
