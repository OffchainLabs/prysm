package filesystem

import (
	"context"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
)

// Blobs
// -----

// NewEphemeralBlobStorage should only be used for tests.
// The instance of BlobStorage returned is backed by an in-memory virtual filesystem,
// improving test performance and simplifying cleanup.
func NewEphemeralBlobStorage(t testing.TB, opts ...BlobStorageOption) *BlobStorage {
	return NewWarmedEphemeralBlobStorageUsingFs(t, afero.NewMemMapFs(), opts...)
}

// NewEphemeralBlobStorageAndFs can be used by tests that want access to the virtual filesystem
// in order to interact with it outside the parameters of the BlobStorage api.
func NewEphemeralBlobStorageAndFs(t testing.TB, opts ...BlobStorageOption) (afero.Fs, *BlobStorage) {
	fs := afero.NewMemMapFs()
	bs := NewWarmedEphemeralBlobStorageUsingFs(t, fs, opts...)
	return fs, bs
}

func NewEphemeralBlobStorageUsingFs(t testing.TB, fs afero.Fs, opts ...BlobStorageOption) *BlobStorage {
	opts = append(opts,
		WithBlobRetentionEpochs(params.BeaconConfig().MinEpochsForBlobsSidecarsRequest),
		WithFs(fs))
	bs, err := NewBlobStorage(opts...)
	if err != nil {
		t.Fatalf("error initializing test BlobStorage, err=%s", err.Error())
	}
	return bs
}

func NewWarmedEphemeralBlobStorageUsingFs(t testing.TB, fs afero.Fs, opts ...BlobStorageOption) *BlobStorage {
	bs := NewEphemeralBlobStorageUsingFs(t, fs, opts...)
	bs.WarmCache()
	return bs
}

type (
	BlobMocker struct {
		fs afero.Fs
		bs *BlobStorage
	}

	DataColumnMocker struct {
		fs  afero.Fs
		dcs *DataColumnStorage
	}
)

// CreateFakeIndices creates empty blob sidecar files at the expected path for the given
// root and indices to influence the result of Indices().
func (bm *BlobMocker) CreateFakeIndices(root [fieldparams.RootLength]byte, slot primitives.Slot, indices ...uint64) error {
	for i := range indices {
		if err := bm.bs.layout.notify(newBlobIdent(root, slots.ToEpoch(slot), indices[i])); err != nil {
			return err
		}
	}
	return nil
}

// CreateFakeIndices creates empty blob sidecar files at the expected path for the given
// root and indices to influence the result of Indices().
func (bm *DataColumnMocker) CreateFakeIndices(root [fieldparams.RootLength]byte, slot primitives.Slot, indices ...uint64) error {
	err := bm.dcs.cache.set(DataColumnsIdent{Root: root, Epoch: slots.ToEpoch(slot), Indices: indices})
	if err != nil {
		return errors.Wrap(err, "cache set")
	}

	return nil
}

// NewEphemeralBlobStorageWithMocker returns a *BlobMocker value in addition to the BlobStorage value.
// BlockMocker encapsulates things blob path construction to avoid leaking implementation details.
func NewEphemeralBlobStorageWithMocker(t testing.TB) (*BlobMocker, *BlobStorage) {
	fs, bs := NewEphemeralBlobStorageAndFs(t)
	return &BlobMocker{fs: fs, bs: bs}, bs
}

func NewMockBlobStorageSummarizer(t *testing.T, epoch primitives.Epoch, set map[[32]byte][]int) BlobStorageSummarizer {
	c := newBlobStorageCache()
	for k, v := range set {
		for i := range v {
			if err := c.ensure(blobIdent{root: k, epoch: epoch, index: uint64(v[i])}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return c
}

// Data columns
// ------------

// NewEphemeralDataColumnStorage should only be used for tests.
// The instance of DataColumnStorage returned is backed by an in-memory virtual filesystem,
// improving test performance and simplifying cleanup.
func NewEphemeralDataColumnStorage(t testing.TB, opts ...DataColumnStorageOption) *DataColumnStorage {
	return NewWarmedEphemeralDataColumnStorageUsingFs(t, afero.NewMemMapFs(), opts...)
}

// NewEphemeralDataColumnStorageAndFs can be used by tests that want access to the virtual filesystem
// in order to interact with it outside the parameters of the DataColumnStorage API.
func NewEphemeralDataColumnStorageAndFs(t testing.TB, opts ...DataColumnStorageOption) (afero.Fs, *DataColumnStorage) {
	fs := afero.NewMemMapFs()
	dcs := NewWarmedEphemeralDataColumnStorageUsingFs(t, fs, opts...)
	return fs, dcs
}

func NewWarmedEphemeralDataColumnStorageUsingFs(t testing.TB, fs afero.Fs, opts ...DataColumnStorageOption) *DataColumnStorage {
	bs := NewEphemeralDataColumnStorageUsingFs(t, fs, opts...)
	bs.WarmCache()
	return bs
}

func NewEphemeralDataColumnStorageUsingFs(t testing.TB, fs afero.Fs, opts ...DataColumnStorageOption) *DataColumnStorage {
	opts = append(opts,
		WithDataColumnRetentionEpochs(params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest),
		WithDataColumnFs(fs),
	)

	bs, err := NewDataColumnStorage(context.Background(), opts...)
	if err != nil {
		t.Fatal(err)
	}

	return bs
}

// NewEphemeralDataColumnStorageWithMocker returns a *BlobMocker value in addition to the BlobStorage value.
// BlockMocker encapsulates things blob path construction to avoid leaking implementation details.
func NewEphemeralDataColumnStorageWithMocker(t testing.TB) (*DataColumnMocker, *DataColumnStorage) {
	fs, dcs := NewEphemeralDataColumnStorageAndFs(t)
	return &DataColumnMocker{fs: fs, dcs: dcs}, dcs
}
