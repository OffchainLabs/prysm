package coverage

import "sync/atomic"

// Notifier delivers coalescing wake-ups to the coverage runtime. It is
// constructed before the blockchain service so producers can hold it from
// their single construction point; it is never closed and Notify never
// blocks, so notifications arriving after the runtime stopped are harmless
// no-ops.
type Notifier struct {
	ch      chan struct{}
	headGen atomic.Uint64
}

// NewNotifier returns a ready-to-use coalescing notifier.
func NewNotifier() *Notifier {
	return &Notifier{ch: make(chan struct{}, 1)}
}

// Notify wakes the coordinator to reconcile durable state. Multiple calls
// coalesce into one pending wake-up.
func (n *Notifier) Notify() {
	if n == nil {
		return
	}
	select {
	case n.ch <- struct{}{}:
	default:
	}
}

// NotifyHead signals that the canonical head changed (new head or reorg). It
// additionally bumps the head generation so an in-progress scan discards its
// uncommitted page and restarts against the new canonical anchor.
func (n *Notifier) NotifyHead() {
	if n == nil {
		return
	}
	n.headGen.Add(1)
	n.Notify()
}

// headGeneration returns the current head generation counter.
func (n *Notifier) headGeneration() uint64 {
	return n.headGen.Load()
}

// wake exposes the coalescing wake-up channel to the coordinator loop.
func (n *Notifier) wake() <-chan struct{} {
	return n.ch
}
