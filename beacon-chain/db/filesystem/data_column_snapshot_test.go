package filesystem

import (
	"encoding/binary"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/spf13/afero"
)

// newSnapshotTestStorage returns a warmed data column storage, backed by `dir`, containing some data column sidecars.
func newSnapshotTestStorage(t *testing.T, dir string) (*DataColumnStorage, []blocks.VerifiedRODataColumn) {
	storage, err := NewDataColumnStorage(t.Context(), WithDataColumnBasePath(dir), WithDataColumnRetentionEpochs(10_000))
	require.NoError(t, err)

	_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
		t,
		[]util.DataColumnParam{
			{Slot: 33, Index: 2, Column: [][]byte{{1}, {2}, {3}}},       // Period 0 - Epoch 1
			{Slot: 33, Index: 127, Column: [][]byte{{2}, {3}, {4}}},     // Period 0 - Epoch 1
			{Slot: 128_002, Index: 0, Column: [][]byte{{1}, {2}, {3}}},  // Period 0 - Epoch 4000
			{Slot: 128_002, Index: 64, Column: [][]byte{{2}, {3}, {4}}}, // Period 0 - Epoch 4000
			{Slot: 131_138, Index: 9, Column: [][]byte{{1}, {2}, {3}}},  // Period 1 - Epoch 4098
		},
	)

	require.NoError(t, storage.Save(verifiedRoDataColumnSidecars))
	storage.WarmCache()

	return storage, verifiedRoDataColumnSidecars
}

func requireSameCache(t *testing.T, expected, actual *dataColumnStorageSummaryCache) {
	require.DeepEqual(t, expected.cache, actual.cache)
	require.Equal(t, expected.lowestCachedEpoch, actual.lowestCachedEpoch)
	require.Equal(t, expected.highestCachedEpoch, actual.highestCachedEpoch)
	require.Equal(t, expected.dataColumnCount, actual.dataColumnCount)
}

// corruptSnapshot writes a snapshot with `storage`, then applies `corrupt` on its content.
func corruptSnapshot(t *testing.T, storage *DataColumnStorage, corrupt func([]byte) []byte) {
	require.NoError(t, storage.Close())

	content, err := afero.ReadFile(storage.fs, snapshotFileName)
	require.NoError(t, err)
	require.NoError(t, afero.WriteFile(storage.fs, snapshotFileName, corrupt(content), snapshotFilePermissions))
}

func TestClose(t *testing.T) {
	t.Run("not warmed", func(t *testing.T) {
		storage, err := NewDataColumnStorage(t.Context(), WithDataColumnBasePath(t.TempDir()))
		require.NoError(t, err)

		require.NoError(t, storage.Close())

		exists, err := afero.Exists(storage.fs, snapshotFileName)
		require.NoError(t, err)
		require.Equal(t, false, exists)
	})

	t.Run("nominal", func(t *testing.T) {
		storage, verifiedRoDataColumnSidecars := newSnapshotTestStorage(t, t.TempDir())

		require.NoError(t, storage.Close())

		exists, err := afero.Exists(storage.fs, snapshotFileName)
		require.NoError(t, err)
		require.Equal(t, true, exists)

		exists, err = afero.Exists(storage.fs, snapshotTmpFileName)
		require.NoError(t, err)
		require.Equal(t, false, exists)

		// Closing again is a no-op.
		require.NoError(t, storage.Close())

		// Nothing can be written after close.
		require.ErrorIs(t, storage.Save(verifiedRoDataColumnSidecars), errDataColumnStorageClosed)
		require.ErrorIs(t, storage.Remove(verifiedRoDataColumnSidecars[0].BlockRoot()), errDataColumnStorageClosed)

		// Reads still work after close.
		dataColumnSidecars, err := storage.Get(verifiedRoDataColumnSidecars[0].BlockRoot(), nil)
		require.NoError(t, err)
		require.Equal(t, 2, len(dataColumnSidecars))
	})
}

func TestConsumeSnapshot(t *testing.T) {
	t.Run("no snapshot", func(t *testing.T) {
		storage, err := NewDataColumnStorage(t.Context(), WithDataColumnBasePath(t.TempDir()))
		require.NoError(t, err)

		require.Equal(t, false, storage.consumeSnapshot())
	})

	t.Run("nominal", func(t *testing.T) {
		dir := t.TempDir()
		storage, _ := newSnapshotTestStorage(t, dir)
		require.NoError(t, storage.Close())

		restarted, err := NewDataColumnStorage(t.Context(), WithDataColumnBasePath(dir), WithDataColumnRetentionEpochs(10_000))
		require.NoError(t, err)

		require.Equal(t, true, restarted.consumeSnapshot())
		requireSameCache(t, storage.cache, restarted.cache)

		// The snapshot is consumed.
		exists, err := afero.Exists(restarted.fs, snapshotFileName)
		require.NoError(t, err)
		require.Equal(t, false, exists)
	})

	t.Run("leftover temporary snapshot", func(t *testing.T) {
		storage, err := NewDataColumnStorage(t.Context(), WithDataColumnBasePath(t.TempDir()))
		require.NoError(t, err)
		require.NoError(t, afero.WriteFile(storage.fs, snapshotTmpFileName, []byte{1, 2, 3}, snapshotFilePermissions))

		require.Equal(t, false, storage.consumeSnapshot())

		exists, err := afero.Exists(storage.fs, snapshotTmpFileName)
		require.NoError(t, err)
		require.Equal(t, false, exists)
	})

	t.Run("corrupted snapshot", func(t *testing.T) {
		dir := t.TempDir()
		storage, _ := newSnapshotTestStorage(t, dir)
		corruptSnapshot(t, storage, func(content []byte) []byte { return content[:len(content)-1] })

		restarted, err := NewDataColumnStorage(t.Context(), WithDataColumnBasePath(dir), WithDataColumnRetentionEpochs(10_000))
		require.NoError(t, err)

		require.Equal(t, false, restarted.consumeSnapshot())
		require.Equal(t, 0, len(restarted.cache.cache))

		// The corrupted snapshot is removed.
		exists, err := afero.Exists(restarted.fs, snapshotFileName)
		require.NoError(t, err)
		require.Equal(t, false, exists)
	})
}

func TestLoadSnapshot(t *testing.T) {
	testCases := []struct {
		name     string
		corrupt  func([]byte) []byte
		expected error
	}{
		{
			name:     "wrong magic",
			corrupt:  func(content []byte) []byte { content[0] ^= 0xff; return content },
			expected: errSnapshotWrongMagic,
		},
		{
			name: "wrong version",
			corrupt: func(content []byte) []byte {
				binary.LittleEndian.PutUint16(content[snapshotVersionOffset:], snapshotFormatVersion+1)
				return content
			},
			expected: errSnapshotWrongVersion,
		},
		{
			name:     "truncated",
			corrupt:  func(content []byte) []byte { return content[:len(content)-1] },
			expected: errSnapshotWrongSize,
		},
		{
			name: "huge count",
			corrupt: func(content []byte) []byte {
				binary.LittleEndian.PutUint64(content[snapshotCountOffset:], 1<<62)
				return content
			},
			expected: errSnapshotWrongSize,
		},
		{
			name: "wrong checksum",
			corrupt: func(content []byte) []byte {
				content[snapshotHeaderSize] ^= 0xff
				return content
			},
			expected: errSnapshotWrongChecksum,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			storage, _ := newSnapshotTestStorage(t, dir)
			corruptSnapshot(t, storage, tc.corrupt)

			restarted, err := NewDataColumnStorage(t.Context(), WithDataColumnBasePath(dir))
			require.NoError(t, err)

			require.ErrorIs(t, restarted.loadSnapshot(), tc.expected)
		})
	}

	t.Run("empty cache", func(t *testing.T) {
		dir := t.TempDir()
		storage, err := NewDataColumnStorage(t.Context(), WithDataColumnBasePath(dir))
		require.NoError(t, err)
		storage.WarmCache()
		require.NoError(t, storage.Close())

		restarted, err := NewDataColumnStorage(t.Context(), WithDataColumnBasePath(dir))
		require.NoError(t, err)

		require.NoError(t, restarted.loadSnapshot())
		require.Equal(t, 0, len(restarted.cache.cache))
	})
}
