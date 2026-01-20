package backfill

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
)

func TestBlockRequest(t *testing.T) {
	cases := []struct {
		name          string
		begin         primitives.Slot
		end           primitives.Slot
		expectedCount uint64
	}{
		{
			name:          "normal case",
			begin:         100,
			end:           200,
			expectedCount: 100,
		},
		{
			name:          "end equals begin",
			begin:         100,
			end:           100,
			expectedCount: 0,
		},
		{
			name:          "end less than begin (would underflow without check)",
			begin:         200,
			end:           100,
			expectedCount: 0,
		},
		{
			name:          "zero values",
			begin:         0,
			end:           0,
			expectedCount: 0,
		},
		{
			name:          "single slot",
			begin:         0,
			end:           1,
			expectedCount: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := batch{begin: tc.begin, end: tc.end}
			req := b.blockRequest()
			require.Equal(t, tc.expectedCount, req.Count)
			require.Equal(t, tc.begin, req.StartSlot)
			require.Equal(t, uint64(1), req.Step)
		})
	}
}

func TestBlobRequest(t *testing.T) {
	cases := []struct {
		name          string
		begin         primitives.Slot
		end           primitives.Slot
		expectedCount uint64
	}{
		{
			name:          "normal case",
			begin:         100,
			end:           200,
			expectedCount: 100,
		},
		{
			name:          "end equals begin",
			begin:         100,
			end:           100,
			expectedCount: 0,
		},
		{
			name:          "end less than begin (would underflow without check)",
			begin:         200,
			end:           100,
			expectedCount: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := batch{begin: tc.begin, end: tc.end}
			req := b.blobRequest()
			require.Equal(t, tc.expectedCount, req.Count)
			require.Equal(t, tc.begin, req.StartSlot)
		})
	}
}

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
	wur := batchBlockUntil

	var got time.Duration
	var errDerp = errors.New("derp")
	batchBlockUntil = func(_ context.Context, ur time.Duration, _ batch) error {
		got = ur
		return errDerp
	}

	b := batch{}.withRetryableError(errors.New("test error"))
	require.ErrorIs(t, b.waitUntilReady(t.Context()), errDerp)
	require.Equal(t, true, retryDelay-time.Until(b.retryAfter) < time.Millisecond)
	require.Equal(t, true, got < retryDelay && got > retryDelay-time.Millisecond)
	require.Equal(t, 1, b.retries)
	batchBlockUntil = wur
}
