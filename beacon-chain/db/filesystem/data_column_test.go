package filesystem

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/spf13/afero"
)

func TestNewDataColumnStorage(t *testing.T) {
	ctx := t.Context()

	t.Run("No base path", func(t *testing.T) {
		_, err := NewDataColumnStorage(ctx)
		require.ErrorIs(t, err, errNoDataColumnBasePath)
	})

	t.Run("Nominal", func(t *testing.T) {
		dir := t.TempDir()

		storage, err := NewDataColumnStorage(ctx, WithDataColumnBasePath(dir))
		require.NoError(t, err)
		require.Equal(t, dir, storage.base)
	})
}

func TestWarmCache(t *testing.T) {
	storage, err := NewDataColumnStorage(
		t.Context(),
		WithDataColumnBasePath(t.TempDir()),
		WithDataColumnRetentionEpochs(10_000),
	)
	require.NoError(t, err)

	_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
		t,
		[]util.DataColumnParam{
			{Slot: 33, Index: 2, Column: [][]byte{{1}, {2}, {3}}},      // Period 0 - Epoch 1
			{Slot: 33, Index: 4, Column: [][]byte{{2}, {3}, {4}}},      // Period 0 - Epoch 1
			{Slot: 128_002, Index: 2, Column: [][]byte{{1}, {2}, {3}}}, // Period 0 - Epoch 4000
			{Slot: 128_002, Index: 4, Column: [][]byte{{2}, {3}, {4}}}, // Period 0 - Epoch 4000
			{Slot: 128_003, Index: 1, Column: [][]byte{{1}, {2}, {3}}}, // Period 0 - Epoch 4000
			{Slot: 128_003, Index: 3, Column: [][]byte{{2}, {3}, {4}}}, // Period 0 - Epoch 4000
			{Slot: 128_034, Index: 2, Column: [][]byte{{1}, {2}, {3}}}, // Period 0 - Epoch 4001
			{Slot: 128_034, Index: 4, Column: [][]byte{{2}, {3}, {4}}}, // Period 0 - Epoch 4001
			{Slot: 131_138, Index: 2, Column: [][]byte{{1}, {2}, {3}}}, // Period 1 - Epoch 4098
			{Slot: 131_138, Index: 1, Column: [][]byte{{1}, {2}, {3}}}, // Period 1 - Epoch 4098
			{Slot: 131_168, Index: 0, Column: [][]byte{{1}, {2}, {3}}}, // Period 1 - Epoch 4099
		},
	)

	err = storage.Save(verifiedRoDataColumnSidecars)
	require.NoError(t, err)

	storage.retentionEpochs = 4_096

	storage.WarmCache()
	require.Equal(t, primitives.Epoch(4_000), storage.cache.lowestCachedEpoch)
	require.Equal(t, 5, len(storage.cache.cache))

	summary, ok := storage.cache.get(verifiedRoDataColumnSidecars[2].BlockRoot())
	require.Equal(t, true, ok)
	require.DeepEqual(t, DataColumnStorageSummary{epoch: 4_000, mask: [fieldparams.NumberOfColumns]bool{false, false, true, false, true}}, summary)

	summary, ok = storage.cache.get(verifiedRoDataColumnSidecars[4].BlockRoot())
	require.Equal(t, true, ok)
	require.DeepEqual(t, DataColumnStorageSummary{epoch: 4_000, mask: [fieldparams.NumberOfColumns]bool{false, true, false, true}}, summary)

	summary, ok = storage.cache.get(verifiedRoDataColumnSidecars[6].BlockRoot())
	require.Equal(t, true, ok)
	require.DeepEqual(t, DataColumnStorageSummary{epoch: 4_001, mask: [fieldparams.NumberOfColumns]bool{false, false, true, false, true}}, summary)

	summary, ok = storage.cache.get(verifiedRoDataColumnSidecars[8].BlockRoot())
	require.Equal(t, true, ok)
	require.DeepEqual(t, DataColumnStorageSummary{epoch: 4_098, mask: [fieldparams.NumberOfColumns]bool{false, true, true}}, summary)

	summary, ok = storage.cache.get(verifiedRoDataColumnSidecars[10].BlockRoot())
	require.Equal(t, true, ok)
	require.DeepEqual(t, DataColumnStorageSummary{epoch: 4_099, mask: [fieldparams.NumberOfColumns]bool{true}}, summary)
}

func TestSaveDataColumnsSidecars(t *testing.T) {
	t.Run("wrong numbers of columns", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.NumberOfColumns = 0
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)

		_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{{Index: 12}, {Index: 1_000_000}, {Index: 48}},
		)

		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err := dataColumnStorage.Save(verifiedRoDataColumnSidecars)
		require.ErrorIs(t, err, errWrongNumberOfColumns)
	})

	t.Run("one of the column index is too large", func(t *testing.T) {
		_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{{Index: 12}, {Index: 1_000_000}, {Index: 48}},
		)

		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err := dataColumnStorage.Save(verifiedRoDataColumnSidecars)
		require.ErrorIs(t, err, errDataColumnIndexTooLarge)
	})

	t.Run("different slots", func(t *testing.T) {
		_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Slot: 1, Index: 12, Column: [][]byte{{1}, {2}, {3}}},
				{Slot: 2, Index: 12, Column: [][]byte{{1}, {2}, {3}}},
			},
		)

		// Create a sidecar with a different slot but the same root.
		alteredVerifiedRoDataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, 2)
		alteredVerifiedRoDataColumnSidecars = append(alteredVerifiedRoDataColumnSidecars, verifiedRoDataColumnSidecars[0])

		altered, err := blocks.NewRODataColumnWithRoot(
			verifiedRoDataColumnSidecars[1].RODataColumn.DataColumnSidecar,
			verifiedRoDataColumnSidecars[0].BlockRoot(),
		)
		require.NoError(t, err)

		verifiedAltered := blocks.NewVerifiedRODataColumn(altered)
		alteredVerifiedRoDataColumnSidecars = append(alteredVerifiedRoDataColumnSidecars, verifiedAltered)

		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err = dataColumnStorage.Save(alteredVerifiedRoDataColumnSidecars)
		require.ErrorIs(t, err, errDataColumnSidecarsFromDifferentSlots)
	})

	t.Run("new file - no data columns to save", func(t *testing.T) {
		_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{},
		)

		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err := dataColumnStorage.Save(verifiedRoDataColumnSidecars)
		require.NoError(t, err)
	})

	t.Run("new file - different data column size", func(t *testing.T) {
		_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Slot: 1, Index: 12, Column: [][]byte{{1}, {2}, {3}}},
				{Slot: 1, Index: 13, Column: [][]byte{{1}, {2}, {3}, {4}}},
			},
		)

		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err := dataColumnStorage.Save(verifiedRoDataColumnSidecars)
		require.ErrorIs(t, err, errWrongSszEncodedDataColumnSidecarSize)
	})

	t.Run("existing file - wrong incoming SSZ encoded size", func(t *testing.T) {
		_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Slot: 1, Index: 12, Column: [][]byte{{1}, {2}, {3}}},
			},
		)

		// Save data columns into a file.
		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err := dataColumnStorage.Save(verifiedRoDataColumnSidecars)
		require.NoError(t, err)

		// Build a data column sidecar for the same block but with a different
		// column index and an different SSZ encoded size.
		_, verifiedRoDataColumnSidecars = util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Slot: 1, Index: 13, Column: [][]byte{{1}, {2}, {3}, {4}}},
			},
		)

		// Try to rewrite the file.
		err = dataColumnStorage.Save(verifiedRoDataColumnSidecars)
		require.ErrorIs(t, err, errWrongSszEncodedDataColumnSidecarSize)
	})

	t.Run("nominal", func(t *testing.T) {
		_, inputVerifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Slot: 1, Index: 12, Column: [][]byte{{1}, {2}, {3}}},
				{Slot: 1, Index: 11, Column: [][]byte{{3}, {4}, {5}}},
				{Slot: 1, Index: 12, Column: [][]byte{{1}, {2}, {3}}}, // OK if duplicate
				{Slot: 1, Index: 13, Column: [][]byte{{6}, {7}, {8}}},
				{Slot: 2, Index: 12, Column: [][]byte{{3}, {4}, {5}}},
				{Slot: 2, Index: 13, Column: [][]byte{{6}, {7}, {8}}},
			},
		)

		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err := dataColumnStorage.Save(inputVerifiedRoDataColumnSidecars)
		require.NoError(t, err)

		_, inputVerifiedRoDataColumnSidecars = util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Slot: 1, Index: 12, Column: [][]byte{{1}, {2}, {3}}}, // OK if duplicate
				{Slot: 1, Index: 15, Column: [][]byte{{2}, {3}, {4}}},
				{Slot: 1, Index: 1, Column: [][]byte{{2}, {3}, {4}}},
				{Slot: 3, Index: 6, Column: [][]byte{{3}, {4}, {5}}},
				{Slot: 3, Index: 2, Column: [][]byte{{6}, {7}, {8}}},
			},
		)

		err = dataColumnStorage.Save(inputVerifiedRoDataColumnSidecars)
		require.NoError(t, err)

		type fixture struct {
			fileName         string
			expectedIndices  [mandatoryNumberOfColumns]byte
			dataColumnParams []util.DataColumnParam
		}

		fixtures := []fixture{
			{
				fileName: "0/0/0x8bb2f09de48c102635622dc27e6de03ae2b22639df7c33edbc8222b2ec423746.sszs",
				expectedIndices: [mandatoryNumberOfColumns]byte{
					0, nonZeroOffset + 4, 0, 0, 0, 0, 0, 0,
					0, 0, 0, nonZeroOffset + 1, nonZeroOffset, nonZeroOffset + 2, 0, nonZeroOffset + 3,
					// The rest is filled with zeroes.
				},
				dataColumnParams: []util.DataColumnParam{
					{Slot: 1, Index: 12, Column: [][]byte{{1}, {2}, {3}}},
					{Slot: 1, Index: 11, Column: [][]byte{{3}, {4}, {5}}},
					{Slot: 1, Index: 13, Column: [][]byte{{6}, {7}, {8}}},
					{Slot: 1, Index: 15, Column: [][]byte{{2}, {3}, {4}}},
					{Slot: 1, Index: 1, Column: [][]byte{{2}, {3}, {4}}},
				},
			},
			{
				fileName: "0/0/0x221f88cae2219050d4e9d8c2d0d83cb4c8ce4c84ab1bb3e0b89f3dec36077c4f.sszs",
				expectedIndices: [mandatoryNumberOfColumns]byte{
					0, 0, 0, 0, 0, 0, 0, 0,
					0, 0, 0, 0, nonZeroOffset, nonZeroOffset + 1, 0, 0,
					// The rest is filled with zeroes.
				},
				dataColumnParams: []util.DataColumnParam{
					{Slot: 2, Index: 12, Column: [][]byte{{3}, {4}, {5}}},
					{Slot: 2, Index: 13, Column: [][]byte{{6}, {7}, {8}}},
				},
			},
			{
				fileName: "0/0/0x7b163bd57e1c4c8b5048c5389698098f4c957d62d7ce86f4ffa9bdc75c16a18b.sszs",
				expectedIndices: [mandatoryNumberOfColumns]byte{
					0, 0, nonZeroOffset + 1, 0, 0, 0, nonZeroOffset, 0,
					// The rest is filled with zeroes.
				},
				dataColumnParams: []util.DataColumnParam{
					{Slot: 3, Index: 6, Column: [][]byte{{3}, {4}, {5}}},
					{Slot: 3, Index: 2, Column: [][]byte{{6}, {7}, {8}}},
				},
			},
		}

		for _, fixture := range fixtures {
			// Build expected data column sidecars.
			_, expectedDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
				t,
				fixture.dataColumnParams,
			)

			// Build expected bytes.
			firstSszEncodedDataColumnSidecar, err := expectedDataColumnSidecars[0].MarshalSSZ()
			require.NoError(t, err)

			dataColumnSidecarsCount := len(expectedDataColumnSidecars)
			sszEncodedDataColumnSidecarSize := len(firstSszEncodedDataColumnSidecar)

			sszEncodedDataColumnSidecars := make([]byte, 0, dataColumnSidecarsCount*sszEncodedDataColumnSidecarSize)
			sszEncodedDataColumnSidecars = append(sszEncodedDataColumnSidecars, firstSszEncodedDataColumnSidecar...)
			for _, dataColumnSidecar := range expectedDataColumnSidecars[1:] {
				sszEncodedDataColumnSidecar, err := dataColumnSidecar.MarshalSSZ()
				require.NoError(t, err)
				sszEncodedDataColumnSidecars = append(sszEncodedDataColumnSidecars, sszEncodedDataColumnSidecar...)
			}

			var encodedSszEncodedDataColumnSidecarSize [sidecarByteLenSize]byte
			binary.BigEndian.PutUint32(encodedSszEncodedDataColumnSidecarSize[:], uint32(sszEncodedDataColumnSidecarSize))

			expectedBytes := make([]byte, 0, headerSize+dataColumnSidecarsCount*sszEncodedDataColumnSidecarSize)
			expectedBytes = append(expectedBytes, []byte{0x01}...)
			expectedBytes = append(expectedBytes, encodedSszEncodedDataColumnSidecarSize[:]...)
			expectedBytes = append(expectedBytes, fixture.expectedIndices[:]...)
			expectedBytes = append(expectedBytes, sszEncodedDataColumnSidecars...)

			blockRoot := expectedDataColumnSidecars[0].BlockRoot()

			// Check the actual content of the file.
			actualBytes, err := afero.ReadFile(dataColumnStorage.fs, fixture.fileName)
			require.NoError(t, err)
			require.DeepSSZEqual(t, expectedBytes, actualBytes)

			// Check the summary.
			indices := map[uint64]bool{}
			for _, dataColumnParam := range fixture.dataColumnParams {
				indices[dataColumnParam.Index] = true
			}

			summary := dataColumnStorage.Summary(blockRoot)
			for index := range uint64(mandatoryNumberOfColumns) {
				require.Equal(t, indices[index], summary.HasIndex(index))
			}

			err = dataColumnStorage.Remove(blockRoot)
			require.NoError(t, err)

			summary = dataColumnStorage.Summary(blockRoot)
			for index := range uint64(mandatoryNumberOfColumns) {
				require.Equal(t, false, summary.HasIndex(index))
			}

			_, err = afero.ReadFile(dataColumnStorage.fs, fixture.fileName)
			require.ErrorIs(t, err, os.ErrNotExist)
		}
	})
}

func TestGetDataColumnSidecars(t *testing.T) {
	t.Run("root not found", func(t *testing.T) {
		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)

		verifiedRODataColumnSidecars, err := dataColumnStorage.Get([fieldparams.RootLength]byte{1}, []uint64{12, 13, 14})
		require.NoError(t, err)
		require.Equal(t, 0, len(verifiedRODataColumnSidecars))
	})

	t.Run("indices not found", func(t *testing.T) {
		_, savedVerifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Index: 12, Column: [][]byte{{1}, {2}, {3}}},
				{Index: 14, Column: [][]byte{{2}, {3}, {4}}},
			},
		)

		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err := dataColumnStorage.Save(savedVerifiedRoDataColumnSidecars)
		require.NoError(t, err)

		verifiedRODataColumnSidecars, err := dataColumnStorage.Get(savedVerifiedRoDataColumnSidecars[0].BlockRoot(), []uint64{3, 1, 2})
		require.NoError(t, err)
		require.Equal(t, 0, len(verifiedRODataColumnSidecars))
	})

	t.Run("nominal", func(t *testing.T) {
		_, expectedVerifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Index: 12, Column: [][]byte{{1}, {2}, {3}}},
				{Index: 14, Column: [][]byte{{2}, {3}, {4}}},
			},
		)

		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err := dataColumnStorage.Save(expectedVerifiedRoDataColumnSidecars)
		require.NoError(t, err)

		root := expectedVerifiedRoDataColumnSidecars[0].BlockRoot()

		verifiedRODataColumnSidecars, err := dataColumnStorage.Get(root, nil)
		require.NoError(t, err)
		require.DeepSSZEqual(t, expectedVerifiedRoDataColumnSidecars, verifiedRODataColumnSidecars)

		verifiedRODataColumnSidecars, err = dataColumnStorage.Get(root, []uint64{12, 13, 14})
		require.NoError(t, err)
		require.DeepSSZEqual(t, expectedVerifiedRoDataColumnSidecars, verifiedRODataColumnSidecars)
	})
}

func TestRemove(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err := dataColumnStorage.Remove([fieldparams.RootLength]byte{1})
		require.NoError(t, err)
	})

	t.Run("nominal", func(t *testing.T) {
		_, inputVerifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Slot: 32, Index: 10, Column: [][]byte{{1}, {2}, {3}}},
				{Slot: 32, Index: 11, Column: [][]byte{{2}, {3}, {4}}},
				{Slot: 33, Index: 10, Column: [][]byte{{1}, {2}, {3}}},
				{Slot: 33, Index: 11, Column: [][]byte{{2}, {3}, {4}}},
			},
		)

		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err := dataColumnStorage.Save(inputVerifiedRoDataColumnSidecars)
		require.NoError(t, err)

		err = dataColumnStorage.Remove(inputVerifiedRoDataColumnSidecars[0].BlockRoot())
		require.NoError(t, err)

		summary := dataColumnStorage.Summary(inputVerifiedRoDataColumnSidecars[0].BlockRoot())
		require.Equal(t, primitives.Epoch(0), summary.epoch)
		require.Equal(t, uint64(0), summary.Count())

		summary = dataColumnStorage.Summary(inputVerifiedRoDataColumnSidecars[3].BlockRoot())
		require.Equal(t, primitives.Epoch(1), summary.epoch)
		require.Equal(t, uint64(2), summary.Count())

		actual, err := dataColumnStorage.Get(inputVerifiedRoDataColumnSidecars[0].BlockRoot(), nil)
		require.NoError(t, err)
		require.Equal(t, 0, len(actual))

		actual, err = dataColumnStorage.Get(inputVerifiedRoDataColumnSidecars[3].BlockRoot(), nil)
		require.NoError(t, err)
		require.Equal(t, 2, len(actual))
	})
}

func TestClear(t *testing.T) {
	_, inputVerifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
		t,
		[]util.DataColumnParam{
			{Slot: 1, Index: 12, Column: [][]byte{{1}, {2}, {3}}},
			{Slot: 2, Index: 13, Column: [][]byte{{6}, {7}, {8}}},
		},
	)

	_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
	err := dataColumnStorage.Save(inputVerifiedRoDataColumnSidecars)
	require.NoError(t, err)

	filePaths := []string{
		"0/0/0x8bb2f09de48c102635622dc27e6de03ae2b22639df7c33edbc8222b2ec423746.sszs",
		"0/0/0x221f88cae2219050d4e9d8c2d0d83cb4c8ce4c84ab1bb3e0b89f3dec36077c4f.sszs",
	}

	for _, filePath := range filePaths {
		_, err = afero.ReadFile(dataColumnStorage.fs, filePath)
		require.NoError(t, err)
	}

	err = dataColumnStorage.Clear()
	require.NoError(t, err)

	summary := dataColumnStorage.Summary([fieldparams.RootLength]byte{1})
	for index := range uint64(mandatoryNumberOfColumns) {
		require.Equal(t, false, summary.HasIndex(index))
	}

	for _, filePath := range filePaths {
		_, err = afero.ReadFile(dataColumnStorage.fs, filePath)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}

func TestMetadata(t *testing.T) {
	t.Run("wrong version", func(t *testing.T) {
		_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Slot: 1, Index: 12, Column: [][]byte{{1}, {2}, {3}}},
			},
		)

		// Save data columns into a file.
		_, dataColumnStorage := NewEphemeralDataColumnStorageAndFs(t)
		err := dataColumnStorage.Save(verifiedRoDataColumnSidecars)
		require.NoError(t, err)

		// Alter the version.
		const filePath = "0/0/0x8bb2f09de48c102635622dc27e6de03ae2b22639df7c33edbc8222b2ec423746.sszs"
		file, err := dataColumnStorage.fs.OpenFile(filePath, os.O_WRONLY, os.FileMode(0600))
		require.NoError(t, err)

		count, err := file.Write([]byte{42})
		require.NoError(t, err)
		require.Equal(t, 1, count)

		// Try to read the metadata.
		_, err = dataColumnStorage.metadata(file)
		require.ErrorIs(t, err, errWrongVersion)

		err = file.Close()
		require.NoError(t, err)
	})
}

func TestNewStorageIndices(t *testing.T) {
	t.Run("wrong number of columns", func(t *testing.T) {
		_, err := newStorageIndices(nil)
		require.ErrorIs(t, err, errWrongNumberOfColumns)
	})

	t.Run("nominal", func(t *testing.T) {
		var indices [mandatoryNumberOfColumns]byte
		indices[0] = 1

		storageIndices, err := newStorageIndices(indices[:])
		require.NoError(t, err)
		require.Equal(t, indices, storageIndices.indices)
	})
}

func TestStorageIndicesGet(t *testing.T) {
	t.Run("index too large", func(t *testing.T) {
		var indices storageIndices
		_, _, err := indices.get(1_000_000)
		require.ErrorIs(t, errDataColumnIndexTooLarge, err)
	})

	t.Run("index not set", func(t *testing.T) {
		const expected = false
		var indices storageIndices
		actual, _, err := indices.get(0)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})

	t.Run("index set", func(t *testing.T) {
		const (
			expectedOk       = true
			expectedPosition = int64(3)
		)

		indices := storageIndices{indices: [mandatoryNumberOfColumns]byte{0, 131}}
		actualOk, actualPosition, err := indices.get(1)
		require.NoError(t, err)
		require.Equal(t, expectedOk, actualOk)
		require.Equal(t, expectedPosition, actualPosition)
	})
}

func TestStorageIndicesLen(t *testing.T) {
	const expected = int64(2)
	indices := storageIndices{count: 2}
	actual := indices.len()
	require.Equal(t, expected, actual)
}

func TestStorageIndicesAll(t *testing.T) {
	expectedIndices := []uint64{1, 3}
	indices := storageIndices{indices: [mandatoryNumberOfColumns]byte{0, 131, 0, 128}}
	actualIndices := indices.all()
	require.DeepEqual(t, expectedIndices, actualIndices)
}

func TestStorageIndicesSet(t *testing.T) {
	t.Run("data column index too large", func(t *testing.T) {
		var indices storageIndices
		err := indices.set(1_000_000, 0)
		require.ErrorIs(t, errDataColumnIndexTooLarge, err)
	})

	t.Run("position too large", func(t *testing.T) {
		var indices storageIndices
		err := indices.set(0, 255)
		require.ErrorIs(t, errDataColumnIndexTooLarge, err)
	})

	t.Run("nominal", func(t *testing.T) {
		expected := [mandatoryNumberOfColumns]byte{0, 0, 128, 0, 131}
		var storageIndices storageIndices
		require.Equal(t, int64(0), storageIndices.len())

		err := storageIndices.set(2, 1)
		require.NoError(t, err)
		require.Equal(t, int64(1), storageIndices.len())

		err = storageIndices.set(4, 3)
		require.NoError(t, err)
		require.Equal(t, int64(2), storageIndices.len())

		err = storageIndices.set(2, 0)
		require.NoError(t, err)
		require.Equal(t, int64(2), storageIndices.len())

		actual := storageIndices.indices
		require.Equal(t, expected, actual)
	})
}

func TestPrune(t *testing.T) {
	t.Run(("nothing to prune"), func(t *testing.T) {
		dir := t.TempDir()
		dataColumnStorage, err := NewDataColumnStorage(t.Context(), WithDataColumnBasePath(dir))
		require.NoError(t, err)

		dataColumnStorage.prune()
	})
	t.Run("nominal", func(t *testing.T) {
		var compareSlices = func(left, right []string) bool {
			if len(left) != len(right) {
				return false
			}

			leftMap := make(map[string]bool, len(left))
			for _, leftItem := range left {
				leftMap[leftItem] = true
			}

			for _, rightItem := range right {
				if _, ok := leftMap[rightItem]; !ok {
					return false
				}
			}

			return true
		}
		_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Slot: 33, Index: 2, Column: [][]byte{{1}, {2}, {3}}},      // Period 0 - Epoch 1
				{Slot: 33, Index: 4, Column: [][]byte{{2}, {3}, {4}}},      // Period 0 - Epoch 1
				{Slot: 128_002, Index: 2, Column: [][]byte{{1}, {2}, {3}}}, // Period 0 - Epoch 4000
				{Slot: 128_002, Index: 4, Column: [][]byte{{2}, {3}, {4}}}, // Period 0 - Epoch 4000
				{Slot: 128_003, Index: 1, Column: [][]byte{{1}, {2}, {3}}}, // Period 0 - Epoch 4000
				{Slot: 128_003, Index: 3, Column: [][]byte{{2}, {3}, {4}}}, // Period 0 - Epoch 4000
				{Slot: 131_138, Index: 2, Column: [][]byte{{1}, {2}, {3}}}, // Period 1 - Epoch 4098
				{Slot: 131_138, Index: 3, Column: [][]byte{{1}, {2}, {3}}}, // Period 1 - Epoch 4098
				{Slot: 131_169, Index: 2, Column: [][]byte{{1}, {2}, {3}}}, // Period 1 - Epoch 4099
				{Slot: 131_169, Index: 3, Column: [][]byte{{1}, {2}, {3}}}, // Period 1 - Epoch 4099
				{Slot: 262_144, Index: 2, Column: [][]byte{{1}, {2}, {3}}}, // Period 2 - Epoch 8192
				{Slot: 262_144, Index: 3, Column: [][]byte{{1}, {2}, {3}}}, // Period 2 - Epoch 8292
			},
		)

		dir := t.TempDir()
		dataColumnStorage, err := NewDataColumnStorage(t.Context(), WithDataColumnBasePath(dir), WithDataColumnRetentionEpochs(10_000))
		require.NoError(t, err)

		err = dataColumnStorage.Save(verifiedRoDataColumnSidecars)
		require.NoError(t, err)

		dirs, err := listDir(dataColumnStorage.fs, ".")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"0", "1", "2"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "0")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"1", "4000"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "1")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"4099", "4098"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "2")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"8192"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "0/1")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"0x775283f428813c949b7e8af07f01fef9790137f021b3597ad2d0d81e8be8f0f0.sszs"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "0/4000")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{
			"0x9977031132157ebb9c81bce952003ce07a4f54e921ca63b7693d1562483fdf9f.sszs",
			"0xb2b14d9d858fa99b70f0405e4e39f38e51e36dd9a70343c109e24eeb5f77e369.sszs",
		}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "1/4098")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"0x5106745cdd6b1aa3602ef4d000ef373af672019264c167fa4bd641a1094aa5c5.sszs"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "1/4099")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"0x4e5f2bd5bb84bf0422af8edd1cc5a52cc6cea85baf3d66d172fe41831ac1239c.sszs"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "2/8192")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"0xa8adba7446eb56a01a9dd6d55e9c3990b10c91d43afb77847b4a21ac4ee62527.sszs"}, dirs))

		_, verifiedRoDataColumnSidecars = util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Slot: 451_141, Index: 2, Column: [][]byte{{1}, {2}, {3}}}, // Period 3 - Epoch 14_098
			},
		)

		err = dataColumnStorage.Save(verifiedRoDataColumnSidecars)
		require.NoError(t, err)

		// dataColumnStorage.prune(14_098)
		dataColumnStorage.prune()

		dirs, err = listDir(dataColumnStorage.fs, ".")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"1", "2", "3"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "1")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"4099"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "2")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"8192"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "3")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"14098"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "1/4099")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"0x4e5f2bd5bb84bf0422af8edd1cc5a52cc6cea85baf3d66d172fe41831ac1239c.sszs"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "2/8192")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"0xa8adba7446eb56a01a9dd6d55e9c3990b10c91d43afb77847b4a21ac4ee62527.sszs"}, dirs))

		dirs, err = listDir(dataColumnStorage.fs, "3/14098")
		require.NoError(t, err)
		require.Equal(t, true, compareSlices([]string{"0x0de28a18cae63cbc6f0b20dc1afb0b1df38da40824a5f09f92d485ade04de97f.sszs"}, dirs))
	})
}

func TestExtractFileMetadata(t *testing.T) {
	t.Run("Unix", func(t *testing.T) {
		// Test with Unix-style path separators (/)
		path := "12/1234/0x8bb2f09de48c102635622dc27e6de03ae2b22639df7c33edbc8222b2ec423746.sszs"
		metadata, err := extractFileMetadata(path)
		if filepath.Separator == '/' {
			// On Unix systems, this should succeed
			require.NoError(t, err)
			require.Equal(t, uint64(12), metadata.period)
			require.Equal(t, primitives.Epoch(1234), metadata.epoch)
			return
		}

		// On Windows systems, this should fail because it uses the wrong separator
		require.NotNil(t, err)
	})

	t.Run("Windows", func(t *testing.T) {
		// Test with Windows-style path separators (\)
		path := "12\\1234\\0x8bb2f09de48c102635622dc27e6de03ae2b22639df7c33edbc8222b2ec423746.sszs"
		metadata, err := extractFileMetadata(path)
		if filepath.Separator == '\\' {
			// On Windows systems, this should succeed
			require.NoError(t, err)
			require.Equal(t, uint64(12), metadata.period)
			require.Equal(t, primitives.Epoch(1234), metadata.epoch)
			return
		}

		// On Unix systems, this should fail because it uses the wrong separator
		require.NotNil(t, err)
	})
}
