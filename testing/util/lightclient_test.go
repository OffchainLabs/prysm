package util_test

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestLightClientUtils(t *testing.T) {

	t.Run("Altair", func(t *testing.T) {
		t.Run("WithNoFinalizedBlock", func(t *testing.T) {
			l := util.NewTestLightClient(t, 1, util.WithNoFinalizedCheckpoint())
			require.IsNil(t, l.FinalizedBlock)
		})
		t.Run("WithIncreasedAttestedSlot", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 1)
			l2 := util.NewTestLightClient(t, 1, util.WithIncreasedAttestedSlot(1))
			require.Equal(t, l1.AttestedBlock.Block().Slot()+1, l2.AttestedBlock.Block().Slot())
		})
		t.Run("WithIncreasedFinalizedSlot", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 1)
			l2 := util.NewTestLightClient(t, 1, util.WithIncreasedFinalizedSlot(1))
			require.Equal(t, l1.FinalizedBlock.Block().Slot()+1, l2.FinalizedBlock.Block().Slot())
		})
		t.Run("WithSupermajority", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 1)
			l2 := util.NewTestLightClient(t, 1, util.WithSupermajority())
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
		t.Run("WithIncreasedAttestedSlot", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 2)
			l2 := util.NewTestLightClient(t, 2, util.WithIncreasedAttestedSlot(1))
			require.Equal(t, l1.AttestedBlock.Block().Slot()+1, l2.AttestedBlock.Block().Slot())
		})
		t.Run("WithIncreasedFinalizedSlot", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 2)
			l2 := util.NewTestLightClient(t, 2, util.WithIncreasedFinalizedSlot(1))
			require.Equal(t, l1.FinalizedBlock.Block().Slot()+1, l2.FinalizedBlock.Block().Slot())
		})
		t.Run("WithSupermajority", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 2)
			l2 := util.NewTestLightClient(t, 2, util.WithSupermajority())
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
		t.Run("WithIncreasedAttestedSlot", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 3)
			l2 := util.NewTestLightClient(t, 3, util.WithIncreasedAttestedSlot(1))
			require.Equal(t, l1.AttestedBlock.Block().Slot()+1, l2.AttestedBlock.Block().Slot())
		})
		t.Run("WithIncreasedFinalizedSlot", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 3)
			l2 := util.NewTestLightClient(t, 3, util.WithIncreasedFinalizedSlot(1))
			require.Equal(t, l1.FinalizedBlock.Block().Slot()+1, l2.FinalizedBlock.Block().Slot())
		})
		t.Run("WithSupermajority", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 3)
			l2 := util.NewTestLightClient(t, 3, util.WithSupermajority())
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
		t.Run("WithIncreasedAttestedSlot", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 4)
			l2 := util.NewTestLightClient(t, 4, util.WithIncreasedAttestedSlot(1))
			require.Equal(t, l1.AttestedBlock.Block().Slot()+1, l2.AttestedBlock.Block().Slot())
		})
		t.Run("WithIncreasedFinalizedSlot", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 4)
			l2 := util.NewTestLightClient(t, 4, util.WithIncreasedFinalizedSlot(1))
			require.Equal(t, l1.FinalizedBlock.Block().Slot()+1, l2.FinalizedBlock.Block().Slot())
		})
		t.Run("WithSupermajority", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 4)
			l2 := util.NewTestLightClient(t, 4, util.WithSupermajority())
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
		t.Run("WithIncreasedAttestedSlot", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 5)
			l2 := util.NewTestLightClient(t, 5, util.WithIncreasedAttestedSlot(1))
			require.Equal(t, l1.AttestedBlock.Block().Slot()+1, l2.AttestedBlock.Block().Slot())
		})
		t.Run("WithIncreasedFinalizedSlot", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 5)
			l2 := util.NewTestLightClient(t, 5, util.WithIncreasedFinalizedSlot(1))
			require.Equal(t, l1.FinalizedBlock.Block().Slot()+1, l2.FinalizedBlock.Block().Slot())
		})
		t.Run("WithSupermajority", func(t *testing.T) {
			l1 := util.NewTestLightClient(t, 5)
			l2 := util.NewTestLightClient(t, 5, util.WithSupermajority())
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
	})

}
