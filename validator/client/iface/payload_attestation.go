package iface

import (
	"context"
	"time"
)

type payloadAttestationRetryKey struct{}

type payloadAttestationRetry struct {
	interval time.Duration
	onRetry  func()
}

// WithPayloadAttestationRetry shares the PTC retry cadence and notification.
// onRetry must be safe to call concurrently from multiple beacon-node reads.
func WithPayloadAttestationRetry(ctx context.Context, interval time.Duration, onRetry func()) context.Context {
	return context.WithValue(ctx, payloadAttestationRetryKey{}, payloadAttestationRetry{interval: interval, onRetry: onRetry})
}

// PayloadAttestationRetryFromContext returns the PTC caller's retry settings.
func PayloadAttestationRetryFromContext(ctx context.Context) (time.Duration, func(), bool) {
	retry, ok := ctx.Value(payloadAttestationRetryKey{}).(payloadAttestationRetry)
	return retry.interval, retry.onRetry, ok
}
