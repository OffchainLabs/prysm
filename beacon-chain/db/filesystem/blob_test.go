package filesystem

import (
	"bytes"
	"math"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/spf13/afero"
)

func TestBlobStorage_SaveBlobData(t *testing.T) {
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	_, sidecars := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, params.BeaconConfig().MaxBlobsPerBlock(ds))
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
		expectedIdx := blobIndexMask{false, false, true, false, false, false}
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
	es := util.SlotAtEpoch(t, params.BeaconConfig().ElectraForkEpoch)
	fs := afero.NewMemMapFs()
	root := [32]byte{}

	okIdx := uint64(params.BeaconConfig().MaxBlobsPerBlock(es)) - 1
	writeFakeSSZ(t, fs, root, es, okIdx)
	bs := NewWarmedEphemeralBlobStorageUsingFs(t, fs, WithLayout(LayoutNameByEpoch))
	indices := bs.Summary(root).mask
	expected := make([]bool, params.BeaconConfig().MaxBlobsPerBlock(es))
	expected[okIdx] = true
	for i := range expected {
		require.Equal(t, expected[i], indices[i])
	}

	oobIdx := uint64(params.BeaconConfig().MaxBlobsPerBlock(es))
	writeFakeSSZ(t, fs, root, es, oobIdx)
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
	require.ErrorIs(t, err, errNoBlobBasePath)
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
