package filesystem

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path"
	"sync"
	"testing"

	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/spf13/afero"
)

func TestBlobStorage_SaveBlobData(t *testing.T) {
	_, sidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 1, params.BeaconConfig().MaxBlobsPerBlock(1))
	testSidecars := verification.FakeVerifySliceForTest(t, sidecars)

	t.Run("no error for duplicate", func(t *testing.T) {
		fs, bs := NewEphemeralBlobStorageAndFs(t)
		existingSidecar := testSidecars[0]

		blobPath := bs.layout.sszPath(identForSidecar(existingSidecar))
		// Serialize the existing BlobSidecar to binary data.
		existingSidecarData, err := ssz.MarshalSSZ(existingSidecar)
		require.NoError(t, err)

		require.NoError(t, bs.Save(existingSidecar))
		// No error when attempting to write twice.
		require.NoError(t, bs.Save(existingSidecar))

		content, err := afero.ReadFile(fs, blobPath)
		require.NoError(t, err)
		require.Equal(t, true, bytes.Equal(existingSidecarData, content))

		// Deserialize the BlobSidecar from the saved file data.
		savedSidecar := &ethpb.BlobSidecar{}
		err = savedSidecar.UnmarshalSSZ(content)
		require.NoError(t, err)

		// Compare the original Sidecar and the saved Sidecar.
		require.DeepSSZEqual(t, existingSidecar.BlobSidecar, savedSidecar)

	})
	t.Run("indices", func(t *testing.T) {
		bs := NewEphemeralBlobStorage(t)
		sc := testSidecars[2]
		require.NoError(t, bs.Save(sc))
		actualSc, err := bs.Get(sc.BlockRoot(), sc.Index)
		require.NoError(t, err)
		expectedIdx := dataIndexMask{false, false, true, false, false, false}
		actualIdx := bs.Summary(actualSc.BlockRoot()).mask
		require.NoError(t, err)
		require.DeepEqual(t, expectedIdx, actualIdx)
	})

	t.Run("round trip write then read", func(t *testing.T) {
		bs := NewEphemeralBlobStorage(t)
		err := bs.Save(testSidecars[0])
		require.NoError(t, err)

		expected := testSidecars[0]
		actual, err := bs.Get(expected.BlockRoot(), expected.Index)
		require.NoError(t, err)
		require.DeepSSZEqual(t, expected, actual)
	})

	t.Run("round trip write, read and delete", func(t *testing.T) {
		bs := NewEphemeralBlobStorage(t)
		err := bs.Save(testSidecars[0])
		require.NoError(t, err)

		expected := testSidecars[0]
		actual, err := bs.Get(expected.BlockRoot(), expected.Index)
		require.NoError(t, err)
		require.DeepSSZEqual(t, expected, actual)

		require.NoError(t, bs.Remove(expected.BlockRoot()))
		_, err = bs.Get(expected.BlockRoot(), expected.Index)
		require.Equal(t, true, db.IsNotFound(err))
	})

	t.Run("clear", func(t *testing.T) {
		blob := testSidecars[0]
		b := NewEphemeralBlobStorage(t)
		require.NoError(t, b.Save(blob))
		res, err := b.Get(blob.BlockRoot(), blob.Index)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NoError(t, b.Clear())
		// After clearing, the blob should not exist in the db.
		_, err = b.Get(blob.BlockRoot(), blob.Index)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("race conditions", func(t *testing.T) {
		// There was a bug where saving the same blob in multiple go routines would cause a partial blob
		// to be empty. This test ensures that several routines can safely save the same blob at the
		// same time. This isn't ideal behavior from the caller, but should be handled safely anyway.
		// See https://github.com/prysmaticlabs/prysm/pull/13648
		b, err := NewBlobStorage(WithBasePath(t.TempDir()))
		require.NoError(t, err)
		blob := testSidecars[0]

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				require.NoError(t, b.Save(blob))
			}()
		}

		wg.Wait()
		res, err := b.Get(blob.BlockRoot(), blob.Index)
		require.NoError(t, err)
		require.DeepSSZEqual(t, blob, res)
	})
}

func TestBlobIndicesBounds(t *testing.T) {
	fs := afero.NewMemMapFs()
	root := [32]byte{}

	okIdx := uint64(params.BeaconConfig().MaxBlobsPerBlock(0)) - 1
	writeFakeSSZ(t, fs, root, 0, okIdx)
	bs := NewWarmedEphemeralBlobStorageUsingFs(t, fs, WithLayout(LayoutNameByEpoch))
	indices := bs.Summary(root).mask
	expected := make([]bool, params.BeaconConfig().MaxBlobsPerBlock(0))
	expected[okIdx] = true
	for i := range expected {
		require.Equal(t, expected[i], indices[i])
	}

	oobIdx := uint64(params.BeaconConfig().MaxBlobsPerBlock(0))
	writeFakeSSZ(t, fs, root, 0, oobIdx)
	// This now fails at cache warmup time.
	require.ErrorIs(t, warmCache(bs.layout, bs.cache), errIndexOutOfBounds)
}

func writeFakeSSZ(t *testing.T, fs afero.Fs, root [32]byte, slot primitives.Slot, idx uint64) {
	epoch := slots.ToEpoch(slot)
	namer := newBlobIdent(root, epoch, idx)
	layout := periodicEpochLayout{}
	require.NoError(t, fs.MkdirAll(layout.dir(namer), 0700))
	fh, err := fs.Create(layout.sszPath(namer))
	require.NoError(t, err)
	_, err = fh.Write([]byte("derp"))
	require.NoError(t, err)
	require.NoError(t, fh.Close())
}

func TestNewBlobStorage(t *testing.T) {
	_, err := NewBlobStorage()
	require.ErrorIs(t, err, errNoBasePath)
	_, err = NewBlobStorage(WithBasePath(path.Join(t.TempDir(), "good")))
	require.NoError(t, err)
}

func TestConfig_WithinRetentionPeriod(t *testing.T) {
	retention := primitives.Epoch(16)
	storage := &BlobStorage{retentionEpochs: retention}

	cases := []struct {
		name      string
		requested primitives.Epoch
		current   primitives.Epoch
		within    bool
	}{
		{
			name:      "before",
			requested: 0,
			current:   retention + 1,
			within:    false,
		},
		{
			name:      "same",
			requested: 0,
			current:   0,
			within:    true,
		},
		{
			name:      "boundary",
			requested: 0,
			current:   retention,
			within:    true,
		},
		{
			name:      "one less",
			requested: retention - 1,
			current:   retention,
			within:    true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.within, storage.WithinRetentionPeriod(c.requested, c.current))
		})
	}

	t.Run("overflow", func(t *testing.T) {
		storage := &BlobStorage{retentionEpochs: math.MaxUint64}
		require.Equal(t, true, storage.WithinRetentionPeriod(1, 1))
	})
}

func TestLayoutNames(t *testing.T) {
	badLayoutName := "bad"
	for _, name := range LayoutNames {
		_, err := newLayout(name, nil, nil, nil)
		require.NoError(t, err)
	}
	_, err := newLayout(badLayoutName, nil, nil, nil)
	require.ErrorIs(t, err, errInvalidLayoutName)
}

func TestBlobStorage_DataColumn_WithAllLayouts(t *testing.T) {
	for _, layout := range LayoutNames {
		t.Run(fmt.Sprintf("layout=%s", layout), func(t *testing.T) {
			sidecars := setupDataColumnTest(t)

			t.Run("no error for duplicate", func(t *testing.T) {
				fs, bs := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))
				sidecar := sidecars[0]

				columnPath := bs.layout.sszPath(identForDataColumnSidecar(sidecar))
				data, err := ssz.MarshalSSZ(sidecar)
				require.NoError(t, err)

				require.NoError(t, bs.SaveDataColumn(sidecar))
				// No error when attempting to write twice.
				require.NoError(t, bs.SaveDataColumn(sidecar))

				content, err := afero.ReadFile(fs, columnPath)
				require.NoError(t, err)
				require.Equal(t, true, bytes.Equal(data, content))

				// Deserialize the DataColumnSidecar from the saved file data.
				saved := &ethpb.DataColumnSidecar{}
				err = saved.UnmarshalSSZ(content)
				require.NoError(t, err)

				// Compare the original Sidecar and the saved Sidecar.
				require.DeepSSZEqual(t, sidecar.DataColumnSidecar, saved)
			})

			t.Run("indices", func(t *testing.T) {
				bs := NewEphemeralBlobStorage(t, WithLayout(layout))
				sidecar := sidecars[2]
				require.NoError(t, bs.SaveDataColumn(sidecar))
				actual, err := bs.GetColumn(sidecar.BlockRoot(), sidecar.ColumnIndex)
				require.NoError(t, err)
				require.DeepEqual(t, sidecar, actual)

				expectedIdx := make(dataIndexMask, params.BeaconConfig().NumberOfColumns)
				expectedIdx[2] = true
				actualIdx := bs.Summary(actual.BlockRoot()).mask
				require.NoError(t, err)
				require.DeepEqual(t, expectedIdx, actualIdx)

				sidecar = sidecars[10]
				expectedIdx[10] = true
				require.NoError(t, bs.SaveDataColumn(sidecar))
				actual, err = bs.GetColumn(sidecar.BlockRoot(), sidecar.ColumnIndex)
				require.NoError(t, err)
				require.DeepEqual(t, sidecar, actual)
				actualIdx = bs.Summary(actual.BlockRoot()).mask
				require.NoError(t, err)
				require.DeepEqual(t, expectedIdx, actualIdx)
			})

			t.Run("write -> read -> delete", func(t *testing.T) {
				bs := NewEphemeralBlobStorage(t, WithLayout(layout))
				err := bs.SaveDataColumn(sidecars[0])
				require.NoError(t, err)

				expected := sidecars[0]
				actual, err := bs.GetColumn(expected.BlockRoot(), expected.ColumnIndex)
				require.NoError(t, err)
				require.DeepEqual(t, expected, actual)

				require.NoError(t, bs.Remove(expected.BlockRoot()))
				for i := range params.BeaconConfig().NumberOfColumns {
					_, err = bs.GetColumn(expected.BlockRoot(), uint64(i))
					require.Equal(t, true, db.IsNotFound(err))
				}
			})

			t.Run("clear", func(t *testing.T) {
				bs := NewEphemeralBlobStorage(t, WithLayout(layout))
				err := bs.SaveDataColumn(sidecars[0])
				require.NoError(t, err)
				res, err := bs.GetColumn(sidecars[0].BlockRoot(), sidecars[0].ColumnIndex)
				require.NoError(t, err)
				require.NotNil(t, res)
				require.NoError(t, bs.Clear())
				// After clearing, the blob should not exist in the db.
				_, err = bs.GetColumn(sidecars[0].BlockRoot(), sidecars[0].ColumnIndex)
				require.ErrorIs(t, err, os.ErrNotExist)
			})
		})
	}
}

func TestBlobStorage_DataColumn_WithMigrationFromFlatToByEpoch(t *testing.T) {
	sidecars := setupDataColumnTest(t)

	// Setup flat layout
	fs, bs := NewEphemeralBlobStorageAndFs(t, WithLayout(LayoutNameFlat))
	sidecar := sidecars[0]
	columnPath := bs.layout.sszPath(identForDataColumnSidecar(sidecar))
	data, err := ssz.MarshalSSZ(sidecar)
	require.NoError(t, err)
	require.NoError(t, bs.SaveDataColumn(sidecar))
	content, err := afero.ReadFile(fs, columnPath)
	require.NoError(t, err)
	require.Equal(t, true, bytes.Equal(data, content))

	// Setup by-epoch layout
	bs = NewWarmedEphemeralBlobStorageUsingFs(t, fs, WithLayout(LayoutNameByEpoch))

	// Verify data is the same
	columnPath = bs.layout.sszPath(identForDataColumnSidecar(sidecar))
	content, err = afero.ReadFile(fs, columnPath)
	require.NoError(t, err)
	require.Equal(t, true, bytes.Equal(data, content))
}

func TestBlobStorage_DataColumn_WithMigrationFromByEpochToFlat(t *testing.T) {
	sidecars := setupDataColumnTest(t)

	// Setup by-epoch layout
	fs, bs := NewEphemeralBlobStorageAndFs(t, WithLayout(LayoutNameFlat))
	for _, sidecar := range sidecars {
		require.NoError(t, bs.SaveDataColumn(sidecar))
	}
	columnPath := bs.layout.sszPath(identForDataColumnSidecar(sidecars[0]))
	content, err := afero.ReadFile(fs, columnPath)
	require.NoError(t, err)
	data, err := ssz.MarshalSSZ(sidecars[0])
	require.NoError(t, err)
	require.Equal(t, true, bytes.Equal(data, content))

	// Setup flat layout
	bs = NewWarmedEphemeralBlobStorageUsingFs(t, fs, WithLayout(LayoutNameByEpoch))

	// Verify data is the same
	columnPath = bs.layout.sszPath(identForDataColumnSidecar(sidecars[0]))
	content, err = afero.ReadFile(fs, columnPath)
	require.NoError(t, err)
	require.Equal(t, true, bytes.Equal(data, content))
}

func setupDataColumnTest(t *testing.T) []blocks.VerifiedRODataColumn {
	// load trusted setup
	err := kzg.Start()
	require.NoError(t, err)

	// Setup right fork epoch
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	_, scs := util.GenerateTestFuluBlockWithSidecar(t, [32]byte{}, 0, 1)
	return verification.FakeVerifyDataColumnSliceForTest(t, scs)
}
