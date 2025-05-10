package builder

import (
	"github.com/OffchainLabs/prysm/v6/api/client/builder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/urfave/cli/v2"
)

type Option func(s *Service) error

// FlagOptions for builder service flag configurations.
func FlagOptions(c *cli.Context) ([]Option, error) {
	endpoint := c.String(flags.MevRelayEndpoint.Name)
	sszEnabled := c.Bool(flags.EnableBuilderSSZ.Name)
	var client *builder.Client
	if endpoint != "" {
		var opts []builder.ClientOpt
		if sszEnabled {
			log.Info("Using APIs with SSZ enabled")
			opts = append(opts, builder.WithSSZ())
		}
		var err error
		client, err = builder.NewClient(endpoint, opts...)
		if err != nil {
			return nil, err
		}
	}
	opts := []Option{
		WithBuilderClient(client),
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
