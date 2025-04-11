package das

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestEnsureDeleteSetDiskSummary(t *testing.T) {
	c := newDataColumnCache()
	key := dataColumnCacheKey{}
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
		dataColumnParamsByBlockRoot := verification.DataColumnsParamsByRoot{{1}: {{ColumnIndex: 10_000}}}
		roDataColumns, _ := verification.CreateTestVerifiedRoDataColumnSidecars(t, dataColumnParamsByBlockRoot)

		var entry dataColumnCacheEntry
		err := entry.stash(&roDataColumns[0])
		require.NotNil(t, err)
	})

	t.Run("Nominal and already existing", func(t *testing.T) {
		dataColumnParamsByBlockRoot := verification.DataColumnsParamsByRoot{{1}: {{ColumnIndex: 1}}}
		roDataColumns, _ := verification.CreateTestVerifiedRoDataColumnSidecars(t, dataColumnParamsByBlockRoot)

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
		root := [fieldparams.RootLength]byte{}
		commitmentsArray := safeCommitmentsArray{nil, [][]byte{[]byte{1}}}

		dataColumnParamsByBlockRoot := verification.DataColumnsParamsByRoot{root: {{ColumnIndex: 1}}}
		roDataColumns, _ := verification.CreateTestVerifiedRoDataColumnSidecars(t, dataColumnParamsByBlockRoot)

		var scs [fieldparams.NumberOfColumns]*blocks.RODataColumn
		scs[1] = &roDataColumns[0]

		dataColumnCacheEntry := dataColumnCacheEntry{scs: scs}

		_, err := dataColumnCacheEntry.filter(root, &commitmentsArray)
		require.NotNil(t, err)
	})

	t.Run("Nominal", func(t *testing.T) {
		root := [fieldparams.RootLength]byte{}
		commitmentsArray := safeCommitmentsArray{nil, [][]byte{[]byte{1}}, nil, [][]byte{[]byte{3}}}

		diskSummary := filesystem.NewDataColumnStorageSummary(42, [fieldparams.NumberOfColumns]bool{false, true})

		dataColumnParamsByBlockRoot := verification.DataColumnsParamsByRoot{root: {{ColumnIndex: 3, KzgCommitments: [][]byte{[]byte{3}}}}}
		expected, _ := verification.CreateTestVerifiedRoDataColumnSidecars(t, dataColumnParamsByBlockRoot)

		var scs [fieldparams.NumberOfColumns]*blocks.RODataColumn
		scs[3] = &expected[0]

		dataColumnCacheEntry := dataColumnCacheEntry{scs: scs, diskSummary: diskSummary}

		actual, err := dataColumnCacheEntry.filter(root, &commitmentsArray)
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
