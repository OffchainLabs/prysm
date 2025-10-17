package filesystem

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestHasIndex(t *testing.T) {
	summary := NewDataColumnStorageSummary(0, [fieldparams.NumberOfColumns]bool{false, true})

	hasIndex := summary.HasIndex(1_000_000)
	require.Equal(t, false, hasIndex)

	hasIndex = summary.HasIndex(0)
	require.Equal(t, false, hasIndex)

	hasIndex = summary.HasIndex(1)
	require.Equal(t, true, hasIndex)
}

func TestHasAtLeastOneIndex(t *testing.T) {
	summary := NewDataColumnStorageSummary(0, [fieldparams.NumberOfColumns]bool{false, true})

	actual := summary.HasAtLeastOneIndex([]uint64{3, 1, fieldparams.NumberOfColumns, 2})
	require.Equal(t, true, actual)

	actual = summary.HasAtLeastOneIndex([]uint64{3, 4, fieldparams.NumberOfColumns, 2})
	require.Equal(t, false, actual)
}

func TestCount(t *testing.T) {
	summary := NewDataColumnStorageSummary(0, [fieldparams.NumberOfColumns]bool{false, true, false, true})

	count := summary.Count()
	require.Equal(t, uint64(2), count)
}

func TestAllAvailableDataColumns(t *testing.T) {
	const count = uint64(1_000)

	summary := NewDataColumnStorageSummary(0, [fieldparams.NumberOfColumns]bool{false, true, false, true})

	indices := make(map[uint64]bool, count)
	for i := range count {
		indices[i] = true
	}

	allAvailable := summary.AllAvailable(indices)
	require.Equal(t, false, allAvailable)

	indices = map[uint64]bool{1: true, 2: true}
	allAvailable = summary.AllAvailable(indices)
	require.Equal(t, false, allAvailable)

	indices = map[uint64]bool{1: true, 3: true}
	allAvailable = summary.AllAvailable(indices)
	require.Equal(t, true, allAvailable)
}

func TestStored(t *testing.T) {
	summary := NewDataColumnStorageSummary(0, [fieldparams.NumberOfColumns]bool{false, true, true, false})

	expected := map[uint64]bool{1: true, 2: true}
	actual := summary.Stored()

	require.Equal(t, len(expected), len(actual))
	for k, v := range expected {
		require.Equal(t, v, actual[k])
	}
}

func TestSummary(t *testing.T) {
	root := [fieldparams.RootLength]byte{}

	summaryCache := newDataColumnStorageSummaryCache()
	expected := NewDataColumnStorageSummary(0, [fieldparams.NumberOfColumns]bool{})
	actual := summaryCache.Summary(root)
	require.DeepEqual(t, expected, actual)

	summaryCache = newDataColumnStorageSummaryCache()
	expected = NewDataColumnStorageSummary(0, [fieldparams.NumberOfColumns]bool{true, false, true, false})
	summaryCache.cache[root] = expected
	actual = summaryCache.Summary(root)
	require.DeepEqual(t, expected, actual)
}

func TestHighestEpoch(t *testing.T) {
	root1 := [fieldparams.RootLength]byte{1}
	root2 := [fieldparams.RootLength]byte{2}
	root3 := [fieldparams.RootLength]byte{3}

	summaryCache := newDataColumnStorageSummaryCache()
	actual := summaryCache.HighestEpoch()
	require.Equal(t, primitives.Epoch(0), actual)

	err := summaryCache.set(DataColumnsIdent{Root: root1, Epoch: 42, Indices: []uint64{1, 3}})
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(42), summaryCache.HighestEpoch())

	err = summaryCache.set(DataColumnsIdent{Root: root2, Epoch: 43, Indices: []uint64{1, 3}})
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(43), summaryCache.HighestEpoch())

	err = summaryCache.set(DataColumnsIdent{Root: root3, Epoch: 40, Indices: []uint64{1, 3}})
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(43), summaryCache.HighestEpoch())
}

func TestSet(t *testing.T) {
	t.Run("Index out of bounds", func(t *testing.T) {
		summaryCache := newDataColumnStorageSummaryCache()
		err := summaryCache.set(DataColumnsIdent{Indices: []uint64{1_000_000}})
		require.ErrorIs(t, err, errDataColumnIndexOutOfBounds)
		require.Equal(t, params.BeaconConfig().FarFutureEpoch, summaryCache.lowestCachedEpoch)
		require.Equal(t, 0, len(summaryCache.cache))
	})

	t.Run("Nominal", func(t *testing.T) {
		root1 := [fieldparams.RootLength]byte{1}
		root2 := [fieldparams.RootLength]byte{2}

		summaryCache := newDataColumnStorageSummaryCache()

		err := summaryCache.set(DataColumnsIdent{Root: root1, Epoch: 42, Indices: []uint64{1, 3}})
		require.NoError(t, err)
		require.Equal(t, primitives.Epoch(42), summaryCache.lowestCachedEpoch)
		require.Equal(t, 1, len(summaryCache.cache))
		expected := DataColumnStorageSummary{epoch: 42, mask: [fieldparams.NumberOfColumns]bool{false, true, false, true}}
		actual := summaryCache.cache[root1]
		require.DeepEqual(t, expected, actual)

		err = summaryCache.set(DataColumnsIdent{Root: root1, Epoch: 42, Indices: []uint64{0, 1}})
		require.NoError(t, err)
		require.Equal(t, primitives.Epoch(42), summaryCache.lowestCachedEpoch)
		require.Equal(t, 1, len(summaryCache.cache))
		expected = DataColumnStorageSummary{epoch: 42, mask: [fieldparams.NumberOfColumns]bool{true, true, false, true}}
		actual = summaryCache.cache[root1]
		require.DeepEqual(t, expected, actual)

		err = summaryCache.set(DataColumnsIdent{Root: root2, Epoch: 43, Indices: []uint64{1}})
		require.NoError(t, err)
		require.Equal(t, primitives.Epoch(42), summaryCache.lowestCachedEpoch) // Epoch 42 is still the lowest
		require.Equal(t, 2, len(summaryCache.cache))
		expected = DataColumnStorageSummary{epoch: 43, mask: [fieldparams.NumberOfColumns]bool{false, true}}
		actual = summaryCache.cache[root2]
		require.DeepEqual(t, expected, actual)
	})
}

func TestGet(t *testing.T) {
	t.Run("Not in cache", func(t *testing.T) {
		summaryCache := newDataColumnStorageSummaryCache()
		root := [fieldparams.RootLength]byte{}
		_, ok := summaryCache.get(root)
		require.Equal(t, false, ok)
	})

	t.Run("In cache", func(t *testing.T) {
		root := [fieldparams.RootLength]byte{}
		summaryCache := newDataColumnStorageSummaryCache()
		summaryCache.cache[root] = NewDataColumnStorageSummary(42, [fieldparams.NumberOfColumns]bool{true, false, true, false})
		actual, ok := summaryCache.get(root)
		require.Equal(t, true, ok)
		expected := NewDataColumnStorageSummary(42, [fieldparams.NumberOfColumns]bool{true, false, true, false})
		require.DeepEqual(t, expected, actual)
	})
}

func TestEvict(t *testing.T) {
	t.Run("No eviction", func(t *testing.T) {
		root := [fieldparams.RootLength]byte{}
		summaryCache := newDataColumnStorageSummaryCache()

		evicted := summaryCache.evict(root)
		require.Equal(t, 0, evicted)
	})

	t.Run("Eviction", func(t *testing.T) {
		root1 := [fieldparams.RootLength]byte{1}
		root2 := [fieldparams.RootLength]byte{2}
		summaryCache := newDataColumnStorageSummaryCache()
		summaryCache.cache[root1] = NewDataColumnStorageSummary(42, [fieldparams.NumberOfColumns]bool{true, false, true, false})
		summaryCache.cache[root2] = NewDataColumnStorageSummary(43, [fieldparams.NumberOfColumns]bool{false, true, false, true})

		evicted := summaryCache.evict(root1)
		require.Equal(t, 2, evicted)
		require.Equal(t, 1, len(summaryCache.cache))

		_, ok := summaryCache.cache[root1]
		require.Equal(t, false, ok)

		_, ok = summaryCache.cache[root2]
		require.Equal(t, true, ok)
	})
}

func TestPruneUpTo(t *testing.T) {
	t.Run("No pruning", func(t *testing.T) {
		summaryCache := newDataColumnStorageSummaryCache()
		err := summaryCache.set(DataColumnsIdent{Root: [fieldparams.RootLength]byte{1}, Epoch: 42, Indices: []uint64{1}})
		require.NoError(t, err)

		err = summaryCache.set(DataColumnsIdent{Root: [fieldparams.RootLength]byte{2}, Epoch: 43, Indices: []uint64{2, 4}})
		require.NoError(t, err)

		count := summaryCache.pruneUpTo(41)
		require.Equal(t, uint64(0), count)
		require.Equal(t, 2, len(summaryCache.cache))
		require.Equal(t, primitives.Epoch(42), summaryCache.lowestCachedEpoch)
	})

	t.Run("Pruning", func(t *testing.T) {
		summaryCache := newDataColumnStorageSummaryCache()
		err := summaryCache.set(DataColumnsIdent{Root: [fieldparams.RootLength]byte{1}, Epoch: 42, Indices: []uint64{1}})
		require.NoError(t, err)

		err = summaryCache.set(DataColumnsIdent{Root: [fieldparams.RootLength]byte{2}, Epoch: 44, Indices: []uint64{2, 4}})
		require.NoError(t, err)

		err = summaryCache.set(DataColumnsIdent{Root: [fieldparams.RootLength]byte{3}, Epoch: 45, Indices: []uint64{2, 4}})
		require.NoError(t, err)

		count := summaryCache.pruneUpTo(42)
		require.Equal(t, uint64(1), count)
		require.Equal(t, 2, len(summaryCache.cache))
		require.Equal(t, primitives.Epoch(44), summaryCache.lowestCachedEpoch)

		count = summaryCache.pruneUpTo(45)
		require.Equal(t, uint64(4), count)
		require.Equal(t, 0, len(summaryCache.cache))
		require.Equal(t, params.BeaconConfig().FarFutureEpoch, summaryCache.lowestCachedEpoch)
		require.Equal(t, primitives.Epoch(0), summaryCache.highestCachedEpoch)

	})

	t.Run("Clear", func(t *testing.T) {
		summaryCache := newDataColumnStorageSummaryCache()
		err := summaryCache.set(DataColumnsIdent{Root: [fieldparams.RootLength]byte{1}, Epoch: 42, Indices: []uint64{1}})
		require.NoError(t, err)

		err = summaryCache.set(DataColumnsIdent{Root: [fieldparams.RootLength]byte{2}, Epoch: 44, Indices: []uint64{2, 4}})
		require.NoError(t, err)

		err = summaryCache.set(DataColumnsIdent{Root: [fieldparams.RootLength]byte{3}, Epoch: 45, Indices: []uint64{2, 4}})
		require.NoError(t, err)

		count := summaryCache.clear()
		require.Equal(t, uint64(5), count)
		require.Equal(t, 0, len(summaryCache.cache))
		require.Equal(t, params.BeaconConfig().FarFutureEpoch, summaryCache.lowestCachedEpoch)
		require.Equal(t, primitives.Epoch(0), summaryCache.highestCachedEpoch)
	})
}
