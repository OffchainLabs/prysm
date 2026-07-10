package async_test

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/OffchainLabs/prysm/v7/async"
)

func TestEveryRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())

		i := int32(0)
		async.RunEvery(ctx, 100*time.Millisecond, func() {
			atomic.AddInt32(&i, 1)
		})

		// Advance the fake clock past a couple of ticks, then settle the ticker
		// goroutine and ensure the value has increased.
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()

		if atomic.LoadInt32(&i) == 0 {
			t.Error("Counter failed to increment with ticker")
		}

		cancel()

		// Settle the goroutine so it observes the cancellation and exits.
		synctest.Wait()

		last := atomic.LoadInt32(&i)

		// Advance the fake clock and ensure the value has not increased.
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()

		if atomic.LoadInt32(&i) != last {
			t.Error("Counter incremented after stop")
		}
	})
}
