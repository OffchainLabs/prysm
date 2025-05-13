package backfill

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ErrChainBroken indicates a backfill batch can't be imported to the db because it is not known to be the ancestor
// of the canonical chain.
var ErrChainBroken = errors.New("batch is not the ancestor of a known finalized root")

type batchState int

func (s batchState) String() string {
	switch s {
	case batchNil:
		return "nil"
	case batchInit:
		return "init"
	case batchSequenced:
		return "sequenced"
	case batchErrRetryable:
		return "error_retryable"
	case batchImportable:
		return "importable"
	case batchImportComplete:
		return "import_complete"
	case batchEndSequence:
		return "end_sequence"
	case batchSyncBlobs:
		return "sync_blobs"
	case batchSyncColumns:
		return "sync_columns"
	default:
		return "unknown"
	}
}

const (
	batchNil batchState = iota
	batchInit
	batchSequenced
	batchErrRetryable
	batchSyncBlobs
	batchSyncColumns
	batchImportable
	batchImportComplete
	batchEndSequence
)

var retryDelay = time.Second

type batchId string

type batch struct {
	firstScheduled time.Time
	scheduled      time.Time
	seq            int // sequence identifier, ie how many times has the sequence() method served this batch
	retries        int
	retryAfter     time.Time
	begin          primitives.Slot
	end            primitives.Slot // half-open interval, [begin, end), ie >= start, < end.
	blocks         verifiedROBlocks
	err            error
	state          batchState
	peer           peer.ID
	nextReqCols    []uint64
	blockPid       peer.ID
	blobs          *blobSync
	columns        *columnSync
}

func (b batch) logFields() logrus.Fields {
	f := map[string]interface{}{
		"batchId":   b.id(),
		"state":     b.state.String(),
		"scheduled": b.scheduled.String(),
		"seq":       b.seq,
		"retries":   b.retries,
		"begin":     b.begin,
		"end":       b.end,
		"busyPid":   b.peer,
		"blockPid":  b.blockPid,
	}
	if b.blobs != nil {
		f["blobPid"] = b.blobs.pid
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
		return errors.Wrapf(ErrChainBroken, "last parent_root=%#x, tail root=%#x", expected, tail.Root())
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
		case batchErrRetryable:
			b.retries += 1
			b.retryAfter = time.Now().Add(retryDelay)
			log.WithFields(b.logFields()).Info("Sequencing batch for retry after delay")
		case batchInit, batchNil:
			b.firstScheduled = b.scheduled
		}
	}
	if s == batchImportComplete {
		backfillBatchTimeRoundtrip.Observe(float64(time.Since(b.firstScheduled).Milliseconds()))
		log.WithFields(b.logFields()).Debug("Backfill batch imported")
	}
	b.state = s
	b.seq += 1
	return b
}

func (b batch) withRetryableError(err error) batch {
	log.WithFields(b.logFields()).WithError(err).Warn("Could not proceed with batch processing due to error")
	b.err = err
	return b.withState(batchErrRetryable)
}

func (b batch) validatingColumnRequest(cb *columnBisector) *validatingColumnRequest {
	req := b.columns.request(b.nextReqCols)
	if req == nil {
		return nil
	}
	return &validatingColumnRequest{
		req:        req,
		columns:    peerdas.NewColumnIndicesFromSlice(b.nextReqCols),
		columnSync: b.columns,
		bisector:   cb,
	}
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
