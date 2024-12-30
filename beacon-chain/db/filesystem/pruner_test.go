package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path"
	"sort"
	"testing"
	"time"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/spf13/afero"
)

func TestTryPruneDir_CachedNotExpired(t *testing.T) {
	fs := afero.NewMemMapFs()
	pr, err := newBlobPruner(fs, 0)
	require.NoError(t, err)
	slot := pr.windowSize
	_, sidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, slot, params.BeaconConfig().MaxBlobsPerBlock(slot))
	sc, err := verification.BlobSidecarNoop(sidecars[0])
	require.NoError(t, err)
	rootStr := rootString(sc.BlockRoot())
	// This slot is right on the edge of what would need to be pruned, so by adding it to the cache and
	// skipping any other test setup, we can be certain the hot cache path never touches the filesystem.
	require.NoError(t, pr.cache.ensure(sc.BlockRoot(), sc.Slot(), 0))
	pruned, err := pr.tryPruneDir(rootStr, pr.windowSize)
	require.NoError(t, err)
	require.Equal(t, 0, pruned)
}

func TestCacheWarmFail(t *testing.T) {
	fs := afero.NewMemMapFs()
	n := blobNamer{root: bytesutil.ToBytes32([]byte("derp")), index: 0}
	bp := n.path()
	mkdir := path.Dir(bp)
	require.NoError(t, fs.MkdirAll(mkdir, directoryPermissions))

	// Create an empty blob index in the fs by touching the file at a seemingly valid path.
	fi, err := fs.Create(bp)
	require.NoError(t, err)
	require.NoError(t, fi.Close())

	// Cache warm should fail due to the unexpected EOF.
	pr, err := newBlobPruner(fs, 0)
	require.NoError(t, err)
	require.ErrorIs(t, pr.warmCache(), errPruningFailures)

	// The cache warm has finished, so calling waitForCache with a super short deadline
	// should not block or hit the context deadline.
	ctx := context.Background()
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(1*time.Millisecond))
	defer cancel()
	c, err := pr.waitForCache(ctx)
	// We will get an error and a nil value for the cache if we hit the deadline.
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestTryPruneDir_CachedExpired(t *testing.T) {
	t.Run("empty directory", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		pr, err := newBlobPruner(fs, 0)
		require.NoError(t, err)
		var slot primitives.Slot = 0
		_, sidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, slot, 1)
		sc, err := verification.BlobSidecarNoop(sidecars[0])
		require.NoError(t, err)
		rootStr := rootString(sc.BlockRoot())
		require.NoError(t, fs.Mkdir(rootStr, directoryPermissions)) // make empty directory
		require.NoError(t, pr.cache.ensure(sc.BlockRoot(), sc.Slot(), 0))
		pruned, err := pr.tryPruneDir(rootStr, slot+1)
		require.NoError(t, err)
		require.Equal(t, 0, pruned)
	})
	t.Run("blobs to delete", func(t *testing.T) {
		fs, bs := NewEphemeralBlobStorageWithFs(t)
		var slot primitives.Slot = 0
		_, sidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, slot, 2)
		scs, err := verification.BlobSidecarSliceNoop(sidecars)
		require.NoError(t, err)

		require.NoError(t, bs.Save(scs[0]))
		require.NoError(t, bs.Save(scs[1]))

		// check that the root->slot is cached
		root := scs[0].BlockRoot()
		rootStr := rootString(root)
		cs, cok := bs.pruner.cache.slot(scs[0].BlockRoot())
		require.Equal(t, true, cok)
		require.Equal(t, slot, cs)

		// ensure that we see the saved files in the filesystem
		files, err := listDir(fs, rootStr)
		require.NoError(t, err)
		require.Equal(t, 2, len(files))

		pruned, err := bs.pruner.tryPruneDir(rootStr, slot+1)
		require.NoError(t, err)
		require.Equal(t, 2, pruned)
		files, err = listDir(fs, rootStr)
		require.ErrorIs(t, err, os.ErrNotExist)
		require.Equal(t, 0, len(files))
	})
}

func TestTryPruneDir_SlotFromFile(t *testing.T) {
	t.Run("expired blobs deleted", func(t *testing.T) {
		fs, bs := NewEphemeralBlobStorageWithFs(t)
		var slot primitives.Slot = 0
		_, sidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, slot, 2)
		scs, err := verification.BlobSidecarSliceNoop(sidecars)
		require.NoError(t, err)

		require.NoError(t, bs.Save(scs[0]))
		require.NoError(t, bs.Save(scs[1]))

		// check that the root->slot is cached
		root := scs[0].BlockRoot()
		rootStr := rootString(root)
		cs, ok := bs.pruner.cache.slot(root)
		require.Equal(t, true, ok)
		require.Equal(t, slot, cs)
		// evict it from the cache so that we trigger the file read path
		bs.pruner.cache.evict(root)
		_, ok = bs.pruner.cache.slot(root)
		require.Equal(t, false, ok)

		// ensure that we see the saved files in the filesystem
		files, err := listDir(fs, rootStr)
		require.NoError(t, err)
		require.Equal(t, 2, len(files))

		pruned, err := bs.pruner.tryPruneDir(rootStr, slot+1)
		require.NoError(t, err)
		require.Equal(t, 2, pruned)
		files, err = listDir(fs, rootStr)
		require.ErrorIs(t, err, os.ErrNotExist)
		require.Equal(t, 0, len(files))
	})
	t.Run("not expired, intact", func(t *testing.T) {
		fs, bs := NewEphemeralBlobStorageWithFs(t)
		// Set slot equal to the window size, so it should be retained.
		slot := bs.pruner.windowSize
		_, sidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, slot, 2)
		scs, err := verification.BlobSidecarSliceNoop(sidecars)
		require.NoError(t, err)

		require.NoError(t, bs.Save(scs[0]))
		require.NoError(t, bs.Save(scs[1]))

		// Evict slot mapping from the cache so that we trigger the file read path.
		root := scs[0].BlockRoot()
		rootStr := rootString(root)
		bs.pruner.cache.evict(root)
		_, ok := bs.pruner.cache.slot(root)
		require.Equal(t, false, ok)

		// Ensure that we see the saved files in the filesystem.
		files, err := listDir(fs, rootStr)
		require.NoError(t, err)
		require.Equal(t, 2, len(files))

		// This should use the slotFromFile code (simulating restart).
		// Setting pruneBefore == slot, so that the slot will be outside the window (at the boundary).
		pruned, err := bs.pruner.tryPruneDir(rootStr, slot)
		require.NoError(t, err)
		require.Equal(t, 0, pruned)

		// Ensure files are still present.
		files, err = listDir(fs, rootStr)
		require.NoError(t, err)
		require.Equal(t, 2, len(files))
	})
}

func TestSlotFromBlob(t *testing.T) {
	cases := []struct {
		slot primitives.Slot
	}{
		{slot: 0},
		{slot: 2},
		{slot: 1123581321},
		{slot: math.MaxUint64},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("slot %d", c.slot), func(t *testing.T) {
			_, sidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, c.slot, 1)
			sc := sidecars[0]
			enc, err := sc.MarshalSSZ()
			require.NoError(t, err)
			slot, err := slotFromBlob(bytes.NewReader(enc))
			require.NoError(t, err)
			require.Equal(t, c.slot, slot)
		})
	}
}

func TestSlotFromFile(t *testing.T) {
	cases := []struct {
		slot primitives.Slot
	}{
		{slot: 0},
		{slot: 2},
		{slot: 1123581321},
		{slot: math.MaxUint64},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("slot %d", c.slot), func(t *testing.T) {
			fs, bs := NewEphemeralBlobStorageWithFs(t)
			_, sidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, c.slot, 1)
			sc, err := verification.BlobSidecarNoop(sidecars[0])
			require.NoError(t, err)
			require.NoError(t, bs.Save(sc))
			fname := namerForSidecar(sc)
			sszPath := fname.path()
			slot, err := slotFromFile(sszPath, fs)
			require.NoError(t, err)
			require.Equal(t, c.slot, slot)
		})
	}
}

type dirFiles struct {
	name     string
	isDir    bool
	children []dirFiles
}

func (df dirFiles) reify(t *testing.T, fs afero.Fs, base string) {
	fullPath := path.Join(base, df.name)
	if df.isDir {
		if df.name != "" {
			require.NoError(t, fs.Mkdir(fullPath, directoryPermissions))
		}
		for _, c := range df.children {
			c.reify(t, fs, fullPath)
		}
	} else {
		fp, err := fs.Create(fullPath)
		require.NoError(t, err)
		_, err = fp.WriteString("derp")
		require.NoError(t, err)
	}
}

func (df dirFiles) childNames() []string {
	cn := make([]string, len(df.children))
	for i := range df.children {
		cn[i] = df.children[i].name
	}
	return cn
}

func TestListDir(t *testing.T) {
	fs := afero.NewMemMapFs()

	// parent directory
	fsLayout := dirFiles{isDir: true}
	// break out each subdir for easier assertions
	notABlob := dirFiles{name: "notABlob", isDir: true}
	childlessBlob := dirFiles{name: "0x0987654321", isDir: true}
	blobWithSsz := dirFiles{name: "0x1123581321", isDir: true,
		children: []dirFiles{{name: "1.ssz"}, {name: "2.ssz"}},
	}
	blobWithSszAndTmp := dirFiles{name: "0x1234567890", isDir: true,
		children: []dirFiles{{name: "5.ssz"}, {name: "0.part"}}}
	fsLayout.children = append(fsLayout.children,
		notABlob, childlessBlob, blobWithSsz, blobWithSszAndTmp)

	topChildren := make([]string, len(fsLayout.children))
	for i := range fsLayout.children {
		topChildren[i] = fsLayout.children[i].name
	}

	fsLayout.reify(t, fs, "")
	cases := []struct {
		name     string
		dirPath  string
		expected []string
		filter   func(string) bool
		err      error
	}{
		{
			name:     "non-existent",
			dirPath:  "derp",
			expected: []string{},
			err:      os.ErrNotExist,
		},
		{
			name:     "empty",
			dirPath:  childlessBlob.name,
			expected: []string{},
		},
		{
			name:     "top",
			dirPath:  ".",
			expected: topChildren,
		},
		{
			name:     "custom filter: only notABlob",
			dirPath:  ".",
			expected: []string{notABlob.name},
			filter: func(s string) bool {
				return s == notABlob.name
			},
		},
		{
			name:     "root filter",
			dirPath:  ".",
			expected: []string{childlessBlob.name, blobWithSsz.name, blobWithSszAndTmp.name},
			filter:   filterRoot,
		},
		{
			name:     "ssz filter",
			dirPath:  blobWithSsz.name,
			expected: blobWithSsz.childNames(),
			filter:   filterSsz,
		},
		{
			name:     "ssz mixed filter",
			dirPath:  blobWithSszAndTmp.name,
			expected: []string{"5.ssz"},
			filter:   filterSsz,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := listDir(fs, c.dirPath)
			if c.filter != nil {
				result = filter(result, c.filter)
			}
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				require.Equal(t, 0, len(result))
			} else {
				require.NoError(t, err)
				sort.Strings(c.expected)
				sort.Strings(result)
				require.DeepEqual(t, c.expected, result)
			}
		})
	}
}

func TestRootFromDir(t *testing.T) {
	cases := []struct {
		name string
		dir  string
		err  error
		root [32]byte
	}{
		{
			name: "happy path",
			dir:  "0xffff875e1d985c5ccb214894983f2428edb271f0f87b68ba7010e4a99df3b5cb",
			root: [32]byte{255, 255, 135, 94, 29, 152, 92, 92, 203, 33, 72, 148, 152, 63, 36, 40,
				237, 178, 113, 240, 248, 123, 104, 186, 112, 16, 228, 169, 157, 243, 181, 203},
		},
		{
			name: "too short",
			dir:  "0xffff875e1d985c5ccb214894983f2428edb271f0f87b68ba7010e4a99df3b5c",
			err:  errInvalidRootString,
		},
		{
			name: "too log",
			dir:  "0xffff875e1d985c5ccb214894983f2428edb271f0f87b68ba7010e4a99df3b5cbb",
			err:  errInvalidRootString,
		},
		{
			name: "missing prefix",
			dir:  "ffff875e1d985c5ccb214894983f2428edb271f0f87b68ba7010e4a99df3b5cb",
			err:  errInvalidRootString,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, err := stringToRoot(c.dir)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.root, root)
		})
	}
}
