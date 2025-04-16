package health

import "context"

type Tracker interface {
	HealthUpdates() <-chan bool
	CheckHealth(ctx context.Context) bool
	Node
}

type Node interface {
	IsHealthy(ctx context.Context) bool
}
