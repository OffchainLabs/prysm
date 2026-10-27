package fallback

import (
	"context"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/sirupsen/logrus"
)

// NodeStatus describes whether a beacon node can serve validator duties.
type NodeStatus int

const (
	// NodeReady means the beacon node and its execution client are synced.
	NodeReady NodeStatus = iota
	// NodeSyncing means the beacon node is still syncing the chain.
	NodeSyncing
	// NodeOptimistic means the beacon node is synced, but its execution client is not.
	NodeOptimistic
	// NodeUnavailable means the beacon node could not be reached, or gave an unexpected answer.
	NodeUnavailable
)

func (s NodeStatus) String() string {
	switch s {
	case NodeReady:
		return "ready"
	case NodeSyncing:
		return "syncing"
	case NodeOptimistic:
		return "optimistic"
	case NodeUnavailable:
		return "unavailable"
	default:
		return "unknown"
	}
}

// HostStatus is the status of one beacon node endpoint.
type HostStatus struct {
	Host   string
	Status NodeStatus
}

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
	ReadyStatus(ctx context.Context) NodeStatus
}

// EnsureReady iterates through the configured hosts and returns true as soon as
// one responds as ready. It starts from the provider's current host and wraps
// around using modular arithmetic, performing failover when a host is not ready.
func EnsureReady(ctx context.Context, provider HostProvider, checker ReadyChecker) bool {
	hosts := provider.Hosts()
	numHosts := len(hosts)
	startingHost := provider.CurrentHost()
	var attempted []HostStatus

	// Find current index
	currentIdx := 0
	for i, h := range hosts {
		if h == startingHost {
			currentIdx = i
			break
		}
	}

	for i := range numHosts {
		status := checker.ReadyStatus(ctx)
		if status == NodeReady {
			if len(attempted) > 0 {
				log.WithFields(logrus.Fields{
					"previous": api.RedactEndpoint(startingHost),
					"current":  api.RedactEndpoint(provider.CurrentHost()),
					"tried":    formatStatuses(attempted),
				}).Warn("Switched to responsive beacon node")
			}
			return true
		}
		attempted = append(attempted, HostStatus{Host: provider.CurrentHost(), Status: status})

		// Try next host if not the last iteration
		if i < numHosts-1 {
			nextIdx := (currentIdx + i + 1) % numHosts
			if err := provider.SwitchHost(nextIdx); err != nil {
				log.WithError(err).Error("Failed to switch host")
			}
		}
	}

	LogNotReady(attempted)
	return false
}

// LogNotReady explains why none of the given beacon nodes can serve validator duties.
// The message names the cause when every node shares it, so that operators know what to fix.
func LogNotReady(statuses []HostStatus) {
	if len(statuses) == 0 {
		return
	}

	msg := "No ready beacon node found"
	if common, ok := commonStatus(statuses); ok {
		switch common {
		case NodeUnavailable:
			msg = "No reachable beacon node found"
		case NodeSyncing:
			msg = "Beacon node is syncing, waiting for it to sync before performing duties"
		case NodeOptimistic:
			msg = "Beacon node is optimistic, waiting for its execution client to sync before performing duties"
		}
	}

	log.WithField("nodes", formatStatuses(statuses)).Warn(msg)
}

// commonStatus returns the status shared by every entry, and false if they differ.
func commonStatus(statuses []HostStatus) (NodeStatus, bool) {
	common := statuses[0].Status
	for _, s := range statuses[1:] {
		if s.Status != common {
			return 0, false
		}
	}

	return common, true
}

// formatStatuses renders statuses as "host1=syncing, host2=unavailable" with redacted hosts.
func formatStatuses(statuses []HostStatus) string {
	parts := make([]string, 0, len(statuses))
	for _, s := range statuses {
		parts = append(parts, api.RedactEndpointList(s.Host)+"="+s.Status.String())
	}

	return strings.Join(parts, ", ")
}
