package proposer

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	logtest "github.com/sirupsen/logrus/hooks/test"
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

func TestSettings_GasLimit(t *testing.T) {
	chainDefault := validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	pubkey, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err)
	pk := bytesutil.ToBytes48(pubkey)

	t.Run("nil settings returns chain default", func(t *testing.T) {
		var ps *Settings
		require.Equal(t, chainDefault, ps.GasLimit(pk))
	})

	t.Run("v2 returns DefaultConfig.GasLimit", func(t *testing.T) {
		ps := &Settings{
			Version:       SchemaV2,
			DefaultConfig: &Option{GasLimit: validator.Uint64(42_000_000)},
		}
		require.Equal(t, validator.Uint64(42_000_000), ps.GasLimit(pk))
	})

	t.Run("v2 unset DefaultConfig.GasLimit returns chain default", func(t *testing.T) {
		ps := &Settings{Version: SchemaV2}
		require.Equal(t, chainDefault, ps.GasLimit(pk))
	})

	t.Run("v1 returns per-validator BuilderConfig.GasLimit", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {BuilderConfig: &BuilderConfig{GasLimit: validator.Uint64(50_000_000)}},
			},
		}
		require.Equal(t, validator.Uint64(50_000_000), ps.GasLimit(pk))
	})

	t.Run("v1 falls back to default BuilderConfig.GasLimit", func(t *testing.T) {
		ps := &Settings{
			DefaultConfig: &Option{BuilderConfig: &BuilderConfig{GasLimit: validator.Uint64(60_000_000)}},
		}
		require.Equal(t, validator.Uint64(60_000_000), ps.GasLimit(pk))
	})
}

func TestSettings_SetGasLimit(t *testing.T) {
	pubkey, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err)
	pk := bytesutil.ToBytes48(pubkey)

	t.Run("nil settings rejects with v1 error message", func(t *testing.T) {
		var ps *Settings
		err := ps.SetGasLimit(pk, validator.Uint64(70_000_000))
		require.ErrorContains(t, "No proposer settings were found to update", err)
	})

	t.Run("v2 writes top-level DefaultConfig.GasLimit and ignores pubkey", func(t *testing.T) {
		ps := &Settings{Version: SchemaV2}
		require.NoError(t, ps.SetGasLimit(pk, validator.Uint64(70_000_000)))
		require.Equal(t, validator.Uint64(70_000_000), ps.DefaultConfig.GasLimit)
		_, found := ps.ProposeConfig[pk]
		require.Equal(t, false, found)
	})

	t.Run("v1 with no builder rejects", func(t *testing.T) {
		ps := &Settings{}
		err := ps.SetGasLimit(pk, validator.Uint64(80_000_000))
		require.ErrorContains(t, "Gas limit changes only apply when builder is enabled", err)
	})

	t.Run("v1 with disabled builder rejects", func(t *testing.T) {
		ps := &Settings{
			DefaultConfig: &Option{BuilderConfig: &BuilderConfig{Enabled: false}},
		}
		err := ps.SetGasLimit(pk, validator.Uint64(80_000_000))
		require.ErrorContains(t, "Gas limit changes only apply when builder is enabled", err)
	})

	t.Run("v1 clones enabled-builder default into new per-validator entry", func(t *testing.T) {
		feeRecipient := common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3")
		ps := &Settings{
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: feeRecipient},
				BuilderConfig:      &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(30_000_000)},
			},
		}
		require.NoError(t, ps.SetGasLimit(pk, validator.Uint64(90_000_000)))
		opt := ps.ProposeConfig[pk]
		require.Equal(t, feeRecipient, opt.FeeRecipientConfig.FeeRecipient)
		require.Equal(t, validator.Uint64(90_000_000), opt.BuilderConfig.GasLimit)
	})

	t.Run("v1 updates existing enabled-builder per-validator entry", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(10_000_000)}},
			},
		}
		require.NoError(t, ps.SetGasLimit(pk, validator.Uint64(20_000_000)))
		require.Equal(t, validator.Uint64(20_000_000), ps.ProposeConfig[pk].BuilderConfig.GasLimit)
	})

	t.Run("v1 per-validator entry with disabled builder rejects", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {BuilderConfig: &BuilderConfig{Enabled: false}},
			},
		}
		err := ps.SetGasLimit(pk, validator.Uint64(20_000_000))
		require.ErrorContains(t, "Gas limit changes only apply when builder is enabled", err)
	})
}

func TestSettings_ResetGasLimit(t *testing.T) {
	chainDefault := validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	pubkey, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err)
	pk := bytesutil.ToBytes48(pubkey)

	t.Run("nil settings returns false", func(t *testing.T) {
		var ps *Settings
		require.Equal(t, false, ps.ResetGasLimit(pk))
	})

	t.Run("v2 with no default returns false", func(t *testing.T) {
		ps := &Settings{Version: SchemaV2}
		require.Equal(t, false, ps.ResetGasLimit(pk))
	})

	t.Run("v2 resets DefaultConfig.GasLimit to chain default", func(t *testing.T) {
		ps := &Settings{
			Version:       SchemaV2,
			DefaultConfig: &Option{GasLimit: validator.Uint64(99_000_000)},
		}
		require.Equal(t, true, ps.ResetGasLimit(pk))
		require.Equal(t, chainDefault, ps.DefaultConfig.GasLimit)
	})

	t.Run("v1 returns false for missing per-validator entry", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{},
		}
		require.Equal(t, false, ps.ResetGasLimit(pk))
	})

	t.Run("v1 resets per-validator to default's BuilderConfig.GasLimit", func(t *testing.T) {
		ps := &Settings{
			DefaultConfig: &Option{BuilderConfig: &BuilderConfig{GasLimit: validator.Uint64(40_000_000)}},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {BuilderConfig: &BuilderConfig{GasLimit: validator.Uint64(99_000_000)}},
			},
		}
		require.Equal(t, true, ps.ResetGasLimit(pk))
		require.Equal(t, validator.Uint64(40_000_000), ps.ProposeConfig[pk].BuilderConfig.GasLimit)
	})

	t.Run("v1 resets per-validator to chain default when no proposer-config default", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {BuilderConfig: &BuilderConfig{GasLimit: validator.Uint64(99_000_000)}},
			},
		}
		require.Equal(t, true, ps.ResetGasLimit(pk))
		require.Equal(t, chainDefault, ps.ProposeConfig[pk].BuilderConfig.GasLimit)
	})
}

func TestSettings_UpgradeToV2(t *testing.T) {
	pubkey, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err)
	pk := bytesutil.ToBytes48(pubkey)

	t.Run("nil settings returns false", func(t *testing.T) {
		var ps *Settings
		require.Equal(t, false, ps.UpgradeToV2())
	})

	t.Run("already v2 returns false", func(t *testing.T) {
		ps := &Settings{Version: SchemaV2}
		require.Equal(t, false, ps.UpgradeToV2())
	})

	t.Run("v1 default lifts BuilderConfig.GasLimit to top-level and drops builder", func(t *testing.T) {
		ps := &Settings{
			DefaultConfig: &Option{
				BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(42_000_000)},
			},
		}
		require.Equal(t, true, ps.UpgradeToV2())
		require.Equal(t, SchemaV2, ps.Version)
		require.Equal(t, validator.Uint64(42_000_000), ps.DefaultConfig.GasLimit)
		require.Equal(t, true, ps.DefaultConfig.BuilderConfig == nil)
	})

	t.Run("v1 top-level GasLimit already set is preserved", func(t *testing.T) {
		ps := &Settings{
			DefaultConfig: &Option{
				GasLimit:      validator.Uint64(70_000_000),
				BuilderConfig: &BuilderConfig{GasLimit: validator.Uint64(42_000_000)},
			},
		}
		require.Equal(t, true, ps.UpgradeToV2())
		require.Equal(t, validator.Uint64(70_000_000), ps.DefaultConfig.GasLimit)
	})

	t.Run("per-validator builder.gas_limit is dropped with warning", func(t *testing.T) {
		hook := logtest.NewGlobal()
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(35_000_000)}},
			},
		}
		require.Equal(t, true, ps.UpgradeToV2())
		require.Equal(t, true, ps.ProposeConfig[pk].BuilderConfig == nil)
		assert.LogsContain(t, hook, "Dropped per-validator builder.gas_limit on 1 key(s)")
	})

	t.Run("nil DefaultConfig still bumps to v2 and drops per-validator builders", func(t *testing.T) {
		hook := logtest.NewGlobal()
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(35_000_000)}},
			},
		}
		require.Equal(t, true, ps.UpgradeToV2())
		require.Equal(t, SchemaV2, ps.Version)
		require.Equal(t, true, ps.DefaultConfig == nil)
		require.Equal(t, true, ps.ProposeConfig[pk].BuilderConfig == nil)
		assert.LogsContain(t, hook, "Dropped per-validator builder.gas_limit on 1 key(s)")
	})

	t.Run("default with no builder and zero GasLimit still bumps to v2", func(t *testing.T) {
		ps := &Settings{
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: common.HexToAddress("0xae967917c465db8578ca9024c205720b1a3651A9")},
			},
		}
		require.Equal(t, true, ps.UpgradeToV2())
		require.Equal(t, SchemaV2, ps.Version)
		require.Equal(t, validator.Uint64(0), ps.DefaultConfig.GasLimit)
		require.Equal(t, true, ps.DefaultConfig.BuilderConfig == nil)
		// Runtime falls back to chain default for zero GasLimit on v2.
		require.Equal(t, validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit), ps.GasLimit(pk))
	})

	t.Run("multiple per-validator entries only count non-zero gas limits", func(t *testing.T) {
		hook := logtest.NewGlobal()
		pubkey2, err := hexutil.Decode("0xbedefeaa94e03438ea819bd4033c6c1bf6b04320ee2075b77273c08d02f8a61bcc303c2cdddddddddddddddddddddddd")
		require.NoError(t, err)
		pk2 := bytesutil.ToBytes48(pubkey2)
		pubkey3, err := hexutil.Decode("0xcccccccc94e03438ea819bd4033c6c1bf6b04320ee2075b77273c08d02f8a61bcc303c2cdddddddddddddddddddddddd")
		require.NoError(t, err)
		pk3 := bytesutil.ToBytes48(pubkey3)
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk:  {BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(35_000_000)}},
				pk2: {BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(40_000_000)}},
				pk3: {BuilderConfig: &BuilderConfig{Enabled: true}},
			},
		}
		require.Equal(t, true, ps.UpgradeToV2())
		require.Equal(t, true, ps.ProposeConfig[pk].BuilderConfig == nil)
		require.Equal(t, true, ps.ProposeConfig[pk2].BuilderConfig == nil)
		require.Equal(t, true, ps.ProposeConfig[pk3].BuilderConfig == nil)
		assert.LogsContain(t, hook, "Dropped per-validator builder.gas_limit on 2 key(s)")
	})
}
