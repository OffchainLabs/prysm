package filesystem

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/spf13/afero"
)

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

type BlobMocker struct {
	fs afero.Fs
	bs *BlobStorage
}

// CreateFakeIndices creates empty blob sidecar files at the expected path for the given
// root and indices to influence the result of Indices().
func (bm *BlobMocker) CreateFakeIndices(root [32]byte, slot primitives.Slot, indices ...uint64) error {
	for i := range indices {
		if err := bm.bs.layout.notify(newBlobIdent(root, slots.ToEpoch(slot), indices[i])); err != nil {
			return err
		}
	}
	return nil
}

// NewEphemeralBlobStorageWithMocker returns a *BlobMocker value in addition to the BlobStorage value.
// BlockMocker encapsulates things blob path construction to avoid leaking implementation details.
func NewEphemeralBlobStorageWithMocker(t testing.TB) (*BlobMocker, *BlobStorage) {
	fs, bs := NewEphemeralBlobStorageAndFs(t)
	return &BlobMocker{fs: fs, bs: bs}, bs
}

func NewMockBlobStorageSummarizer(t *testing.T, set map[[32]byte][]int) BlobStorageSummarizer {
	c := newBlobStorageCache()
	for k, v := range set {
		for i := range v {
			if err := c.ensure(blobIdent{root: k, epoch: 0, index: uint64(v[i])}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return c
}
