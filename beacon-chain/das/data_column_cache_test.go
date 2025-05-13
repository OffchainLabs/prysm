package das

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestEnsureDeleteSetDiskSummary(t *testing.T) {
	c := newDataColumnCache()
	key := cacheKey{}
	entry := c.ensure(key)
	require.Equal(t, 0, len(entry.scs))

	nonDupe := c.ensure(key)
	require.Equal(t, entry, nonDupe) // same pointer
	expect, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{{Index: 1}})
	require.NoError(t, entry.stash(expect[0]))
	require.Equal(t, 1, len(entry.scs))
	cols, err := nonDupe.append([]blocks.RODataColumn{}, expect[0].BlockRoot(), peerdas.NewColumnIndicesFromSlice([]uint64{expect[0].Index}))
	require.NoError(t, err)
	require.DeepEqual(t, expect[0], cols[0])

	c.delete(key)
	entry = c.ensure(key)
	require.Equal(t, 0, len(entry.scs))
	require.NotEqual(t, entry, nonDupe) // different pointer
}

func TestStash(t *testing.T) {
	t.Run("Index too high", func(t *testing.T) {
		columns, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{{Index: 10_000}})

		var entry dataColumnCacheEntry
		err := entry.stash(columns[0])
		require.NotNil(t, err)
	})

	t.Run("Nominal and already existing", func(t *testing.T) {
		roDataColumns, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{{Index: 1}})

		entry := newDataColumnCacheEntry()
		err := entry.stash(roDataColumns[0])
		require.NoError(t, err)

		require.DeepEqual(t, roDataColumns[0], entry.scs[1])
		require.NoError(t, entry.stash(roDataColumns[0]))
		// stash simply replaces duplicate values now
		require.DeepEqual(t, roDataColumns[0], entry.scs[1])
	})
}

func TestAppendDataColumns(t *testing.T) {
	t.Run("All available", func(t *testing.T) {
		sum := filesystem.NewDataColumnStorageSummary(42, [fieldparams.NumberOfColumns]bool{false, true, false, true})
		notStored := indicesNotStored(sum, peerdas.NewColumnIndicesFromSlice([]uint64{1, 3}))
		actual, err := newDataColumnCacheEntry().append([]blocks.RODataColumn{}, [fieldparams.RootLength]byte{}, notStored)
		require.NoError(t, err)
		require.Equal(t, 0, len(actual))
	})

	t.Run("Some scs missing", func(t *testing.T) {
		sum := filesystem.NewDataColumnStorageSummary(42, [fieldparams.NumberOfColumns]bool{})

		notStored := indicesNotStored(sum, peerdas.NewColumnIndicesFromSlice([]uint64{1}))
		actual, err := newDataColumnCacheEntry().append([]blocks.RODataColumn{}, [fieldparams.RootLength]byte{}, notStored)
		require.Equal(t, 0, len(actual))
		require.NotNil(t, err)
	})

	t.Run("Nominal", func(t *testing.T) {
		indices := peerdas.NewColumnIndicesFromSlice([]uint64{1, 3})
		expected, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{{Index: 3, KzgCommitments: [][]byte{[]byte{3}}}})

		scs := map[uint64]blocks.RODataColumn{
			3: expected[0],
		}
		sum := filesystem.NewDataColumnStorageSummary(42, [fieldparams.NumberOfColumns]bool{false, true})
		entry := dataColumnCacheEntry{scs: scs}

		actual, err := entry.append([]blocks.RODataColumn{}, expected[0].BlockRoot(), indicesNotStored(sum, indices))
		require.NoError(t, err)

		require.DeepEqual(t, expected, actual)
	})
}
