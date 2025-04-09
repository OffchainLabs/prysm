package filesystem

import (
	"context"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

func (s *Store) GenesisValidatorsRoot(_ context.Context) ([]byte, error) {
	// Get configuration.
	configuration, err := s.configuration()
	if err != nil {
		return nil, errors.Wrap(err, "could not get config")
	}

	// Return nil if config file does not exist.
	if configuration == nil {
		return nil, nil
	}

	// Return nil if genesis validators root is empty.
	if configuration.GenesisValidatorsRoot == nil {
		return nil, nil
	}

	// Convert genValRoot to bytes.
	genValRootBytes, err := hexutil.Decode(*configuration.GenesisValidatorsRoot)
	if err != nil {
		return nil, errors.Wrap(err, "could not decode genesis validators root")
	}

	return genValRootBytes, nil
}

// SaveGenesisValidatorsRoot saves the genesis validators root to db.
func (s *Store) SaveGenesisValidatorsRoot(_ context.Context, genValRoot []byte) error {
	// Return nil if genesis validators root is empty.
	if genValRoot == nil {
		return nil
	}

	// Convert genValRoot to hex.
	genValRootHex := hexutil.Encode(genValRoot)

	// Get configuration.
	configuration, err := s.configuration()
	if err != nil {
		return errors.Wrap(err, "could not get config")
	}

	if configuration == nil {
		// Create new config.
		configuration = &Configuration{
			GenesisValidatorsRoot: &genValRootHex,
		}

		// Save the config.
		if err := s.saveConfiguration(configuration); err != nil {
			return errors.Wrap(err, "could not save config")
		}

		return nil
	}

	// Modify the value of genesis validators root.
	configuration.GenesisValidatorsRoot = &genValRootHex

	// Save the config.
	if err := s.saveConfiguration(configuration); err != nil {
		return errors.Wrap(err, "could not save config")
	}

	return nil
}
