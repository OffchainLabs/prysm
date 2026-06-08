package async_test

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestDebounce_NoEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		eventsChan := make(chan any, 100)
		ctx, cancel := context.WithCancel(t.Context())
		interval := time.Second
		timesHandled := int32(0)
		go async.Debounce(ctx, interval, eventsChan, func(event any) {
			atomic.AddInt32(&timesHandled, 1)
		})
		// With no events, advance past the interval and cancel; the handler
		// must never run.
		time.Sleep(interval)
		cancel()
		synctest.Wait()
		assert.Equal(t, int32(0), atomic.LoadInt32(&timesHandled), "Wrong number of handled calls")
	})
}

func TestDebounce_CtxClosing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		eventsChan := make(chan any, 100)
		ctx, cancel := context.WithCancel(t.Context())
		interval := time.Second
		timesHandled := int32(0)
		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					eventsChan <- struct{}{}
				}
			}
		}()
		go async.Debounce(ctx, interval, eventsChan, func(event any) {
			atomic.AddInt32(&timesHandled, 1)
		})
		// Events arrive every 100ms, always resetting the debounce timer before
		// the interval elapses, so the handler must never run before cancel.
		time.Sleep(interval)
		cancel()
		synctest.Wait()
		assert.Equal(t, int32(0), atomic.LoadInt32(&timesHandled), "Wrong number of handled calls")
	})
}

func TestDebounce_SingleHandlerInvocation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		eventsChan := make(chan any, 100)
		ctx, cancel := context.WithCancel(t.Context())
		interval := time.Second
		timesHandled := int32(0)
		go async.Debounce(ctx, interval, eventsChan, func(event any) {
			atomic.AddInt32(&timesHandled, 1)
		})
		for range 100 {
			eventsChan <- struct{}{}
		}
		// We should expect 100 rapid fire changes to only have caused
		// 1 handler to trigger after the debouncing period.
		time.Sleep(interval * 2)
		synctest.Wait()
		assert.Equal(t, int32(1), atomic.LoadInt32(&timesHandled), "Wrong number of handled calls")
		cancel()
	})
}

func TestDebounce_MultipleHandlerInvocation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		eventsChan := make(chan any, 100)
		ctx, cancel := context.WithCancel(t.Context())
		interval := time.Second
		timesHandled := int32(0)
		go async.Debounce(ctx, interval, eventsChan, func(event any) {
			atomic.AddInt32(&timesHandled, 1)
		})
		for range 100 {
			eventsChan <- struct{}{}
		}
		// Settle the debounce goroutine: it has drained all events but the fake
		// clock has not advanced, so the handler must not have fired yet.
		synctest.Wait()
		require.Equal(t, int32(0), atomic.LoadInt32(&timesHandled), "Events must prevent from handler execution")

		// By this time the first event should be triggered.
		time.Sleep(2 * time.Second)
		synctest.Wait()
		assert.Equal(t, int32(1), atomic.LoadInt32(&timesHandled), "Wrong number of handled calls")

		// Second event.
		eventsChan <- struct{}{}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		assert.Equal(t, int32(2), atomic.LoadInt32(&timesHandled), "Wrong number of handled calls")

		cancel()
	})
}
