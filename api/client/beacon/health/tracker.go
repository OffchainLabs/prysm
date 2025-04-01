package health

import (
	"context"
	"sync"
)

type Tracker struct {
	isHealthy  *bool
	healthChan chan bool
	node       HealthNode
	sync.RWMutex
}

func NewTracker(node HealthNode) *Tracker {
	return &Tracker{
		node:       node,
		healthChan: make(chan bool, 1),
	}
}

// HealthUpdates provides a read-only channel for health updates.
func (n *Tracker) HealthUpdates() <-chan bool {
	return n.healthChan
}

func (n *Tracker) IsHealthy() bool {
	n.RLock()
	defer n.RUnlock()
	if n.isHealthy == nil {
		return false
	}
	return *n.isHealthy
}

func (n *Tracker) CheckHealth(ctx context.Context) bool {
	n.Lock()
	defer n.Unlock()

	newStatus := n.node.IsHealthy(ctx)
	if n.isHealthy == nil {
		n.isHealthy = &newStatus
	}

	isStatusChanged := newStatus != *n.isHealthy
	if isStatusChanged {
		// Update the health status
		n.isHealthy = &newStatus
		// Send the new status to the health channel, potentially overwriting the existing value
		select {
		case <-n.healthChan:
			n.healthChan <- newStatus
		default:
			n.healthChan <- newStatus
		}
	}
	return newStatus
}
