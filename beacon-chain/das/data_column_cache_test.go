package das

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestEnsureDeleteSetDiskSummary(t *testing.T) {
	c := newDataColumnCache()
	key := cacheKey{}
	entry := c.ensure(key)
	require.DeepEqual(t, dataColumnCacheEntry{}, *entry)

	diskSummary := filesystem.NewDataColumnStorageSummary(42, [fieldparams.NumberOfColumns]bool{true})
	entry.setDiskSummary(diskSummary)
	entry = c.ensure(key)
	require.DeepEqual(t, dataColumnCacheEntry{diskSummary: diskSummary}, *entry)

	c.delete(key)
	entry = c.ensure(key)
	require.DeepEqual(t, dataColumnCacheEntry{}, *entry)
}

func TestStash(t *testing.T) {
	t.Run("Index too high", func(t *testing.T) {
		roDataColumns, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{{Index: 10_000}})

		var entry dataColumnCacheEntry
		err := entry.stash(&roDataColumns[0])
		require.NotNil(t, err)
	})

	t.Run("Nominal and already existing", func(t *testing.T) {
		roDataColumns, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{{Index: 1}})

		var entry dataColumnCacheEntry
		err := entry.stash(&roDataColumns[0])
		require.NoError(t, err)

		require.DeepEqual(t, roDataColumns[0], entry.scs[1])

		err = entry.stash(&roDataColumns[0])
		require.NotNil(t, err)
	})
}

func TestFilterDataColumns(t *testing.T) {
	t.Run("All available", func(t *testing.T) {
		commitmentsArray := safeCommitmentsArray{nil, [][]byte{[]byte{1}}, nil, [][]byte{[]byte{3}}}

		diskSummary := filesystem.NewDataColumnStorageSummary(42, [fieldparams.NumberOfColumns]bool{false, true, false, true})

		dataColumnCacheEntry := dataColumnCacheEntry{diskSummary: diskSummary}

		actual, err := dataColumnCacheEntry.filter([fieldparams.RootLength]byte{}, &commitmentsArray)
		require.NoError(t, err)
		require.IsNil(t, actual)
	})

	t.Run("Some scs missing", func(t *testing.T) {
		commitmentsArray := safeCommitmentsArray{nil, [][]byte{[]byte{1}}}

		diskSummary := filesystem.NewDataColumnStorageSummary(42, [fieldparams.NumberOfColumns]bool{})

		dataColumnCacheEntry := dataColumnCacheEntry{diskSummary: diskSummary}

		_, err := dataColumnCacheEntry.filter([fieldparams.RootLength]byte{}, &commitmentsArray)
		require.NotNil(t, err)
	})

	t.Run("Commitments not equal", func(t *testing.T) {
		commitmentsArray := safeCommitmentsArray{nil, [][]byte{[]byte{1}}}

		roDataColumns, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{{Index: 1}})

		var scs [fieldparams.NumberOfColumns]*blocks.RODataColumn
		scs[1] = &roDataColumns[0]

		dataColumnCacheEntry := dataColumnCacheEntry{scs: scs}

		_, err := dataColumnCacheEntry.filter(roDataColumns[0].BlockRoot(), &commitmentsArray)
		require.NotNil(t, err)
	})

	t.Run("Nominal", func(t *testing.T) {
		commitmentsArray := safeCommitmentsArray{nil, [][]byte{[]byte{1}}, nil, [][]byte{[]byte{3}}}
		diskSummary := filesystem.NewDataColumnStorageSummary(42, [fieldparams.NumberOfColumns]bool{false, true})
		expected, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{{Index: 3, KzgCommitments: [][]byte{[]byte{3}}}})

		var scs [fieldparams.NumberOfColumns]*blocks.RODataColumn
		scs[3] = &expected[0]

		dataColumnCacheEntry := dataColumnCacheEntry{scs: scs, diskSummary: diskSummary}

		actual, err := dataColumnCacheEntry.filter(expected[0].BlockRoot(), &commitmentsArray)
		require.NoError(t, err)

		require.DeepEqual(t, expected, actual)
	})
}

func TestCount(t *testing.T) {
	s := safeCommitmentsArray{nil, [][]byte{[]byte{1}}, nil, [][]byte{[]byte{3}}}
	require.Equal(t, 2, s.count())
}

func TestNonEmptyIndices(t *testing.T) {
	s := safeCommitmentsArray{nil, [][]byte{[]byte{10}}, nil, [][]byte{[]byte{20}}}
	actual := s.nonEmptyIndices()
	require.DeepEqual(t, map[uint64]bool{1: true, 3: true}, actual)
}

func TestSliceBytesEqual(t *testing.T) {
	t.Run("Different lengths", func(t *testing.T) {
		a := [][]byte{[]byte{1, 2, 3}}
		b := [][]byte{[]byte{1, 2, 3}, []byte{4, 5, 6}}
		require.Equal(t, false, sliceBytesEqual(a, b))
	})

	t.Run("Same length but different content", func(t *testing.T) {
		a := [][]byte{[]byte{1, 2, 3}, []byte{4, 5, 6}}
		b := [][]byte{[]byte{1, 2, 3}, []byte{4, 5, 7}}
		require.Equal(t, false, sliceBytesEqual(a, b))
	})

	t.Run("Equal slices", func(t *testing.T) {
		a := [][]byte{[]byte{1, 2, 3}, []byte{4, 5, 6}}
		b := [][]byte{[]byte{1, 2, 3}, []byte{4, 5, 6}}
		require.Equal(t, true, sliceBytesEqual(a, b))
	})
}
