package sync

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ToggleGroup represents a service that can be toggled.
type ToggleGroup uint32

const (
	NilService ToggleGroup = iota
	// ToggleGroupRangeSync represents the initial-sync service.
	ToggleGroupRangeSync
	// ToggleGroupBackfill represents the backfill service.
	ToggleGroupBackfill
)

// String returns a human-readable representation of the ToggleGroup.
func (t ToggleGroup) String() string {
	switch t {
	case NilService:
		return "none active"
	case ToggleGroupRangeSync:
		return "range sync"
	case ToggleGroupBackfill:
		return "backfill"
	default:
		return "unknown"
	}
}

type blockedToggleRoutine struct {
	id     ToggleGroup
	ch     chan struct{}
	queued time.Time
}

// ServiceToggler provides a mechanism similar to a WaitGroup, enabling multiple goroutines running
// in a pre-defined ToggleGroup to collectively acquire an exclusive "lock", which is released once
// all goroutines in that ToggleGroup have called Release.
// See documentation on the Acquire() and Release() methods for details.
type ServiceToggler struct {
	blocked []*blockedToggleRoutine
	current ToggleGroup
	mu      sync.Mutex
	active  int
}

// NewServiceToggler initialize a ServiceToggler.
func NewServiceToggler() *ServiceToggler {
	return &ServiceToggler{
		blocked: make([]*blockedToggleRoutine, 0),
		current: NilService,
		active:  0,
	}
}

// Acquire blocks until the calling service can have exclusive permission to run.
// If the calling service is already the current service and no service is waiting,
// Acquire will return immediately. If another service is waitig, it will not be allowed
// to acquire the "lock" until the other service has a chance to acquire and release outstanding
// lock requets.
// If the calling service is not the current service, it will block until all active
// threads of the current service have called Release.
func (t *ServiceToggler) Acquire(ctx context.Context, id ToggleGroup) error {
	t.mu.Lock()
	// This means we are initializing for the first time.
	// This is different from the normal toggle case because there won't be a call
	// to release to unblock the thread.
	if t.current == NilService {
		t.current = id
		t.active += 1
		t.mu.Unlock()
		return nil
	}
	// Fast path: if we are already the current service and there is no queue, just proceed.
	// We never want to starve the other service, so as soon as we have a queue, start toggling
	// between the different services.
	if t.current == id && len(t.blocked) == 0 {
		t.active += 1
		t.mu.Unlock()
		return nil
	}
	// ch will be closed when Release is called by the last active thread of the current service.
	// until then the call to Acquire will block at the <-ch line below.
	ch := make(chan struct{})
	t.blocked = append(t.blocked, &blockedToggleRoutine{
		id:     id,
		ch:     ch,
		queued: time.Now(),
	})
	log.WithField("service", id.String()).WithField("blocked", len(t.blocked)).Debug("Waiting on sync toggle")
	t.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release decrements the active thread counter and manages unblocking threads for the next service in line
// if there is a queue.
func (t *ServiceToggler) Release(id ToggleGroup) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current != id {
		// This is an impossible condition but ignoring it seems safer than breaking other invariants.
		return
	}
	t.active -= 1

	// If there are blocked threads, and this is the last active thread,
	// release the next blocked thread before completing Release.
	// We only want the last releaser to manage the blocked queue.
	if t.active > 0 || len(t.blocked) == 0 {
		return
	}

	next := t.blocked[0]
	t.current = next.id
	t.active += 1
	close(next.ch) // release the blocked thread
	for _, next := range t.blocked[1:] {
		if next.id != t.current {
			break
		}
		t.active += 1
		close(next.ch)
	}
	t.blocked = t.blocked[t.active:]
	log.WithFields(logrus.Fields{
		"releasedBy": id.String(),
		"service":    next.id.String(),
		"unblocked":  t.active,
		"waited":     time.Since(next.queued),
	}).Debug("Unblocked goroutines from sync toggle")
	// We hold the lock so t.active = number of elements removed from t.blocked
}
