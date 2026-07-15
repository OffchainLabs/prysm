package event

import (
	"context"
	"sync"
	"sync/atomic"
)

// StreamGuard ensures at most one event stream feeds a channel: starting a new
// run stops the previous one and waits for it to exit. The zero value is ready
// to use.
type StreamGuard struct {
	running atomic.Bool
	mu      sync.Mutex
	stop    context.CancelFunc
	done    chan struct{}
}

// Replace stops any previous run and waits for it to exit, then registers a
// new one. It returns the run's context and a finish function the caller must
// call (typically deferred) when the run exits.
func (g *StreamGuard) Replace(ctx context.Context) (context.Context, func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stop != nil {
		g.stop()
		<-g.done
	}
	subCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	g.stop, g.done = cancel, done
	return subCtx, func() {
		cancel()
		close(done)
	}
}

// MarkRunning records whether the stream is connected and pumping events.
func (g *StreamGuard) MarkRunning(running bool) {
	g.running.Store(running)
}

// IsRunning reports whether the stream is connected and pumping events.
func (g *StreamGuard) IsRunning() bool {
	return g.running.Load()
}
