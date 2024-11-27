package filesystem

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestSlotByRoot_Summary(t *testing.T) {
	noneSet := make([]bool, params.BeaconConfig().MaxBlobsPerBlock(0))
	allSet := make([]bool, params.BeaconConfig().MaxBlobsPerBlock(0))
	firstSet := make([]bool, params.BeaconConfig().MaxBlobsPerBlock(0))
	lastSet := make([]bool, params.BeaconConfig().MaxBlobsPerBlock(0))
	oneSet := make([]bool, params.BeaconConfig().MaxBlobsPerBlock(0))
	firstSet[0] = true
	lastSet[len(lastSet)-1] = true
	oneSet[1] = true
	for i := range allSet {
		allSet[i] = true
	}
	cases := []struct {
		name     string
		root     [32]byte
		expected blobIndexMask
	}{
		{
			name: "not found",
		},
		{
			name:     "none set",
			expected: noneSet,
		},
		{
			name:     "index 1 set",
			expected: oneSet,
		},
		{
			name:     "all set",
			expected: allSet,
		},
		{
			name:     "first set",
			expected: firstSet,
		},
		{
			name:     "last set",
			expected: lastSet,
		},
	}
	sc := newBlobStorageCache()
	for _, c := range cases {
		if c.expected != nil {
			key := bytesutil.ToBytes32([]byte(c.name))
			sc.cache[key] = BlobStorageSummary{slot: 0, mask: c.expected}
		}
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key := bytesutil.ToBytes32([]byte(c.name))
			sum := sc.Summary(key)
			for i, has := range c.expected {
				ui := uint64(i)
				if c.expected == nil {
					require.Equal(t, false, sum.HasIndex(ui))
				} else {
					require.Equal(t, has, sum.HasIndex(ui))
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
			count: params.BeaconConfig().MaxBlobsPerBlock(0) + 1,
			aa:    false,
		},
		{
			name:   "max present",
			count:  params.BeaconConfig().MaxBlobsPerBlock(0),
			idxSet: idxUpTo(params.BeaconConfig().MaxBlobsPerBlock(0)),
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
			mask := make([]bool, params.BeaconConfig().MaxBlobsPerBlock(0))
			for _, idx := range c.idxSet {
				mask[idx] = true
			}
			sum := BlobStorageSummary{mask: mask}
			require.Equal(t, c.aa, sum.AllAvailable(c.count))
		})
	}
}
