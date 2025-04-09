package filesystem

import (
	"context"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

func (s *Store) SaveGraffitiOrderedIndex(_ context.Context, index uint64) error {
	// Get the configuration.
	configuration, err := s.configuration()
	if err != nil {
		return errors.Wrapf(err, "could not get configuration")
	}

	if configuration == nil {
		// Create an new configuration.
		configuration = &Configuration{
			Graffiti: &Graffiti{
				OrderedIndex: index,
			},
		}

		// Save the configuration.
		if err := s.saveConfiguration(configuration); err != nil {
			return errors.Wrapf(err, "could not save configuration")
		}

		return nil
	}

	if configuration.Graffiti == nil {
		// Create a new graffiti.
		configuration.Graffiti = &Graffiti{
			OrderedIndex: index,
		}

		// Save the configuration.
		if err := s.saveConfiguration(configuration); err != nil {
			return errors.Wrapf(err, "could not save configuration")
		}

		return nil
	}

	// Modify the value of ordered index.
	configuration.Graffiti.OrderedIndex = index

	// Save the configuration.
	if err := s.saveConfiguration(configuration); err != nil {
		return errors.Wrapf(err, "could not save configuration")
	}

	return nil
}

func (s *Store) GraffitiOrderedIndex(_ context.Context, fileHash [32]byte) (uint64, error) {
	// Encode the file hash to string.
	fileHashHex := hexutil.Encode(fileHash[:])

	// Get the configuration.
	configuration, err := s.configuration()
	if err != nil {
		return 0, errors.Wrapf(err, "could not get configuration")
	}

	if configuration == nil {
		// Create an new configuration.
		configuration = &Configuration{
			Graffiti: &Graffiti{
				OrderedIndex: 0,
				FileHash:     &fileHashHex,
			},
		}

		// Save the configuration.
		if err := s.saveConfiguration(configuration); err != nil {
			return 0, errors.Wrapf(err, "could not save configuration")
		}

		return 0, nil
	}

	if configuration.Graffiti == nil {
		// Create a new graffiti.
		configuration.Graffiti = &Graffiti{
			OrderedIndex: 0,
			FileHash:     &fileHashHex,
		}

		// Save the configuration.
		if err := s.saveConfiguration(configuration); err != nil {
			return 0, errors.Wrapf(err, "could not save configuration")
		}

		return 0, nil
	}

	// Check if file hash does not exist or is not equal to the file hash in configuration.
	if configuration.Graffiti.FileHash == nil || *configuration.Graffiti.FileHash != fileHashHex {
		// Modify the value of ordered index.
		configuration.Graffiti.OrderedIndex = 0

		// Modify the value of file hash.
		configuration.Graffiti.FileHash = &fileHashHex

		// Save the configuration.
		if err := s.saveConfiguration(configuration); err != nil {
			return 0, errors.Wrapf(err, "could not save configuration")
		}

		return 0, nil
	}

	return configuration.Graffiti.OrderedIndex, nil
}

func (s *Store) GraffitiFileHash() ([32]byte, bool, error) {
	// Get configuration.
	configuration, err := s.configuration()
	if err != nil {
		return [32]byte{}, false, errors.Wrapf(err, "could not get configuration")
	}

	// If configuration is nil or graffiti is nil or file hash is nil, set graffiti file hash as not existing.
	if configuration == nil || configuration.Graffiti == nil || configuration.Graffiti.FileHash == nil {
		return [32]byte{}, false, nil
	}

	// Convert the graffiti file hash to [32]byte.
	fileHashBytes, err := hexutil.Decode(*configuration.Graffiti.FileHash)
	if err != nil {
		return [32]byte{}, false, errors.Wrapf(err, "could not decode graffiti file hash")
	}

	if len(fileHashBytes) != 32 {
		return [32]byte{}, false, errors.Wrapf(err, "invalid graffiti file hash length")
	}

	var fileHash [32]byte
	copy(fileHash[:], fileHashBytes)

	// Return the graffiti file hash.
	return fileHash, true, nil
}
