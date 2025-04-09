package filesystem

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
)

// ErrNoProposerSettingsFound is an error thrown when no settings are found.
var ErrNoProposerSettingsFound = errors.New("no proposer settings found in bucket")

// ProposerSettings returns the proposer settings.
func (s *Store) ProposerSettings(_ context.Context) (*proposer.Settings, error) {
	// Get configuration
	configuration, err := s.configuration()
	if err != nil {
		return nil, errors.Wrap(err, "could not get configuration")
	}

	// Return on error if config file does not exist.
	if configuration == nil || configuration.ProposerSettings == nil {
		return nil, ErrNoProposerSettingsFound
	}

	// Convert proposer settings to validator service config.
	proposerSettings, err := proposer.SettingFromConsensus(configuration.ProposerSettings)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert proposer settings")
	}

	return proposerSettings, nil
}

// ProposerSettingsExists returns true if proposer settings exists, false otherwise.
func (s *Store) ProposerSettingsExists(_ context.Context) (bool, error) {
	// Get configuration.
	configuration, err := s.configuration()
	if err != nil {
		return false, errors.Wrap(err, "could not get configuration")
	}

	// If configuration is nil, return false.
	if configuration == nil {
		return false, nil
	}

	// Return true if proposer settings exists, false otherwise.
	exists := configuration.ProposerSettings != nil
	return exists, nil
}

// SaveProposerSettings saves the proposer settings.
func (s *Store) SaveProposerSettings(_ context.Context, proposerSettings *proposer.Settings) error {
	// Check if there is something to save.
	if !proposerSettings.ShouldBeSaved() {
		log.Warn("proposer settings are empty, nothing has been saved")
		return nil
	}

	// Convert proposer settings to payload.
	proposerSettingsPayload := proposerSettings.ToConsensus()

	// Get configuration.
	configuration, err := s.configuration()
	if err != nil {
		return errors.Wrap(err, "could not get configuration")
	}

	if configuration == nil {
		// If configuration is nil, create new config.
		configuration = &Configuration{
			ProposerSettings: proposerSettingsPayload,
		}

		// Save the configuration.
		if err := s.saveConfiguration(configuration); err != nil {
			return errors.Wrap(err, "could not save configuration")
		}

		return nil
	}

	// Modify the value of proposer settings.
	configuration.ProposerSettings = proposerSettingsPayload

	// Save the configuration.
	if err := s.saveConfiguration(configuration); err != nil {
		return errors.Wrap(err, "could not save configuration")
	}

	return nil
}
