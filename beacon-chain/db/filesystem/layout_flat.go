package filesystem

import (
	"encoding/binary"
	"io"
	"os"
	"path"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
)

type flatLayout struct {
	fs     afero.Fs
	cache  *blobStorageSummaryCache
	pruner *blobPruner
}

var _ fsLayout = &flatLayout{}

func newFlatLayout(fs afero.Fs, cache *blobStorageSummaryCache, pruner *blobPruner) fsLayout {
	l := &flatLayout{fs: fs, cache: cache, pruner: pruner}
	return l
}

func (l *flatLayout) iterateIdents(before primitives.Epoch) (*identIterator, error) {
	if _, err := l.fs.Stat("."); err != nil {
		if os.IsNotExist(err) {
			return &identIterator{eof: true}, nil // The directory is non-existent, which is fine; stop iteration.
		}
		return nil, errors.Wrapf(err, "error reading path %s", periodicEpochBaseDir)
	}
	entries, err := listDir(l.fs, ".")
	if err != nil {
		return nil, errors.Wrapf(err, "could not list root directory")
	}
	slotAndIndex := &flatSlotReader{fs: l.fs, cache: l.cache, before: before}
	return &identIterator{
		fs: l.fs,
		// Please see comments on the `layers` field in `identIterator`` if the role of the layers is unclear.
		layers: []layoutLayer{
			{populateIdent: populateRoot, filter: isFlatCachedAndBefore(l.cache, before)},
			{populateIdent: slotAndIndex.populateEpoch, filter: slotAndIndex.isSSZAndBefore}},
		entries: entries,
	}, nil
}

func (*flatLayout) name() string {
	return LayoutNameFlat
}

func (l *flatLayout) blockParentDirs(ident blobIdent) []string {
	return []string{}
}

func (*flatLayout) dir(n blobIdent) string {
	return rootToString(n.root)
}

func (l *flatLayout) sszPath(n blobIdent) string {
	return path.Join(l.dir(n), n.sszFname())
}

func (l *flatLayout) partPath(n blobIdent, entropy string) string {
	return path.Join(l.dir(n), n.partFname(entropy))
}

func (l *flatLayout) ident(root [32]byte, idx uint64) (blobIdent, error) {
	return l.cache.identForIdx(root, idx)
}

func (l *flatLayout) dirIdent(root [32]byte) (blobIdent, error) {
	return l.cache.identForRoot(root)
}

func (l *flatLayout) summary(root [32]byte) BlobStorageSummary {
	return l.cache.Summary(root)
}

func (l *flatLayout) remove(ident blobIdent) (int, error) {
	removed := l.cache.evict(ident.root)
	if err := l.fs.RemoveAll(l.dir(ident)); err != nil {
		return removed, err
	}
	return removed, nil
}

func (l *flatLayout) notify(ident blobIdent) error {
	if err := l.cache.ensure(ident); err != nil {
		return err
	}
	l.pruner.notify(ident.epoch, l)
	return nil
}

func (l *flatLayout) pruneBefore(before primitives.Epoch) (*pruneSummary, error) {
	sums, err := pruneBefore(before, l)
	if err != nil {
		return nil, err
	}

	// Roll up summaries and clean up per-epoch directories.
	rollup := &pruneSummary{}
	for _, sum := range sums {
		rollup.blobsPruned += sum.blobsPruned
		rollup.failedRemovals = append(rollup.failedRemovals, sum.failedRemovals...)
	}

	return rollup, nil
}

// Below this line are iteration support funcs and types that are specific to the flat layout.

// Read slot from marshaled BlobSidecar data in the given file. See slotFromBlob for details.
func slotFromFile(name string, fs afero.Fs) (primitives.Slot, error) {
	f, err := fs.Open(name)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.WithError(err).Errorf("Could not close blob file")
		}
	}()
	return slotFromBlob(f)
}

// slotFromBlob reads the ssz data of a file at the specified offset (8 + 131072 + 48 + 48 = 131176 bytes),
// which is calculated based on the size of the BlobSidecar struct and is based on the size of the fields
// preceding the slot information within SignedBeaconBlockHeader.
func slotFromBlob(at io.ReaderAt) (primitives.Slot, error) {
	b := make([]byte, 8)
	_, err := at.ReadAt(b, 131176)
	if err != nil {
		return 0, err
	}
	rawSlot := binary.LittleEndian.Uint64(b)
	return primitives.Slot(rawSlot), nil
}

type flatSlotReader struct {
	before primitives.Epoch
	fs     afero.Fs
	cache  *blobStorageSummaryCache
}

func (l *flatSlotReader) populateEpoch(ident blobIdent, fname string) (blobIdent, error) {
	ident, err := populateIndex(ident, fname)
	if err != nil {
		return ident, err
	}
	sum, ok := l.cache.get(ident.root)
	if ok {
		ident.epoch = sum.epoch
		// Return early if the index is already known to the cache.
		if sum.HasIndex(ident.index) {
			return ident, nil
		}
	} else {
		// If the root is not in the cache, we need to read the slot from the file.
		slot, err := slotFromFile(fname, l.fs)
		if err != nil {
			return ident, err
		}
		ident.epoch = slots.ToEpoch(slot)
	}
	return ident, l.cache.ensure(ident)
}

func (l *flatSlotReader) isSSZAndBefore(fname string) bool {
	if !isSszFile(fname) {
		return false
	}
	// If 'before' != 0, assuming isSSZAndBefore is used as a filter on the same layer with populateEpoch, this will typically
	// call populateEpoch before the iteration code calls it. So we can guarantee that the cache gets populated
	// in either case, because if it is filtered out here, we either have a malformed path (root can't be determined) in which case
	// the filter code won't call it anyway, or we have a valid path and the cache will be populated before the epoch can be compared.
	if l.before == 0 {
		return true
	}
	ident, err := populateRoot(blobIdent{}, path.Dir(fname))
	// Filter out the path if we can't determine its root.
	if err != nil {
		return false
	}
	ident, err = l.populateEpoch(ident, fname)
	// Filter out the path if we can't determine its epoch or properly cache it.
	if err != nil {
		return false
	}
	return ident.epoch < l.before
}

// isFlatCachedAndBefore returns a filter callback function to exclude roots that are known to be after the given epoch
// based on the cache. It's an opportunistic filter; if the cache is not populated, it will not attempt to populate it.
// isSSZAndBefore on the other hand, is a strict filter that will only return true if the file is an SSZ file and
// the epoch can be determined.
func isFlatCachedAndBefore(cache *blobStorageSummaryCache, before primitives.Epoch) func(string) bool {
	if before == 0 {
		return isRootDir
	}
	return func(p string) bool {
		if !isRootDir(p) {
			return false
		}
		root, err := rootFromPath(p)
		if err != nil {
			return false
		}
		sum, ok := cache.get(root)
		// If we don't know the epoch by looking at the root, don't try to filter it.
		if !ok {
			return true
		}
		return sum.epoch < before
	}
}
