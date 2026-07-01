package builder

import (
	"github.com/OffchainLabs/prysm/v7/api/client/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/urfave/cli/v2"
)

type Option func(s *Service) error

// FlagOptions for builder service flag configurations.
func FlagOptions(c *cli.Context) ([]Option, error) {
	endpoint := c.String(flags.MevRelayEndpoint.Name)
	sszDisabled := c.Bool(flags.DisableBuilderSSZ.Name)
	var clientOpts []builder.ClientOpt
	if !sszDisabled {
		clientOpts = append(clientOpts, builder.WithSSZ())
	}
	var client *builder.Client
	if endpoint != "" {
		if !sszDisabled {
			log.Info("Using Builder APIs with SSZ enabled")
		}
		var err error
		client, err = builder.NewClient(endpoint, clientOpts...)
		if err != nil {
			return nil, err
		}
	}
	opts := []Option{
		WithBuilderClient(client),
		WithBuilderClientOpts(clientOpts...),
	}
	return opts, nil
}

// WithBuilderClient sets the builder client for the beacon chain builder service.
func WithBuilderClient(client builder.BuilderClient) Option {
	return func(s *Service) error {
		s.cfg.builderClient = client
		return nil
	}
}

// Recorded so lazily dialed per-URL Gloas builder clients match the flag client's configuration.
func WithBuilderClientOpts(opts ...builder.ClientOpt) Option {
	return func(s *Service) error {
		s.clientOpts = opts
		return nil
	}
}

// WithHeadFetcher gets the head info from chain service.
func WithHeadFetcher(svc blockchain.HeadFetcher) Option {
	return func(s *Service) error {
		s.cfg.headFetcher = svc
		return nil
	}
}

// WithDatabase for head access.
func WithDatabase(beaconDB db.HeadAccessDatabase) Option {
	return func(s *Service) error {
		s.cfg.beaconDB = beaconDB
		return nil
	}
}

// WithRegistrationCache uses a cache for the validator registrations instead of a persistent db.
func WithRegistrationCache() Option {
	return func(s *Service) error {
		s.registrationCache = cache.NewRegistrationCache()
		return nil
	}
}
