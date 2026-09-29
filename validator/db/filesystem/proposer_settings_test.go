package filesystem

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
	ctx := t.Context()

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
	ctx := t.Context()

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
	ctx := t.Context()

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

// v2 settings with a builders list must survive a filesystem round-trip: Version
// preserved and optional entry bytes normalized to nil (matching the bolt backend).
func TestStore_ProposerSettings_V2BuildersRoundTrip(t *testing.T) {
	ctx := t.Context()
	u64 := func(v uint64) *validator.Uint64 { u := validator.Uint64(v); return &u }
	key := getPubkeyFromString(t, "0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")

	in := &proposer.Settings{
		Version: proposer.SchemaV2,
		ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
			key: {
				FeeRecipientConfig: &proposer.FeeRecipientConfig{FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3")},
				GasLimit:           40000000,
				BuilderConfig: &proposer.BuilderConfig{
					Enabled: true,
					Builders: []*proposer.BuilderEntry{
						{URL: "https://builder-a.example", AuthData: []byte("secret"), MaxExecutionPayment: u64(1000000000)},
						{URL: "https://builder-b.example"},
					},
				},
			},
		},
	}

	store, err := NewStore(t.TempDir(), nil)
	require.NoError(t, err)
	require.NoError(t, store.SaveProposerSettings(ctx, in))

	got, err := store.ProposerSettings(ctx)
	require.NoError(t, err)
	require.DeepEqual(t, in, got)

	// The entry without auth_data must load back as nil, not empty-non-nil.
	require.IsNil(t, got.ProposeConfig[key].BuilderConfig.Builders[1].AuthData)

	// An explicit empty list (use no builders) must survive the round-trip
	// rather than reloading as nil (inherit).
	in.ProposeConfig[key].BuilderConfig.Builders = []*proposer.BuilderEntry{}
	require.NoError(t, store.SaveProposerSettings(ctx, in))
	got, err = store.ProposerSettings(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.ProposeConfig[key].BuilderConfig.Builders)
	require.Equal(t, 0, len(got.ProposeConfig[key].BuilderConfig.Builders))
}
