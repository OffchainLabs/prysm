package filesystem

import (
	"testing"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestSlotByRoot_Summary(t *testing.T) {
	t.Skip("Use new test for data columns")
	var noneSet, allSet, firstSet, lastSet, oneSet blobIndexMask
	firstSet[0] = true
	lastSet[len(lastSet)-1] = true
	oneSet[1] = true
	for i := range allSet {
		allSet[i] = true
	}
	cases := []struct {
		name     string
		root     [32]byte
		expected *blobIndexMask
	}{
		{
			name: "not found",
		},
		{
			name:     "none set",
			expected: &noneSet,
		},
		{
			name:     "index 1 set",
			expected: &oneSet,
		},
		{
			name:     "all set",
			expected: &allSet,
		},
		{
			name:     "first set",
			expected: &firstSet,
		},
		{
			name:     "last set",
			expected: &lastSet,
		},
	}
	sc := newBlobStorageCache()
	for _, c := range cases {
		if c.expected != nil {
			key := bytesutil.ToBytes32([]byte(c.name))
			sc.cache[key] = BlobStorageSummary{slot: 0, mask: *c.expected}
		}
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key := bytesutil.ToBytes32([]byte(c.name))
			sum := sc.Summary(key)
			for i := range c.expected {
				ui := uint64(i)
				if c.expected == nil {
					require.Equal(t, false, sum.HasIndex(ui))
				} else {
					require.Equal(t, c.expected[i], sum.HasIndex(ui))
				}
			}
		})
	}
}

func TestAllAvailable(t *testing.T) {
	idxUpTo := func(u int) []int {
		r := make([]int, u)
		for i := range r {
			r[i] = i
		}
		return r
	}
	require.DeepEqual(t, []int{}, idxUpTo(0))
	require.DeepEqual(t, []int{0}, idxUpTo(1))
	require.DeepEqual(t, []int{0, 1, 2, 3, 4, 5}, idxUpTo(6))
	cases := []struct {
		name   string
		idxSet []int
		count  int
		aa     bool
	}{
		{
			// If there are no blobs committed, then all the committed blobs are available.
			name:  "none in idx, 0 arg",
			count: 0,
			aa:    true,
		},
		{
			name:  "none in idx, 1 arg",
			count: 1,
			aa:    false,
		},
		{
			name:   "first in idx, 1 arg",
			idxSet: []int{0},
			count:  1,
			aa:     true,
		},
		{
			name:   "second in idx, 1 arg",
			idxSet: []int{1},
			count:  1,
			aa:     false,
		},
		{
			name:   "first missing, 2 arg",
			idxSet: []int{1},
			count:  2,
			aa:     false,
		},
		{
			name:  "all missing, 1 arg",
			count: 6,
			aa:    false,
		},
		{
			name:  "out of bound is safe",
			count: fieldparams.MaxBlobsPerBlock + 1,
			aa:    false,
		},
		{
			name:   "max present",
			count:  fieldparams.MaxBlobsPerBlock,
			idxSet: idxUpTo(fieldparams.MaxBlobsPerBlock),
			aa:     true,
		},
		{
			name:   "one present",
			count:  1,
			idxSet: idxUpTo(1),
			aa:     true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var mask blobIndexMask
			for _, idx := range c.idxSet {
				mask[idx] = true
			}
			sum := BlobStorageSummary{mask: mask}
			require.Equal(t, c.aa, sum.AllAvailable(c.count))
		})
	}
}

func TestHasDataColumnIndex(t *testing.T) {
	storedIndices := map[uint64]bool{
		1: true,
		3: true,
		5: true,
	}

	cases := []struct {
		name     string
		idx      uint64
		expected bool
	}{
		{
			name:     "index is too high",
			idx:      fieldparams.NumberOfColumns,
			expected: false,
		},
		{
			name:     "non existing index",
			idx:      2,
			expected: false,
		},
		{
			name:     "existing index",
			idx:      3,
			expected: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var mask blobIndexMask

			for idx := range storedIndices {
				mask[idx] = true
			}

			sum := BlobStorageSummary{mask: mask}
			require.Equal(t, c.expected, sum.HasDataColumnIndex(c.idx))
		})
	}
}

func TestAllDataColumnAvailable(t *testing.T) {
	tooManyColumns := make(map[uint64]bool, fieldparams.NumberOfColumns+1)
	for i := uint64(0); i < fieldparams.NumberOfColumns+1; i++ {
		tooManyColumns[i] = true
	}

	columns346 := map[uint64]bool{
		3: true,
		4: true,
		6: true,
	}

	columns36 := map[uint64]bool{
		3: true,
		6: true,
	}

	cases := []struct {
		name          string
		storedIndices map[uint64]bool
		testedIndices map[uint64]bool
		expected      bool
	}{
		{
			name:          "no tested indices",
			storedIndices: columns346,
			testedIndices: map[uint64]bool{},
			expected:      true,
		},
		{
			name:          "too many tested indices",
			storedIndices: columns346,
			testedIndices: tooManyColumns,
			expected:      false,
		},
		{
			name:          "not all tested indices are stored",
			storedIndices: columns36,
			testedIndices: columns346,
			expected:      false,
		},
		{
			name:          "all tested indices are stored",
			storedIndices: columns346,
			testedIndices: columns36,
			expected:      true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var mask blobIndexMask

			for idx := range c.storedIndices {
				mask[idx] = true
			}

			sum := BlobStorageSummary{mask: mask}
			require.Equal(t, c.expected, sum.AllDataColumnsAvailable(c.testedIndices))
		})
	}
}
