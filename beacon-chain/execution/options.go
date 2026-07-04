package execution

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/network"
	"github.com/OffchainLabs/prysm/v7/network/authorization"
)

type Option func(s *Service) error

// WithHttpEndpoint parse http endpoint for the powchain service to use.
func WithHttpEndpoint(endpointString string) Option {
	return func(s *Service) error {
		s.cfg.currHttpEndpoint = network.HttpEndpoint(endpointString)
		return nil
	}
}

func WithPartialColumnsSupported() Option {
	return func(s *Service) error {
		s.partialColumnsSupported = true
		return nil
	}
}

// WithHttpEndpointAndJWTSecret for authenticating the execution node JSON-RPC endpoint.
func WithHttpEndpointAndJWTSecret(endpointString string, secret []byte) Option {
	return func(s *Service) error {
		if len(secret) == 0 {
			return nil
		}
		// Overwrite authorization type for all endpoints to be of a bearer type.
		hEndpoint := network.HttpEndpoint(endpointString)
		hEndpoint.Auth.Method = authorization.Bearer
		hEndpoint.Auth.Value = string(secret)

		s.cfg.currHttpEndpoint = hEndpoint
		return nil
	}
}

// WithHeaders adds headers to the execution node JSON-RPC requests.
func WithHeaders(headers []string) Option {
	return func(s *Service) error {
		s.cfg.headers = headers
		return nil
	}
}

// WithBeaconNodeStatsUpdater to set the beacon node stats updater.
func WithBeaconNodeStatsUpdater(updater BeaconNodeStatsUpdater) Option {
	return func(s *Service) error {
		s.cfg.beaconNodeStatsUpdater = updater
		return nil
	}
}

func WithJwtId(jwtId string) Option {
	return func(s *Service) error {
		s.cfg.jwtId = jwtId
		return nil
	}
}

// WithVerifierWaiter gives the sync package direct access to the verifier waiter.
func WithVerifierWaiter(v *verification.InitializerWaiter) Option {
	return func(s *Service) error {
		s.verifierWaiter = v
		return nil
	}
}

// WithGraffitiInfo sets the GraffitiInfo for client version tracking.
func WithGraffitiInfo(g *GraffitiInfo) Option {
	return func(s *Service) error {
		s.graffitiInfo = g
		return nil
	}
}
