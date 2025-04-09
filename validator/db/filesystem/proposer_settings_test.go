package filesystem

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func getPubkeyFromString(t *testing.T, pubkeyString string) [fieldparams.BLSPubkeyLength]byte {
	var pubkey [fieldparams.BLSPubkeyLength]byte
	pubkeyBytes, err := hexutil.Decode(pubkeyString)
	require.NoError(t, err, "hexutil.Decode should not return an error")
	copy(pubkey[:], pubkeyBytes)
	return pubkey
}

func getFeeRecipientFromString(t *testing.T, feeRecipientString string) [fieldparams.FeeRecipientLength]byte {
	var feeRecipient [fieldparams.FeeRecipientLength]byte
	feeRecipientBytes, err := hexutil.Decode(feeRecipientString)
	require.NoError(t, err, "hexutil.Decode should not return an error")
	copy(feeRecipient[:], feeRecipientBytes)
	return feeRecipient
}

func TestStore_ProposerSettings(t *testing.T) {
	ctx := context.Background()

	pubkeyString := "0xb3533c600c6c22aa5177f295667deacffde243980d3c04da4057ab0941dcca1dff83ae8e6534bedd2d23d83446e604d6"
	customFeeRecipientString := "0xd4E96eF8eee8678dBFf4d535E033Ed1a4F7605b7"
	defaultFeeRecipientString := "0xC771172AE08B5FC37B3AC3D445225928DE883876"

	pubkey := getPubkeyFromString(t, pubkeyString)
	customFeeRecipient := getFeeRecipientFromString(t, customFeeRecipientString)
	defaultFeeRecipient := getFeeRecipientFromString(t, defaultFeeRecipientString)

	for _, tt := range []struct {
		name                     string
		configuration            *Configuration
		expectedProposerSettings *proposer.Settings
		expectedError            error
	}{
		{
			name:                     "configuration is nil",
			configuration:            nil,
			expectedProposerSettings: nil,
			expectedError:            ErrNoProposerSettingsFound,
		},
		{
			name:                     "configuration.ProposerSettings is nil",
			configuration:            &Configuration{ProposerSettings: nil},
			expectedProposerSettings: nil,
			expectedError:            ErrNoProposerSettingsFound,
		},
		{
			name: "configuration.ProposerSettings is something",
			configuration: &Configuration{
				ProposerSettings: &validatorpb.ProposerSettingsPayload{
					ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{
						pubkeyString: {
							FeeRecipient: customFeeRecipientString,
						},
					},
					DefaultConfig: &validatorpb.ProposerOptionPayload{
						FeeRecipient: defaultFeeRecipientString,
					},
				},
			},
			expectedProposerSettings: &proposer.Settings{
				ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
					pubkey: {
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: customFeeRecipient,
						},
					},
				},
				DefaultConfig: &proposer.Option{
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: defaultFeeRecipient,
					},
				},
			},
			expectedError: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new store.
			store, err := NewStore(t.TempDir(), nil)
			require.NoError(t, err, "NewStore should not return an error")

			// Save configuration.
			err = store.saveConfiguration(tt.configuration)
			require.NoError(t, err, "saveConfiguration should not return an error")

			// Get proposer settings.
			actualProposerSettings, err := store.ProposerSettings(ctx)
			if tt.expectedError != nil {
				require.ErrorIs(t, err, tt.expectedError, "ProposerSettings should return expected error")
			} else {
				require.NoError(t, err, "ProposerSettings should not return an error")
			}

			require.DeepEqual(t, tt.expectedProposerSettings, actualProposerSettings, "ProposerSettings should return expected")
		})
	}
}

func TestStore_ProposerSettingsExists(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name          string
		configuration *Configuration
		expectedExits bool
	}{
		{
			name:          "configuration is nil",
			configuration: nil,
			expectedExits: false,
		},
		{
			name:          "configuration.ProposerSettings is nil",
			configuration: &Configuration{ProposerSettings: nil},
			expectedExits: false,
		},
		{
			name:          "configuration.ProposerSettings is something",
			configuration: &Configuration{ProposerSettings: &validatorpb.ProposerSettingsPayload{}},
			expectedExits: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new store.
			store, err := NewStore(t.TempDir(), nil)
			require.NoError(t, err, "NewStore should not return an error")

			// Save configuration.
			err = store.saveConfiguration(tt.configuration)
			require.NoError(t, err, "saveConfiguration should not return an error")

			// Get proposer settings.
			actualExists, err := store.ProposerSettingsExists(ctx)
			require.NoError(t, err, "ProposerSettingsExists should not return an error")
			require.Equal(t, tt.expectedExits, actualExists, "ProposerSettingsExists should return expected")
		})
	}
}

func TestStore_SaveProposerSettings(t *testing.T) {
	ctx := context.Background()

	preExistingFeeRecipientString := "0xD871172AE08B5FC37B3AC3D445225928DE883876"
	incomingFeeRecipientString := "0xC771172AE08B5FC37B3AC3D445225928DE883876"

	incomingFeeRecipient := getFeeRecipientFromString(t, incomingFeeRecipientString)

	incomingProposerSettings := &proposer.Settings{
		DefaultConfig: &proposer.Option{
			FeeRecipientConfig: &proposer.FeeRecipientConfig{
				FeeRecipient: incomingFeeRecipient,
			},
		},
	}

	expectedConfiguration := &Configuration{
		ProposerSettings: &validatorpb.ProposerSettingsPayload{
			ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{},
			DefaultConfig: &validatorpb.ProposerOptionPayload{
				FeeRecipient: incomingFeeRecipientString,
			},
		},
	}

	for _, tt := range []struct {
		name                     string
		preExistingConfiguration *Configuration
		proposerSettings         *proposer.Settings
		expectedConfiguration    *Configuration
	}{
		{
			name:                     "proposerSettings is nil",
			preExistingConfiguration: nil,
			proposerSettings:         nil,
			expectedConfiguration:    nil,
		},
		{
			name:                     "configuration is nil",
			preExistingConfiguration: nil,
			proposerSettings:         incomingProposerSettings,
			expectedConfiguration:    expectedConfiguration,
		},
		{
			name: "configuration is something",
			preExistingConfiguration: &Configuration{
				ProposerSettings: &validatorpb.ProposerSettingsPayload{
					ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{},
					DefaultConfig: &validatorpb.ProposerOptionPayload{
						FeeRecipient: preExistingFeeRecipientString,
					},
				},
			},
			proposerSettings:      incomingProposerSettings,
			expectedConfiguration: expectedConfiguration,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new store.
			store, err := NewStore(t.TempDir(), nil)
			require.NoError(t, err, "NewStore should not return an error")

			// Save pre-existing configuration.
			err = store.saveConfiguration(tt.preExistingConfiguration)
			require.NoError(t, err, "saveConfiguration should not return an error")

			// Update proposer settings.
			err = store.SaveProposerSettings(ctx, tt.proposerSettings)
			require.NoError(t, err, "UpdateProposerSettingsDefault should not return an error")

			// Get configuration.
			actualConfiguration, err := store.configuration()
			require.NoError(t, err, "configuration should not return an error")
			require.DeepEqual(t, tt.expectedConfiguration, actualConfiguration, "configuration should return expected")
		})
	}
}
