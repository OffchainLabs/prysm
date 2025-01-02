package das

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func TestCacheEnsureDelete(t *testing.T) {
	c := newCache()
	require.Equal(t, 0, len(c.entries))
	root := bytesutil.ToBytes32([]byte("root"))
	slot := primitives.Slot(1234)
	k := cacheKey{root: root, slot: slot}
	entry := c.ensure(k)
	require.Equal(t, 1, len(c.entries))
	require.Equal(t, c.entries[k], entry)

	c.delete(k)
	require.Equal(t, 0, len(c.entries))
	var nilEntry *cacheEntry
	require.Equal(t, nilEntry, c.entries[k])
}

type filterTestCaseSetupFunc func(t *testing.T) (*cacheEntry, [][]byte, []blocks.ROBlob)

func filterTestCaseSetup(slot primitives.Slot, nBlobs int, onDisk []int, numExpected int) filterTestCaseSetupFunc {
	return func(t *testing.T) (*cacheEntry, [][]byte, []blocks.ROBlob) {
		blk, blobs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, slot, nBlobs)
		commits, err := commitmentsToCheck(blk, blk.Block().Slot())
		require.NoError(t, err)
		entry := &cacheEntry{}
		if len(onDisk) > 0 {
			od := map[[32]byte][]int{blk.Root(): onDisk}
			sumz := filesystem.NewMockBlobStorageSummarizer(t, od)
			sum := sumz.Summary(blk.Root())
			entry.setDiskSummary(sum)
		}
		expected := make([]blocks.ROBlob, 0, nBlobs)
		for i := 0; i < len(commits); i++ {
			if entry.diskSummary.HasIndex(uint64(i)) {
				continue
			}
			// If we aren't telling the cache a blob is on disk, add it to the expected list and stash.
			expected = append(expected, blobs[i])
			require.NoError(t, entry.stash(&blobs[i]))
		}
		require.Equal(t, numExpected, len(expected))
		return entry, commits, expected
	}
}

func TestFilterDiskSummary(t *testing.T) {
	denebSlot, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	require.NoError(t, err)
	cases := []struct {
		name  string
		setup filterTestCaseSetupFunc
	}{
		{
			name:  "full blobs, all on disk",
			setup: filterTestCaseSetup(denebSlot, 6, []int{0, 1, 2, 3, 4, 5}, 0),
		},
		{
			name:  "full blobs, first on disk",
			setup: filterTestCaseSetup(denebSlot, 6, []int{0}, 5),
		},
		{
			name:  "full blobs, middle on disk",
			setup: filterTestCaseSetup(denebSlot, 6, []int{2}, 5),
		},
		{
			name:  "full blobs, last on disk",
			setup: filterTestCaseSetup(denebSlot, 6, []int{5}, 5),
		},
		{
			name:  "full blobs, none on disk",
			setup: filterTestCaseSetup(denebSlot, 6, []int{}, 6),
		},
		{
			name:  "one commitment, on disk",
			setup: filterTestCaseSetup(denebSlot, 1, []int{0}, 0),
		},
		{
			name:  "one commitment, not on disk",
			setup: filterTestCaseSetup(denebSlot, 1, []int{}, 1),
		},
		{
			name:  "two commitments, first on disk",
			setup: filterTestCaseSetup(denebSlot, 2, []int{0}, 1),
		},
		{
			name:  "two commitments, last on disk",
			setup: filterTestCaseSetup(denebSlot, 2, []int{1}, 1),
		},
		{
			name:  "two commitments, none on disk",
			setup: filterTestCaseSetup(denebSlot, 2, []int{}, 2),
		},
		{
			name:  "two commitments, all on disk",
			setup: filterTestCaseSetup(denebSlot, 2, []int{0, 1}, 0),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entry, commits, expected := c.setup(t)
			// first (root) argument doesn't matter, it is just for logs
			got, err := entry.filter([32]byte{}, commits, 100)
			require.NoError(t, err)
			require.Equal(t, len(expected), len(got))
		})
	}
}

func TestFilter(t *testing.T) {
	denebSlot, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	require.NoError(t, err)
	cases := []struct {
		name  string
		setup func(t *testing.T) (*cacheEntry, [][]byte, []blocks.ROBlob)
		err   error
	}{
		{
			name: "commitments mismatch - extra sidecar",
			setup: func(t *testing.T) (*cacheEntry, [][]byte, []blocks.ROBlob) {
				entry, commits, expected := filterTestCaseSetup(denebSlot, 6, []int{0, 1}, 4)(t)
				commits[5] = nil
				return entry, commits, expected
			},
			err: errCommitmentMismatch,
		},
		{
			name: "sidecar missing",
			setup: func(t *testing.T) (*cacheEntry, [][]byte, []blocks.ROBlob) {
				entry, commits, expected := filterTestCaseSetup(denebSlot, 6, []int{0, 1}, 4)(t)
				entry.scs[5] = nil
				return entry, commits, expected
			},
			err: errMissingSidecar,
		},
		{
			name: "commitments mismatch - different bytes",
			setup: func(t *testing.T) (*cacheEntry, [][]byte, []blocks.ROBlob) {
				entry, commits, expected := filterTestCaseSetup(denebSlot, 6, []int{0, 1}, 4)(t)
				entry.scs[5].KzgCommitment = []byte("nope")
				return entry, commits, expected
			},
			err: errCommitmentMismatch,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entry, commits, expected := c.setup(t)
			// first (root) argument doesn't matter, it is just for logs
			got, err := entry.filter([32]byte{}, commits, 100)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, len(expected), len(got))
		})
	}
}
