package backfill

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var errChainBroken = errors.New("batch is not the ancestor of a known finalized root")

// retryLogMod defines how often retryable errors are logged at debug level instead of trace.
const retryLogMod = 5

// retryDelay defines the delay between retry attempts for a batch.
const retryDelay = time.Second

type batchState int

func (s batchState) String() string {
	switch s {
	case batchNil:
		return "nil"
	case batchInit:
		return "init"
	case batchSequenced:
		return "sequenced"
	case batchSyncBlobs:
		return "sync_blobs"
	case batchSyncColumns:
		return "sync_columns"
	case batchImportable:
		return "importable"
	case batchImportComplete:
		return "import_complete"
	case batchEndSequence:
		return "end_sequence"
	case batchErrRetryable:
		return "error_retryable"
	case batchErrFatal:
		return "error_fatal"
	default:
		return "unknown"
	}
}

const (
	batchNil batchState = iota
	batchInit
	batchSequenced
	batchSyncBlobs
	batchSyncColumns
	batchImportable
	batchImportComplete
	batchErrRetryable
	batchErrFatal // if this is received in the main loop, the worker pool will be shut down.
	batchEndSequence
)

type batchId string

type batch struct {
	firstScheduled time.Time
	scheduled      time.Time
	seq            int // sequence identifier, ie how many times has the sequence() method served this batch
	retries        int
	retryAfter     time.Time
	begin          primitives.Slot
	end            primitives.Slot // half-open interval, [begin, end), ie >= begin, < end.
	blocks         verifiedROBlocks
	err            error
	state          batchState
	// `assignedPeer` is used by the worker pool to assign and unassign peer.IDs to serve requests for the current batch state.
	// Depending on the state it will be copied to blockPeer, columns.Peer, blobs.Peer.
	assignedPeer peer.ID
	blockPeer    peer.ID
	nextReqCols  []uint64
	blobs        *blobSync
	columns      *columnSync
}

func (b batch) logFields() logrus.Fields {
	f := map[string]any{
		"batchId":    b.id(),
		"state":      b.state.String(),
		"scheduled":  b.scheduled.String(),
		"seq":        b.seq,
		"retries":    b.retries,
		"retryAfter": b.retryAfter.String(),
		"begin":      b.begin,
		"end":        b.end,
		"busyPid":    b.assignedPeer,
		"blockPid":   b.blockPeer,
	}
	if b.blobs != nil {
		f["blobPid"] = b.blobs.peer
	}
	if b.columns != nil {
		f["colPid"] = b.columns.peer
	}
	if b.retries > 0 {
		f["retryAfter"] = b.retryAfter.String()
	}
	if b.state == batchSyncColumns {
		f["nextColumns"] = fmt.Sprintf("%v", b.nextReqCols)
	}
	if b.state == batchErrRetryable && b.blobs != nil {
		f["blobsMissing"] = b.blobs.needed()
	}
	return f
}

// replaces returns true if `r` is a version of `b` that has been updated by a worker,
// meaning it should replace `b` in the batch sequencing queue.
func (b batch) replaces(r batch) bool {
	if r.state == batchImportComplete {
		return false
	}
	if b.begin != r.begin {
		return false
	}
	if b.end != r.end {
		return false
	}
	return b.seq >= r.seq
}

func (b batch) id() batchId {
	return batchId(fmt.Sprintf("%d:%d", b.begin, b.end))
}

func (b batch) ensureParent(expected [32]byte) error {
	tail := b.blocks[len(b.blocks)-1]
	if tail.Root() != expected {
		return errors.Wrapf(errChainBroken, "last parent_root=%#x, tail root=%#x", expected, tail.Root())
	}
	return nil
}

func (b batch) blockRequest() *eth.BeaconBlocksByRangeRequest {
	return &eth.BeaconBlocksByRangeRequest{
		StartSlot: b.begin,
		Count:     uint64(b.end - b.begin),
		Step:      1,
	}
}

func (b batch) blobRequest() *eth.BlobSidecarsByRangeRequest {
	return &eth.BlobSidecarsByRangeRequest{
		StartSlot: b.begin,
		Count:     uint64(b.end - b.begin),
	}
}

func (b batch) transitionToNext() batch {
	if len(b.blocks) == 0 {
		return b.withState(batchSequenced)
	}
	if len(b.columns.columnsNeeded()) > 0 {
		return b.withState(batchSyncColumns)
	}
	if b.blobs != nil && b.blobs.needed() > 0 {
		return b.withState(batchSyncBlobs)
	}
	return b.withState(batchImportable)
}

func (b batch) withState(s batchState) batch {
	if s == batchSequenced {
		b.scheduled = time.Now()
		switch b.state {
		case batchInit, batchNil:
			b.firstScheduled = b.scheduled
		}
	}
	if s == batchImportComplete {
		backfillBatchTimeRoundtrip.Observe(float64(time.Since(b.firstScheduled).Milliseconds()))
	}
	b.state = s
	b.seq += 1
	return b
}

func (b batch) withRetryableError(err error) batch {
	b.err = err
	b.retries += 1
	b.retryAfter = time.Now().Add(retryDelay)

	msg := "Could not proceed with batch processing due to error"
	logBase := log.WithFields(b.logFields()).WithError(err)
	// Log at trace level to limit log noise,
	// but escalate to debug level every nth attempt for batches that have some peristent issue.
	if b.retries&retryLogMod != 0 {
		logBase.Trace(msg)
	} else {
		logBase.Debug(msg)
	}
	return b.withState(batchErrRetryable)
}

func (b batch) withFatalError(err error) batch {
	log.WithFields(b.logFields()).WithError(err).Error("Fatal batch processing error")
	b.err = err
	return b.withState(batchErrFatal)
}

func (b batch) withError(err error) batch {
	if isRetryable(err) {
		return b.withRetryableError(err)
	}
	return b.withFatalError(err)
}

func (b batch) validatingColumnRequest(cb *columnBisector) (*validatingColumnRequest, error) {
	req, err := b.columns.request(b.nextReqCols, columnRequestLimit)
	if err != nil {
		return nil, errors.Wrap(err, "columns request")
	}
	if req == nil {
		return nil, nil
	}
	return &validatingColumnRequest{
		req:        req,
		columnSync: b.columns,
		bisector:   cb,
	}, nil
}

// resetToRetryColumns is called after a partial batch failure. It adds column indices back
// to the toDownload structure for any blocks where those columns failed, and resets the bisector state.
// Note that this method will also prune any columns that have expired, meaning we no longer need them
// per spec and/or our backfill & retention settings.
func resetToRetryColumns(b batch, needs das.CurrentNeeds) batch {
	// return the given batch as-is if it isn't in a state that this func should handle.
	if b.columns == nil || b.columns.bisector == nil || len(b.columns.bisector.errs) == 0 {
		return b.transitionToNext()
	}
	pruned := make(map[[32]byte]struct{})
	b.columns.pruneExpired(needs, pruned)

	// clear out failed column state in the bisector and add back to
	bisector := b.columns.bisector
	roots := bisector.failingRoots()
	// Add all the failed columns back to the toDownload structure and reset the bisector state.
	for _, root := range roots {
		if _, rm := pruned[root]; rm {
			continue
		}
		bc := b.columns.toDownload[root]
		bc.remaining.Merge(bisector.failuresFor(root))
	}
	b.columns.bisector.reset()

	return b.transitionToNext()
}

var batchBlockUntil = func(ctx context.Context, untilRetry time.Duration, b batch) error {
	log.WithFields(b.logFields()).WithField("untilRetry", untilRetry.String()).
		Debug("Sleeping for retry backoff delay")
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(untilRetry):
		return nil
	}
}

func (b batch) waitUntilReady(ctx context.Context) error {
	// Wait to retry a failed batch to avoid hammering peers
	// if we've hit a state where batches will consistently fail.
	// Avoids spamming requests and logs.
	if b.retries > 0 {
		untilRetry := time.Until(b.retryAfter)
		if untilRetry > time.Millisecond {
			return batchBlockUntil(ctx, untilRetry, b)
		}
	}
	return nil
}

func (b batch) workComplete() bool {
	return b.state == batchImportable
}

func (b batch) expired(needs das.CurrentNeeds) bool {
	if !needs.Block.At(b.end - 1) {
		log.WithFields(b.logFields()).WithField("retentionStartSlot", needs.Block.Begin).Debug("Batch outside retention window")
		return true
	}
	return false
}

func (b batch) selectPeer(picker *sync.PeerPicker, busy map[peer.ID]bool) (peer.ID, []uint64, error) {
	if b.state == batchSyncColumns {
		return picker.ForColumns(b.columns.columnsNeeded(), busy)
	}
	peer, err := picker.ForBlocks(busy)
	return peer, nil, err
}

func sortBatchDesc(bb []batch) {
	sort.Slice(bb, func(i, j int) bool {
		return bb[i].end > bb[j].end
	})
}
