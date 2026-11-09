package p2p

import (
	"encoding/hex"
	"fmt"
	"slices"
	"sync"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/slice"
	"github.com/sirupsen/logrus"
)

// dataColumnBroadcastLogQuietPeriod is how long to wait, after the last broadcast for a given block root,
// before logging the aggregated broadcasts for this root.
const dataColumnBroadcastLogQuietPeriod = 100 * time.Millisecond

// dataColumnBroadcastLogInfo aggregates the broadcasts of data column sidecars for a single block root.
type dataColumnBroadcastLogInfo struct {
	slot              primitives.Slot
	minSinceStartTime time.Duration
	maxSinceStartTime time.Duration
	sumSinceStartTime time.Duration
	indices           []uint64
	timer             *time.Timer
}

// dataColumnBroadcastLogger aggregates data column sidecar broadcasts per block root,
// and emits a single debug log per root once no broadcast happened for this root during the quiet period.
type dataColumnBroadcastLogger struct {
	mu          sync.Mutex
	quietPeriod time.Duration
	pending     map[[fieldparams.RootLength]byte]*dataColumnBroadcastLogInfo
}

// record registers the broadcast of the data column sidecar with the given index for the given root,
// and (re)schedules the log for this root after the quiet period.
func (l *dataColumnBroadcastLogger) record(root [fieldparams.RootLength]byte, slot primitives.Slot, index uint64, sinceStartTime time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.pending == nil {
		l.pending = make(map[[fieldparams.RootLength]byte]*dataColumnBroadcastLogInfo)
	}

	quietPeriod := l.quietPeriod
	if quietPeriod == 0 {
		quietPeriod = dataColumnBroadcastLogQuietPeriod
	}

	info, ok := l.pending[root]
	if ok {
		info.minSinceStartTime = min(info.minSinceStartTime, sinceStartTime)
		info.maxSinceStartTime = max(info.maxSinceStartTime, sinceStartTime)
		info.sumSinceStartTime += sinceStartTime
		info.indices = append(info.indices, index)

		// If the timer already fired, its flush is waiting for the lock and will log this record too.
		// The flush scheduled by this reset is then a no-op.
		info.timer.Reset(quietPeriod)
		return
	}

	info = &dataColumnBroadcastLogInfo{
		slot:              slot,
		minSinceStartTime: sinceStartTime,
		maxSinceStartTime: sinceStartTime,
		sumSinceStartTime: sinceStartTime,
		indices:           []uint64{index},
	}

	info.timer = time.AfterFunc(quietPeriod, func() { l.flush(root, info) })
	l.pending[root] = info
}

// flush logs the aggregated broadcasts for the given root.
// It is a no-op if info is no longer the pending entry for this root (already flushed).
func (l *dataColumnBroadcastLogger) flush(root [fieldparams.RootLength]byte, info *dataColumnBroadcastLogInfo) {
	l.mu.Lock()
	if l.pending[root] != info {
		l.mu.Unlock()
		return
	}

	delete(l.pending, root)
	l.mu.Unlock()

	count := len(info.indices)
	slices.Sort(info.indices)
	avgSinceStartTime := info.sumSinceStartTime / time.Duration(count)

	log.WithFields(logrus.Fields{
		"slot":           info.slot,
		"root":           fmt.Sprintf("0x%s...", hex.EncodeToString(root[:])[:8]),
		"count":          count,
		"indices":        slice.PrettySlice(info.indices),
		"sinceStartTime": prettyMinMaxAverage(info.minSinceStartTime, info.maxSinceStartTime, avgSinceStartTime),
	}).Debug("Broadcasted data column sidecars summary")
}

func prettyMinMaxAverage(min, max, average time.Duration) string {
	return fmt.Sprintf("[min: %v, avg: %v, max: %v]", min, average, max)
}
