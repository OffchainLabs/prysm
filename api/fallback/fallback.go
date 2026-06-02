package fallback

import (
	"context"

	"github.com/sirupsen/logrus"
)

// HostProvider is the subset of connection-provider methods that EnsureReady
// needs. Both grpc.GrpcConnectionProvider and rest.RestConnectionProvider
// satisfy this interface.
type HostProvider interface {
	Hosts() []string
	CurrentHost() string
	SwitchHost(index int) error
}

// ReadyChecker can report whether the current endpoint is ready.
// iface.NodeClient satisfies this implicitly.
type ReadyChecker interface {
	IsReady(ctx context.Context) bool
}

// EnsureReady iterates through the configured hosts and returns true as soon as
// one responds as ready. It starts from the provider's current host and wraps
// around using modular arithmetic, performing failover when a host is not ready.
func EnsureReady(ctx context.Context, provider HostProvider, checker ReadyChecker) bool {
	hosts := provider.Hosts()
	numHosts := len(hosts)
	startingHost := provider.CurrentHost()
	var attemptedHosts []string

	// Find current index
	currentIdx := 0
	for i, h := range hosts {
		if h == startingHost {
			currentIdx = i
			break
		}
	}

	for i := range numHosts {
		if checker.IsReady(ctx) {
			if len(attemptedHosts) > 0 {
				log.WithFields(logrus.Fields{
					"previous": startingHost,
					"current":  provider.CurrentHost(),
					"tried":    attemptedHosts,
				}).Warn("Switched to responsive beacon node")
			}
			return true
		}
		attemptedHosts = append(attemptedHosts, provider.CurrentHost())

		// Try next host if not the last iteration
		if i < numHosts-1 {
			nextIdx := (currentIdx + i + 1) % numHosts
			if err := provider.SwitchHost(nextIdx); err != nil {
				log.WithError(err).Error("Failed to switch host")
			}
		}
	}

	log.WithField("tried", attemptedHosts).Warn("No responsive beacon node found")
	return false
}
