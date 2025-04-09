package backfill

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestSortBatchDesc(t *testing.T) {
	orderIn := []primitives.Slot{100, 10000, 1}
	orderOut := []primitives.Slot{10000, 100, 1}
	batches := make([]batch, len(orderIn))
	for i := range orderIn {
		batches[i] = batch{end: orderIn[i]}
	}
	sortBatchDesc(batches)
	for i := range orderOut {
		require.Equal(t, orderOut[i], batches[i].end)
	}
}

func TestWaitUntilReady(t *testing.T) {
	b := batch{}.withState(batchErrRetryable)
	require.Equal(t, time.Time{}, b.retryAfter)
	var got time.Duration
	wur := batchBlockUntil
	var errDerp = errors.New("derp")
	batchBlockUntil = func(_ context.Context, ur time.Duration, _ batch) error {
		got = ur
		return errDerp
	}
	// retries counter and timestamp are set when we mark the batch for sequencing, if it is in the retry state
	b = b.withState(batchSequenced)
	require.ErrorIs(t, b.waitUntilReady(context.Background()), errDerp)
	require.Equal(t, true, retryDelay-time.Until(b.retryAfter) < time.Millisecond)
	require.Equal(t, true, got < retryDelay && got > retryDelay-time.Millisecond)
	require.Equal(t, 1, b.retries)
	batchBlockUntil = wur
}
