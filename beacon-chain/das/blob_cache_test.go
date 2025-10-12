package das

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

func TestCacheEnsureDelete(t *testing.T) {
	c := newBlobCache()
	require.Equal(t, 0, len(c.entries))
	root := bytesutil.ToBytes32([]byte("root"))
	slot := primitives.Slot(1234)
	k := cacheKey{root: root, slot: slot}
	entry := c.ensure(k)
	require.Equal(t, 1, len(c.entries))
	require.Equal(t, c.entries[k], entry)

	c.delete(k)
	require.Equal(t, 0, len(c.entries))
	var nilEntry *blobCacheEntry
	require.Equal(t, nilEntry, c.entries[k])
}

type filterTestCaseSetupFunc func(t *testing.T) (*blobCacheEntry, [][]byte, []blocks.ROBlob)

func filterTestCaseSetup(slot primitives.Slot, nBlobs int, onDisk []int, numExpected int) filterTestCaseSetupFunc {
	return func(t *testing.T) (*blobCacheEntry, [][]byte, []blocks.ROBlob) {
		blk, blobs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, slot, nBlobs)
		commits, err := commitmentsToCheck(blk, blk.Block().Slot())
		require.NoError(t, err)
		entry := &blobCacheEntry{}
		if len(onDisk) > 0 {
			od := map[[32]byte][]int{blk.Root(): onDisk}
			sumz := filesystem.NewMockBlobStorageSummarizer(t, slots.ToEpoch(slot), od)
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
		setup func(t *testing.T) (*blobCacheEntry, [][]byte, []blocks.ROBlob)
		err   error
	}{
		{
			name: "commitments mismatch - extra sidecar",
			setup: func(t *testing.T) (*blobCacheEntry, [][]byte, []blocks.ROBlob) {
				entry, commits, expected := filterTestCaseSetup(denebSlot, 6, []int{0, 1}, 4)(t)
				commits[5] = nil
				return entry, commits, expected
			},
			err: errCommitmentMismatch,
		},
		{
			name: "sidecar missing",
			setup: func(t *testing.T) (*blobCacheEntry, [][]byte, []blocks.ROBlob) {
				entry, commits, expected := filterTestCaseSetup(denebSlot, 6, []int{0, 1}, 4)(t)
				entry.scs[5] = nil
				return entry, commits, expected
			},
			err: errMissingSidecar,
		},
		{
			name: "commitments mismatch - different bytes",
			setup: func(t *testing.T) (*blobCacheEntry, [][]byte, []blocks.ROBlob) {
				entry, commits, expected := filterTestCaseSetup(denebSlot, 6, []int{0, 1}, 4)(t)
				entry.scs[5].KzgCommitment = []byte("nope")
				return entry, commits, expected
			},
			err: errCommitmentMismatch,
		},
		{
			name: "empty scs array with commitments",
			setup: func(t *testing.T) (*blobCacheEntry, [][]byte, []blocks.ROBlob) {
				// This reproduces the panic condition where entry.scs is empty or nil
				// but we have commitments to check
				entry := &blobCacheEntry{
					scs: nil, // Empty/nil array that caused the panic
				}
				// Create a commitment that would trigger the check at index 0
				commits := [][]byte{
					bytesutil.PadTo([]byte("commitment1"), 48),
				}
				return entry, commits, nil
			},
			err: errMissingSidecar,
		},
		{
			name: "scs array shorter than commitments",
			setup: func(t *testing.T) (*blobCacheEntry, [][]byte, []blocks.ROBlob) {
				// This reproduces the condition where entry.scs exists but is shorter
				// than the number of commitments we're checking
				entry := &blobCacheEntry{
					scs: make([]*blocks.ROBlob, 2), // Only 2 slots
				}
				// Create 4 commitments, accessing index 2 and 3 would have panicked
				commits := [][]byte{
					nil,
					nil,
					bytesutil.PadTo([]byte("commitment3"), 48),
					bytesutil.PadTo([]byte("commitment4"), 48),
				}
				return entry, commits, nil
			},
			err: errMissingSidecar,
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
