package filesystem

import (
	"context"
	"fmt"
	"math"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/logging"
	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
)

var (
	errIndexOutOfBounds    = errors.New("blob index in file name >= DeprecatedMaxBlobsPerBlock")
	errEmptyBlobWritten    = errors.New("zero bytes written to disk when saving blob sidecar")
	errSidecarEmptySSZData = errors.New("sidecar marshalled to an empty ssz byte slice")
	errNoBasePath          = errors.New("BlobStorage base path not specified in init")
	errInvalidRootString   = errors.New("Could not parse hex string as a [32]byte")
)

const (
	sszExt  = "ssz"
	partExt = "part"

	directoryPermissions = 0700
)

// BlobStorageOption is a functional option for configuring a BlobStorage.
type BlobStorageOption func(*BlobStorage) error

// WithBasePath is a required option that sets the base path of blob storage.
func WithBasePath(base string) BlobStorageOption {
	return func(b *BlobStorage) error {
		b.base = base
		return nil
	}
}

// WithBlobRetentionEpochs is an option that changes the number of epochs blobs will be persisted.
func WithBlobRetentionEpochs(e primitives.Epoch) BlobStorageOption {
	return func(b *BlobStorage) error {
		b.retentionEpochs = e
		return nil
	}
}

// WithSaveFsync is an option that causes Save to call fsync before renaming part files for improved durability.
func WithSaveFsync(fsync bool) BlobStorageOption {
	return func(b *BlobStorage) error {
		b.fsync = fsync
		return nil
	}
}

// NewBlobStorage creates a new instance of the BlobStorage object. Note that the implementation of BlobStorage may
// attempt to hold a file lock to guarantee exclusive control of the blob storage directory, so this should only be
// initialized once per beacon node.
func NewBlobStorage(opts ...BlobStorageOption) (*BlobStorage, error) {
	b := &BlobStorage{}
	for _, o := range opts {
		if err := o(b); err != nil {
			return nil, errors.Wrap(err, "failed to create blob storage")
		}
	}
	if b.base == "" {
		return nil, errNoBasePath
	}
	b.base = path.Clean(b.base)
	if err := file.MkdirAll(b.base); err != nil {
		return nil, errors.Wrapf(err, "failed to create blob storage at %s", b.base)
	}
	b.fs = afero.NewBasePathFs(afero.NewOsFs(), b.base)
	pruner, err := newBlobPruner(b.fs, b.retentionEpochs)
	if err != nil {
		return nil, err
	}
	b.pruner = pruner
	return b, nil
}

// BlobStorage is the concrete implementation of the filesystem backend for saving and retrieving BlobSidecars.
type BlobStorage struct {
	base            string
	retentionEpochs primitives.Epoch
	fsync           bool
	fs              afero.Fs
	pruner          *blobPruner
}

// WarmCache runs the prune routine with an expiration of slot of 0, so nothing will be pruned, but the pruner's cache
// will be populated at node startup, avoiding a costly cold prune (~4s in syscalls) during syncing.
func (bs *BlobStorage) WarmCache() {
	if bs.pruner == nil {
		return
	}
	go func() {
		start := time.Now()
		if err := bs.pruner.warmCache(); err != nil {
			log.WithError(err).Error("Error encountered while warming up blob pruner cache")
		}
		log.WithField("elapsed", time.Since(start)).Info("Blob filesystem cache warm-up complete.")
	}()
}

// ErrBlobStorageSummarizerUnavailable is a sentinel error returned when there is no pruner/cache available.
// This should be used by code that optionally uses the summarizer to optimize rpc requests. Being able to
// fallback when there is no summarizer allows client code to avoid test complexity where the summarizer doesn't matter.
var ErrBlobStorageSummarizerUnavailable = errors.New("BlobStorage not initialized with a pruner or cache")

// WaitForSummarizer blocks until the BlobStorageSummarizer is ready to use.
// BlobStorageSummarizer is not ready immediately on node startup because it needs to sample the blob filesystem to
// determine which blobs are available.
func (bs *BlobStorage) WaitForSummarizer(ctx context.Context) (BlobStorageSummarizer, error) {
	if bs == nil || bs.pruner == nil {
		return nil, ErrBlobStorageSummarizerUnavailable
	}
	return bs.pruner.waitForCache(ctx)
}

// Save saves blobs given a list of sidecars.
func (bs *BlobStorage) Save(sidecar blocks.VerifiedROBlob) error {
	startTime := time.Now()
	fname := namerForSidecar(sidecar)
	sszPath := fname.path()
	exists, err := afero.Exists(bs.fs, sszPath)
	if err != nil {
		return err
	}
	if exists {
		log.WithFields(logging.BlobFields(sidecar.ROBlob)).Debug("Ignoring a duplicate blob sidecar save attempt")
		return nil
	}
	if bs.pruner != nil {
		if err := bs.pruner.notify(sidecar.BlockRoot(), sidecar.Slot(), sidecar.Index); err != nil {
			return errors.Wrapf(err, "problem maintaining pruning cache/metrics for sidecar with root=%#x", sidecar.BlockRoot())
		}
	}

	// Serialize the ethpb.BlobSidecar to binary data using SSZ.
	sidecarData, err := sidecar.MarshalSSZ()
	if err != nil {
		return errors.Wrap(err, "failed to serialize sidecar data")
	} else if len(sidecarData) == 0 {
		return errSidecarEmptySSZData
	}

	if err := bs.fs.MkdirAll(fname.dir(), directoryPermissions); err != nil {
		return err
	}
	partPath := fname.partPath(fmt.Sprintf("%p", sidecarData))

	partialMoved := false
	// Ensure the partial file is deleted.
	defer func() {
		if partialMoved {
			return
		}
		// It's expected to error if the save is successful.
		err = bs.fs.Remove(partPath)
		if err == nil {
			log.WithFields(logrus.Fields{
				"partPath": partPath,
			}).Debugf("Removed partial file")
		}
	}()

	// Create a partial file and write the serialized data to it.
	partialFile, err := bs.fs.Create(partPath)
	if err != nil {
		return errors.Wrap(err, "failed to create partial file")
	}

	n, err := partialFile.Write(sidecarData)
	if err != nil {
		closeErr := partialFile.Close()
		if closeErr != nil {
			return closeErr
		}
		return errors.Wrap(err, "failed to write to partial file")
	}
	if bs.fsync {
		if err := partialFile.Sync(); err != nil {
			return err
		}
	}

	if err := partialFile.Close(); err != nil {
		return err
	}

	if n != len(sidecarData) {
		return fmt.Errorf("failed to write the full bytes of sidecarData, wrote only %d of %d bytes", n, len(sidecarData))
	}

	if n == 0 {
		return errEmptyBlobWritten
	}

	// Atomically rename the partial file to its final name.
	err = bs.fs.Rename(partPath, sszPath)
	if err != nil {
		return errors.Wrap(err, "failed to rename partial file to final name")
	}
	partialMoved = true
	blobsWrittenCounter.Inc()
	blobSaveLatency.Observe(float64(time.Since(startTime).Milliseconds()))

	return nil
}

// Get retrieves a single BlobSidecar by its root and index.
// Since BlobStorage only writes blobs that have undergone full verification, the return
// value is always a VerifiedROBlob.
func (bs *BlobStorage) Get(root [32]byte, idx uint64) (blocks.VerifiedROBlob, error) {
	startTime := time.Now()
	expected := blobNamer{root: root, index: idx}
	encoded, err := afero.ReadFile(bs.fs, expected.path())
	var v blocks.VerifiedROBlob
	if err != nil {
		return v, err
	}
	s := &ethpb.BlobSidecar{}
	if err := s.UnmarshalSSZ(encoded); err != nil {
		return v, err
	}
	ro, err := blocks.NewROBlobWithRoot(s, root)
	if err != nil {
		return blocks.VerifiedROBlob{}, err
	}
	defer func() {
		blobFetchLatency.Observe(float64(time.Since(startTime).Milliseconds()))
	}()
	return verification.BlobSidecarNoop(ro)
}

// Remove removes all blobs for a given root.
func (bs *BlobStorage) Remove(root [32]byte) error {
	rootDir := blobNamer{root: root}.dir()
	return bs.fs.RemoveAll(rootDir)
}

// Indices generates a bitmap representing which BlobSidecar.Index values are present on disk for a given root.
// This value can be compared to the commitments observed in a block to determine which indices need to be found
// on the network to confirm data availability.
func (bs *BlobStorage) Indices(root [32]byte, s primitives.Slot) ([]bool, error) {
	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(s)
	mask := make([]bool, maxBlobsPerBlock)

	rootDir := blobNamer{root: root}.dir()
	entries, err := afero.ReadDir(bs.fs, rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return mask, nil
		}
		return mask, err
	}

	for i := range entries {
		if entries[i].IsDir() {
			continue
		}
		name := entries[i].Name()
		if !strings.HasSuffix(name, sszExt) {
			continue
		}
		parts := strings.Split(name, ".")
		if len(parts) != 2 {
			continue
		}
		u, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return mask, errors.Wrapf(err, "unexpected directory entry breaks listing, %s", parts[0])
		}
		if u >= uint64(maxBlobsPerBlock) {
			return mask, errIndexOutOfBounds
		}
		mask[u] = true
	}
	return mask, nil
}

// Clear deletes all files on the filesystem.
func (bs *BlobStorage) Clear() error {
	dirs, err := listDir(bs.fs, ".")
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := bs.fs.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}

// WithinRetentionPeriod checks if the requested epoch is within the blob retention period.
func (bs *BlobStorage) WithinRetentionPeriod(requested, current primitives.Epoch) bool {
	if requested > math.MaxUint64-bs.retentionEpochs {
		// If there is an overflow, then the retention period was set to an extremely large number.
		return true
	}
	return requested+bs.retentionEpochs >= current
}

type blobNamer struct {
	root  [32]byte
	index uint64
}

func namerForSidecar(sc blocks.VerifiedROBlob) blobNamer {
	return blobNamer{root: sc.BlockRoot(), index: sc.Index}
}

func (p blobNamer) dir() string {
	return rootString(p.root)
}

func (p blobNamer) partPath(entropy string) string {
	return path.Join(p.dir(), fmt.Sprintf("%s-%d.%s", entropy, p.index, partExt))
}

func (p blobNamer) path() string {
	return path.Join(p.dir(), fmt.Sprintf("%d.%s", p.index, sszExt))
}

func rootString(root [32]byte) string {
	return fmt.Sprintf("%#x", root)
}

func stringToRoot(str string) ([32]byte, error) {
	slice, err := hexutil.Decode(str)
	if err != nil {
		return [32]byte{}, errors.Wrapf(errInvalidRootString, "input=%s", str)
	}
	return bytesutil.ToBytes32(slice), nil
}
