package helpers

import (
	"context"
	"time"
)

// Sleep sleeps for the given duration or until the context is done.
func Sleep(ctx context.Context, duration time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(duration):
	}
}
