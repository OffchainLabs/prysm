package proposer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBuilderConfig_AuthDataFromFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(file, []byte(`{
		"default_config": {
			"fee_recipient": "0x8943545177806ED17B9F23F0a21ee5948eCaa776",
			"builder": {
				"enabled": true,
				"builders": [
					{"url": "http://builder-a:8080"},
					{"url": "http://sidecar:9000", "pubkey": "0xb0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7", "auth_data": "0x1234abcd", "max_execution_payment": "77", "min_bid": "5", "builder_boost_factor": "150"},
					{"url": "http://sidecar:9000", "auth_data": "0xdeadbeef"}
				],
				"max_execution_payment": "1000"
			}
		}
	}`), 0o600))

	var payload *validatorpb.ProposerSettingsPayload
	require.NoError(t, config.UnmarshalFromFile(file, &payload))
	settings, err := proposer.SettingFromConsensus(payload)
	require.NoError(t, err)

	bc := settings.DefaultConfig.BuilderConfig
	require.NotNil(t, bc)
	require.Equal(t, 3, len(bc.Builders))
	require.Equal(t, uint64(1000), uint64(bc.MaxExecutionPayment))
	require.Equal(t, "http://builder-a:8080", bc.Builders[0].URL)
	require.Equal(t, "", bc.Builders[0].Pubkey)
	data, err := bc.Builders[0].AuthDataBytes()
	require.NoError(t, err)
	require.DeepEqual(t, []byte("http://builder-a:8080"), data)
	require.Equal(t, "0x1234abcd", bc.Builders[1].AuthData)
	data, err = bc.Builders[1].AuthDataBytes()
	require.NoError(t, err)
	require.DeepEqual(t, []byte{0x12, 0x34, 0xab, 0xcd}, data)
	require.Equal(t, uint64(77), uint64(bc.Builders[1].MaxExecutionPayment))
	require.Equal(t, uint64(5), uint64(bc.Builders[1].MinBid))
	require.Equal(t, uint64(150), uint64(bc.Builders[1].BuilderBoostFactor))

	// Everything survives the clone and the consensus round-trip used by the DB.
	require.Equal(t, "0x1234abcd", bc.Clone().Builders[1].AuthData)
	roundTrip, err := proposer.SettingFromConsensus(settings.ToConsensus())
	require.NoError(t, err)
	require.DeepEqual(t, bc.Builders, roundTrip.DefaultConfig.BuilderConfig.Builders)
}

func TestBuilderConfig_InvalidAuthData(t *testing.T) {
	payload := &validatorpb.ProposerSettingsPayload{
		DefaultConfig: &validatorpb.ProposerOptionPayload{
			FeeRecipient: "0x8943545177806ED17B9F23F0a21ee5948eCaa776",
			Builder: &validatorpb.BuilderConfig{
				Enabled:  true,
				Builders: []*validatorpb.BuilderEntry{{Url: "http://builder-a:8080", AuthData: "not-hex"}},
			},
		},
	}
	_, err := proposer.SettingFromConsensus(payload)
	require.ErrorContains(t, "not valid 0x-hex", err)
}

func TestBuilderConfig_DuplicateEntries(t *testing.T) {
	payload := &validatorpb.ProposerSettingsPayload{
		DefaultConfig: &validatorpb.ProposerOptionPayload{
			FeeRecipient: "0x8943545177806ED17B9F23F0a21ee5948eCaa776",
			Builder: &validatorpb.BuilderConfig{
				Enabled: true,
				Builders: []*validatorpb.BuilderEntry{
					{Url: "http://builder-a:8080"},
					// Explicit auth_data spelling the default URL bytes is the same (url, data) pair.
					{Url: "http://builder-a:8080", AuthData: "0x687474703a2f2f6275696c6465722d613a38303830"},
				},
			},
		},
	}
	_, err := proposer.SettingFromConsensus(payload)
	require.ErrorContains(t, "duplicate builder entry", err)

	payload.DefaultConfig.Builder.Builders[1].AuthData = "0xdeadbeef"
	_, err = proposer.SettingFromConsensus(payload)
	require.NoError(t, err)
}

func TestBuilderConfig_RelaysDeprecatedAlias(t *testing.T) {
	file := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(file, []byte(`{
		"default_config": {
			"fee_recipient": "0x8943545177806ED17B9F23F0a21ee5948eCaa776",
			"builder": {
				"enabled": true,
				"relays": ["http://builder-a:8080"]
			}
		}
	}`), 0o600))

	var payload *validatorpb.ProposerSettingsPayload
	require.NoError(t, config.UnmarshalFromFile(file, &payload))
	settings, err := proposer.SettingFromConsensus(payload)
	require.NoError(t, err)

	bc := settings.DefaultConfig.BuilderConfig
	require.NotNil(t, bc)
	require.Equal(t, 1, len(bc.Builders))
	require.Equal(t, "http://builder-a:8080", bc.Builders[0].URL)
}
