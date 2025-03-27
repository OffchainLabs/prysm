package epbs

import (
	"sync"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
)

type PayloadPendingQueue struct {
	queue map[[32]byte]*enginev1.SignedExecutionPayloadEnvelope
	sync.Mutex
}

// NewPayloadPendingQueue creates a new instance of the PayloadPendingQueue.
func NewPayloadPendingQueue() *PayloadPendingQueue {
	return &PayloadPendingQueue{
		queue: make(map[[32]byte]*enginev1.SignedExecutionPayloadEnvelope),
	}
}

// Add adds a new payload to the pending queue.
func (q *PayloadPendingQueue) Add(payload *enginev1.SignedExecutionPayloadEnvelope) {
	q.Lock()
	defer q.Unlock()
	q.queue[[32]byte(payload.Message.BeaconBlockRoot)] = payload
}

// Remove removes a payload from the pending queue.
func (q *PayloadPendingQueue) Remove(root [32]byte) {
	q.Lock()
	defer q.Unlock()
	delete(q.queue, root)
}

// Get returns a payload from the pending queue.
func (q *PayloadPendingQueue) Get(root [32]byte) *enginev1.SignedExecutionPayloadEnvelope {
	q.Lock()
	defer q.Unlock()
	return q.queue[root]
}

// Has returns true if the pending queue contains a payload with the given root.
func (q *PayloadPendingQueue) Has(root [32]byte) bool {
	q.Lock()
	defer q.Unlock()
	_, ok := q.queue[root]
	return ok
}

// Prune removes all payloads from the pending queue that are older than the given slot.
func (q *PayloadPendingQueue) Prune(slot primitives.Slot) {
	q.Lock()
	defer q.Unlock()
	for root, p := range q.queue {
		if p.Message.Slot < slot {
			delete(q.queue, root)
		}
	}
}
