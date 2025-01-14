package sync_contribution

import (
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	v2 "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestMaxCoverSyncContributionAggregation(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		res, err := maxCoverSyncContributionAggregation(nil)
		require.NoError(t, err)
		assert.Equal(t, 0, len(res))
	})

	t.Run("empty input", func(t *testing.T) {
		res, err := maxCoverSyncContributionAggregation([]*v2.SyncCommitteeContribution{})
		require.NoError(t, err)
		assert.Equal(t, 0, len(res))
	})

	t.Run("single contribution", func(t *testing.T) {
		c := &v2.SyncCommitteeContribution{
			AggregationBits: bitfield.NewBitlist(8),
		}
		c.AggregationBits.SetBitAt(0, true)

		res, err := maxCoverSyncContributionAggregation([]*v2.SyncCommitteeContribution{c})
		require.NoError(t, err)
		assert.Equal(t, 1, len(res))
		assert.DeepEqual(t, c.AggregationBits, res[0].AggregationBits)
	})

	t.Run("non overlapping contributions", func(t *testing.T) {
		c1 := &v2.SyncCommitteeContribution{
			AggregationBits: bitfield.NewBitlist(8),
		}
		c1.AggregationBits.SetBitAt(0, true)
		c1.AggregationBits.SetBitAt(1, true)

		c2 := &v2.SyncCommitteeContribution{
			AggregationBits: bitfield.NewBitlist(8),
		}
		c2.AggregationBits.SetBitAt(2, true)
		c2.AggregationBits.SetBitAt(3, true)

		res, err := maxCoverSyncContributionAggregation([]*v2.SyncCommitteeContribution{c1, c2})
		require.NoError(t, err)
		assert.Equal(t, 1, len(res))

		// Check that all bits are covered
		expectedBits := bitfield.NewBitlist(8)
		for i := 0; i < 4; i++ {
			expectedBits.SetBitAt(uint64(i), true)
		}
		assert.DeepEqual(t, expectedBits, res[0].AggregationBits)
	})

	t.Run("overlapping contributions", func(t *testing.T) {
		c1 := &v2.SyncCommitteeContribution{
			AggregationBits: bitfield.NewBitlist(8),
		}
		c1.AggregationBits.SetBitAt(0, true)
		c1.AggregationBits.SetBitAt(1, true)

		c2 := &v2.SyncCommitteeContribution{
			AggregationBits: bitfield.NewBitlist(8),
		}
		c2.AggregationBits.SetBitAt(1, true)
		c2.AggregationBits.SetBitAt(2, true)

		res, err := maxCoverSyncContributionAggregation([]*v2.SyncCommitteeContribution{c1, c2})
		require.NoError(t, err)
		assert.Equal(t, 2, len(res))

		// Check that the original contributions are preserved
		assert.DeepEqual(t, c1.AggregationBits, res[0].AggregationBits)
		assert.DeepEqual(t, c2.AggregationBits, res[1].AggregationBits)
	})
} 
