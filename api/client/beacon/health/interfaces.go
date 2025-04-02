package health

import "context"

type HealthTracker interface {
	HealthUpdates() <-chan bool
	IsHealthy(ctx context.Context) bool
	CheckHealth(ctx context.Context) bool
}

type HealthNode interface {
	IsHealthy(ctx context.Context) bool
}
