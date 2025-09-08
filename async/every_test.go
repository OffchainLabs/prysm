package async_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async"
)

func TestEveryRuns(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	i := int32(0)
	async.RunEvery(ctx, 100*time.Millisecond, func() {
		atomic.AddInt32(&i, 1)
	})

	// Sleep for a bit and ensure the value has increased.
	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&i) == 0 {
		t.Error("Counter failed to increment with ticker")
	}

	cancel()

	// Sleep for a bit to let the cancel take place.
	time.Sleep(100 * time.Millisecond)

	last := atomic.LoadInt32(&i)

	// Sleep for a bit and ensure the value has not increased.
	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&i) != last {
		t.Error("Counter incremented after stop")
	}
}

func TestEveryDynamicRuns(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	i := int32(0)
	intervalCount := int32(0)

	// Start with 50ms intervals, then increase to 100ms after 2 calls
	async.RunEveryDynamic(ctx, func() time.Duration {
		count := atomic.LoadInt32(&intervalCount)
		atomic.AddInt32(&intervalCount, 1)
		if count < 2 {
			return 50 * time.Millisecond
		}
		return 100 * time.Millisecond
	}, func() {
		atomic.AddInt32(&i, 1)
	})

	// After 150ms, should have run at least 2 times (at 50ms and 100ms)
	time.Sleep(150 * time.Millisecond)
	count1 := atomic.LoadInt32(&i)
	if count1 < 2 {
		t.Errorf("Expected at least 2 runs after 150ms, got %d", count1)
	}

	// After another 150ms (total 300ms), should have run at least 3 times
	// (50ms, 100ms, 150ms, 250ms)
	time.Sleep(150 * time.Millisecond)
	count2 := atomic.LoadInt32(&i)
	if count2 < 3 {
		t.Errorf("Expected at least 3 runs after 300ms, got %d", count2)
	}

	cancel()

	// Sleep for a bit to let the cancel take place.
	time.Sleep(100 * time.Millisecond)

	last := atomic.LoadInt32(&i)

	// Sleep for a bit and ensure the value has not increased.
	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&i) != last {
		t.Error("Counter incremented after stop")
	}
}
