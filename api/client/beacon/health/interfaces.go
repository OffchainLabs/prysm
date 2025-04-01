package health

import "context"

type Tracker interface {
	HealthUpdates() <-chan bool
	IsHealthy(ctx context.Context) bool
	CheckHealth(ctx context.Context) bool
}

type Node interface {
	IsHealthy(ctx context.Context) bool
}
