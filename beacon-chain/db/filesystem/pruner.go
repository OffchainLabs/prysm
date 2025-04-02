package filesystem

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/sirupsen/logrus"
)

const retentionBuffer primitives.Epoch = 2

var errNotBlobSSZ = errors.New("not a blob ssz file")

// blobPruner keeps track of the tail end of the retention period, based only the blobs it has seen via the notify method.
// If the retention period advances in response to notify being called,
// the pruner will invoke the pruneBefore method of the given layout in a new goroutine.
// The details of pruning are left entirely to the layout, with the pruner's only responsibility being to
// schedule just one pruning operation at a time, for each forward movement of the minimum retention epoch.
type blobPruner struct {
	mu              sync.Mutex
	prunedBefore    atomic.Uint64
	retentionPeriod primitives.Epoch
}

func newBlobPruner(retain primitives.Epoch) *blobPruner {
	p := &blobPruner{retentionPeriod: retain + retentionBuffer}
	return p
}

// notify returns a channel that is closed when the pruning operation is complete.
// This is useful for tests, but at runtime fsLayouts or BlobStorage should not wait for completion.
func (p *blobPruner) notify(latest primitives.Epoch, layout fsLayout) chan struct{} {
	done := make(chan struct{})
	floor := periodFloor(latest, p.retentionPeriod)
	if primitives.Epoch(p.prunedBefore.Swap(uint64(floor))) >= floor {
		// Only trigger pruning if the atomic swap changed the previous value of prunedBefore.
		close(done)
		return done
	}
	go func() {
		p.mu.Lock()
		start := time.Now()
		defer p.mu.Unlock()
		sum, err := layout.pruneBefore(floor)
		if err != nil {
			log.WithError(err).WithFields(sum.LogFields()).Warn("Encountered errors during blob pruning.")
		}
		log.WithFields(logrus.Fields{
			"upToEpoch":    floor,
			"duration":     time.Since(start).String(),
			"filesRemoved": sum.blobsPruned,
		}).Debug("Pruned old blobs")
		blobsPrunedCounter.Add(float64(sum.blobsPruned))
		close(done)
	}()
	return done
}

func periodFloor(latest, period primitives.Epoch) primitives.Epoch {
	if latest < period {
		return 0
	}
	return latest - period
}
