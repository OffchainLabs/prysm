package filesystem

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/spf13/afero"
)

type periodicEpochLayout struct {
	fs     afero.Fs
	cache  *blobStorageCache
	pruner *blobPruner
}

var _ fsLayout = &periodicEpochLayout{}

func newPeriodicEpochLayout(fs afero.Fs, cache *blobStorageCache, pruner *blobPruner) fsLayout {
	l := &periodicEpochLayout{fs: fs, cache: cache, pruner: pruner}
	return l
}

func (l *periodicEpochLayout) notify(ident blobIdent) error {
	if err := l.cache.ensure(ident); err != nil {
		return err
	}
	l.pruner.notify(ident.epoch, l)
	return nil
}

// If before == 0, it won't be used as a filter and all idents will be returned.
func (l *periodicEpochLayout) iterateIdents(before primitives.Epoch) (*identIterator, error) {
	_, err := l.fs.Stat(periodicEpochBaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &identIterator{eof: true}, nil // The directory is non-existent, which is fine; stop iteration.
		}
		return nil, errors.Wrapf(err, "error reading path %s", periodicEpochBaseDir)
	}
	// iterate root, which should have directories named by "period"
	entries, err := listDir(l.fs, periodicEpochBaseDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list %s", periodicEpochBaseDir)
	}

	return &identIterator{
		fs:   l.fs,
		path: periodicEpochBaseDir,
		levels: []layoutLevel{
			{populateIdent: populateNoop, filter: isBeforePeriod(before)},
			{populateIdent: populateEpoch, filter: isBeforeEpoch(before)},
			{populateIdent: populateRoot, filter: isRootDir},  // extract root from path
			{populateIdent: populateIndex, filter: isSszFile}, // extract index from filename
		},
		entries: entries,
	}, nil
}

func (l *periodicEpochLayout) ident(root [32]byte, idx uint64) (blobIdent, error) {
	return l.cache.identForIdx(root, idx)
}

func (l *periodicEpochLayout) dirIdent(root [32]byte) (blobIdent, error) {
	return l.cache.identForRoot(root)
}

func (l *periodicEpochLayout) summary(root [32]byte) BlobStorageSummary {
	return l.cache.Summary(root)
}

func (l *periodicEpochLayout) dir(n blobIdent) string {
	return filepath.Join(l.epochDir(n.epoch), rootToString(n.root))
}

func (*periodicEpochLayout) epochDir(epoch primitives.Epoch) string {
	return filepath.Join(periodicEpochBaseDir, fmt.Sprintf("%d", periodForEpoch(epoch)), fmt.Sprintf("%d", epoch))
}

func periodForEpoch(epoch primitives.Epoch) primitives.Epoch {
	return epoch / params.BeaconConfig().MinEpochsForBlobsSidecarsRequest
}

func (l *periodicEpochLayout) sszPath(n blobIdent) string {
	return filepath.Join(l.dir(n), n.sszFname())
}

func (l *periodicEpochLayout) partPath(n blobIdent, entropy string) string {
	return path.Join(l.dir(n), n.partFname(entropy))
}

func (l *periodicEpochLayout) pruneBefore(before primitives.Epoch) (*pruneSummary, error) {
	sums, err := pruneBefore(before, l)
	if err != nil {
		return nil, err
	}
	// Roll up summaries and clean up per-epoch directories.
	rollup := &pruneSummary{}
	for epoch, sum := range sums {
		rollup.blobsPruned += sum.blobsPruned
		rollup.failedRemovals = append(rollup.failedRemovals, sum.failedRemovals...)
		rmdir := l.epochDir(epoch)
		if len(sum.failedRemovals) == 0 {
			if err := l.fs.Remove(rmdir); err != nil {
				log.WithField("dir", rmdir).WithError(err).Error("Failed to remove epoch directory while pruning")
			}
		} else {
			log.WithField("dir", rmdir).WithField("numFailed", len(sum.failedRemovals)).WithError(err).Error("Unable to remove epoch directory due to pruning failures")
		}
	}

	return rollup, nil
}

func (l *periodicEpochLayout) remove(ident blobIdent) (int, error) {
	removed := l.cache.evict(ident.root)
	// Skip the syscall if there are no blobs to remove.
	if removed == 0 {
		return 0, nil
	}
	if err := l.fs.RemoveAll(l.dir(ident)); err != nil {
		return removed, err
	}
	return removed, nil
}

// Funcs below this line are iteration support methods that are specific to the epoch layout.

func isBeforePeriod(before primitives.Epoch) func(string) bool {
	if before == 0 {
		return filterNoop
	}
	beforePeriod := periodForEpoch(before)
	if before%4096 != 0 {
		// Add one because we need to include the period the epoch is in, unless it is the first epoch in the period,
		// in which case we can just look at any previous period.
		beforePeriod += 1
	}
	return func(p string) bool {
		period, err := periodFromPath(p)
		if err != nil {
			return false
		}
		return primitives.Epoch(period) < beforePeriod
	}
}

func isBeforeEpoch(before primitives.Epoch) func(string) bool {
	if before == 0 {
		return filterNoop
	}
	return func(p string) bool {
		epoch, err := epochFromPath(p)
		if err != nil {
			return false
		}
		return epoch < before
	}
}

func epochFromPath(p string) (primitives.Epoch, error) {
	subdir := filepath.Base(p)
	epoch, err := strconv.ParseUint(subdir, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(errInvalidDirectoryLayout,
			"failed to decode epoch as uint, err=%s, dir=%s", err.Error(), p)
	}
	return primitives.Epoch(epoch), nil
}

func periodFromPath(p string) (uint64, error) {
	subdir := filepath.Base(p)
	period, err := strconv.ParseUint(subdir, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(errInvalidDirectoryLayout,
			"failed to decode period from path as uint, err=%s, dir=%s", err.Error(), p)
	}
	return period, nil
}

func populateEpoch(namer blobIdent, dir string) (blobIdent, error) {
	epoch, err := epochFromPath(dir)
	if err != nil {
		return namer, err
	}
	namer.epoch = epoch
	return namer, nil
}
