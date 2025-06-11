package health

import (
	"context"
	"sync"
)

type NodeHealthTracker struct {
	isHealthy   bool
	initialized bool
	healthChan  chan bool
	node        Node
	sync.RWMutex
}

func NewTracker(node Node) Tracker {
	return &NodeHealthTracker{
		node:        node,
		healthChan:  make(chan bool, 1),
		isHealthy:   false,
		initialized: false,
	}
}

// HealthUpdates provides a read-only channel for health updates.
func (n *NodeHealthTracker) HealthUpdates() <-chan bool {
	return n.healthChan
}

func (n *NodeHealthTracker) IsHealthy(_ context.Context) bool {
	n.RLock()
	defer n.RUnlock()
	return n.isHealthy
}

func (n *NodeHealthTracker) CheckHealth(ctx context.Context) bool {
	n.Lock()
	defer n.Unlock()

	newStatus := n.node.IsHealthy(ctx)

	// Send update if this is first check or status changed
	if !n.initialized || newStatus != n.isHealthy {
		n.isHealthy = newStatus
		n.initialized = true

		// Non-blocking send to channel
		select {
		case n.healthChan <- newStatus:
		default:
			// Channel full, drain and send
			<-n.healthChan
			n.healthChan <- newStatus
		}
	}

	return newStatus
}
