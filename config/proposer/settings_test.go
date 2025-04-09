package proposer

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/validator"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func Test_Proposer_Setting_Cloning(t *testing.T) {
	key1hex := "0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a"
	key1, err := hexutil.Decode(key1hex)
	require.NoError(t, err)
	settings := &Settings{
		ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
			bytesutil.ToBytes48(key1): {
				FeeRecipientConfig: &FeeRecipientConfig{
					FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
				},
				BuilderConfig: &BuilderConfig{
					Enabled:  true,
					GasLimit: validator.Uint64(40000000),
					Relays:   []string{"https://example-relay.com"},
				},
			},
		},
		DefaultConfig: &Option{
			FeeRecipientConfig: &FeeRecipientConfig{
				FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
			},
			BuilderConfig: &BuilderConfig{
				Enabled:  false,
				GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
				Relays:   []string{"https://example-relay.com"},
			},
		},
	}
	t.Run("Happy Path Cloning", func(t *testing.T) {
		clone := settings.Clone()
		require.DeepEqual(t, settings, clone)
		option, ok := settings.ProposeConfig[bytesutil.ToBytes48(key1)]
		require.Equal(t, true, ok)
		newFeeRecipient := "0x44455530FCE8a85ec7055A5F8b2bE214B3DaeFd3"
		option.FeeRecipientConfig.FeeRecipient = common.HexToAddress(newFeeRecipient)
		coption, k := clone.ProposeConfig[bytesutil.ToBytes48(key1)]
		require.Equal(t, true, k)
		require.NotEqual(t, option.FeeRecipientConfig.FeeRecipient.Hex(), coption.FeeRecipientConfig.FeeRecipient.Hex())
		require.Equal(t, "0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3", coption.FeeRecipientConfig.FeeRecipient.Hex())
	})
	t.Run("Happy Path Cloning Builder config", func(t *testing.T) {
		clone := settings.DefaultConfig.BuilderConfig.Clone()
		require.DeepEqual(t, settings.DefaultConfig.BuilderConfig, clone)
		settings.DefaultConfig.BuilderConfig.GasLimit = 1
		require.NotEqual(t, settings.DefaultConfig.BuilderConfig.GasLimit, clone.GasLimit)
	})

	t.Run("Happy Path BuilderConfigFromConsensus", func(t *testing.T) {
		clone := settings.DefaultConfig.BuilderConfig.Clone()
		config := BuilderConfigFromConsensus(clone.ToConsensus())
		require.DeepEqual(t, config.Relays, clone.Relays)
		require.Equal(t, config.Enabled, clone.Enabled)
		require.Equal(t, config.GasLimit, clone.GasLimit)
	})
	t.Run("To Payload and SettingFromConsensus", func(t *testing.T) {
		payload := settings.ToConsensus()
		option, ok := settings.ProposeConfig[bytesutil.ToBytes48(key1)]
		require.Equal(t, true, ok)
		fee := option.FeeRecipientConfig.FeeRecipient.Hex()
		potion, pok := payload.ProposerConfig[key1hex]
		require.Equal(t, true, pok)
		require.Equal(t, option.FeeRecipientConfig.FeeRecipient.Hex(), potion.FeeRecipient)
		require.Equal(t, settings.DefaultConfig.FeeRecipientConfig.FeeRecipient.Hex(), payload.DefaultConfig.FeeRecipient)
		require.Equal(t, settings.DefaultConfig.BuilderConfig.Enabled, payload.DefaultConfig.Builder.Enabled)
		potion.FeeRecipient = fee
		newSettings, err := SettingFromConsensus(payload)
		require.NoError(t, err)
		noption, ok := newSettings.ProposeConfig[bytesutil.ToBytes48(key1)]
		require.Equal(t, true, ok)
		require.Equal(t, option.FeeRecipientConfig.FeeRecipient.Hex(), noption.FeeRecipientConfig.FeeRecipient.Hex())
		require.Equal(t, option.BuilderConfig.GasLimit, option.BuilderConfig.GasLimit)
		require.Equal(t, option.BuilderConfig.Enabled, option.BuilderConfig.Enabled)
	})
}

func TestProposerSettings_ShouldBeSaved(t *testing.T) {
	key1hex := "0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a"
	key1, err := hexutil.Decode(key1hex)
	require.NoError(t, err)
	type fields struct {
		ProposeConfig map[[fieldparams.BLSPubkeyLength]byte]*Option
		DefaultConfig *Option
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "Should be saved, proposeconfig populated and no default config",
			fields: fields{
				ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
					bytesutil.ToBytes48(key1): {
						FeeRecipientConfig: &FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
						},
						BuilderConfig: &BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(40000000),
							Relays:   []string{"https://example-relay.com"},
						},
					},
				},
				DefaultConfig: nil,
			},
			want: true,
		},
		{
			name: "Should be saved, default populated and no proposeconfig ",
			fields: fields{
				ProposeConfig: nil,
				DefaultConfig: &Option{
					FeeRecipientConfig: &FeeRecipientConfig{
						FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
					},
					BuilderConfig: &BuilderConfig{
						Enabled:  true,
						GasLimit: validator.Uint64(40000000),
						Relays:   []string{"https://example-relay.com"},
					},
				},
			},
			want: true,
		},
		{
			name: "Should be saved, all populated",
			fields: fields{
				ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
					bytesutil.ToBytes48(key1): {
						FeeRecipientConfig: &FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
						},
						BuilderConfig: &BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(40000000),
							Relays:   []string{"https://example-relay.com"},
						},
					},
				},
				DefaultConfig: &Option{
					FeeRecipientConfig: &FeeRecipientConfig{
						FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
					},
					BuilderConfig: &BuilderConfig{
						Enabled:  true,
						GasLimit: validator.Uint64(40000000),
						Relays:   []string{"https://example-relay.com"},
					},
				},
			},
			want: true,
		},

		{
			name: "Should not be saved, proposeconfig not populated and default not populated",
			fields: fields{
				ProposeConfig: nil,
				DefaultConfig: nil,
			},
			want: false,
		},
		{
			name: "Should not be saved, builder data only",
			fields: fields{
				ProposeConfig: nil,
				DefaultConfig: &Option{
					BuilderConfig: &BuilderConfig{
						Enabled:  true,
						GasLimit: validator.Uint64(40000000),
						Relays:   []string{"https://example-relay.com"},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &Settings{
				ProposeConfig: tt.fields.ProposeConfig,
				DefaultConfig: tt.fields.DefaultConfig,
			}
			if got := settings.ShouldBeSaved(); got != tt.want {
				t.Errorf("ShouldBeSaved() = %v, want %v", got, tt.want)
			}
		})
	}
}
