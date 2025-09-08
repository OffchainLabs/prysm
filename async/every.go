// Package async includes helpers for scheduling runnable, periodic functions and contains useful helpers for converting multi-processor computation.
package async

import (
	"context"
	"reflect"
	"runtime"
	"time"

	log "github.com/sirupsen/logrus"
)

// RunEvery runs the provided command periodically.
// It runs in a goroutine, and can be cancelled by finishing the supplied context.
func RunEvery(ctx context.Context, period time.Duration, f func()) {
	funcName := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	ticker := time.NewTicker(period)
	go func() {
		for {
			select {
			case <-ticker.C:
				log.WithField("function", funcName).Trace("Running")
				f()
			case <-ctx.Done():
				log.WithField("function", funcName).Debug("Context is closed, exiting")
				ticker.Stop()
				return
			}
		}
	}()
}

// RunEveryDynamic runs the provided command periodically with a dynamic interval.
// The interval is determined by calling the intervalFunc before each execution.
// It runs in a goroutine, and can be cancelled by finishing the supplied context.
func RunEveryDynamic(ctx context.Context, intervalFunc func() time.Duration, f func()) {
	go func() {
		for {
			// Get the next interval duration
			interval := intervalFunc()
			timer := time.NewTimer(interval)

			select {
			case <-timer.C:
				f()
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}()
}
