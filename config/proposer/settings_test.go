package proposer

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	logtest "github.com/sirupsen/logrus/hooks/test"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func uint64ValPtr(v uint64) *validator.Uint64 {
	u := validator.Uint64(v)
	return &u
}

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
					Enabled:             true,
					GasLimit:            validator.Uint64(40000000),
					MaxExecutionPayment: uint64ValPtr(1000000000),
				},
			},
		},
		DefaultConfig: &Option{
			FeeRecipientConfig: &FeeRecipientConfig{
				FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
			},
			BuilderConfig: &BuilderConfig{
				Enabled:             false,
				GasLimit:            validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
				MaxExecutionPayment: uint64ValPtr(2000000000),
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
	t.Run("Cloning preserves the use-none builders marker", func(t *testing.T) {
		clone := (&BuilderConfig{Enabled: true, Builders: []*BuilderEntry{}}).Clone()
		require.NotNil(t, clone.Builders)
		require.Equal(t, 0, len(clone.Builders))
	})
	t.Run("Consensus round-trip preserves the use-none builders marker", func(t *testing.T) {
		got := BuilderConfigFromConsensus((&BuilderConfig{Enabled: true, Builders: []*BuilderEntry{}}).ToConsensus())
		require.NotNil(t, got.Builders)
		require.Equal(t, 0, len(got.Builders))
		// And nil stays nil, meaning "inherit".
		require.IsNil(t, BuilderConfigFromConsensus((&BuilderConfig{Enabled: true}).ToConsensus()).Builders)
	})

	t.Run("Happy Path BuilderConfigFromConsensus", func(t *testing.T) {
		clone := settings.DefaultConfig.BuilderConfig.Clone()
		config := BuilderConfigFromConsensus(clone.ToConsensus())
		require.DeepEqual(t, config.Enabled, clone.Enabled)
		require.Equal(t, config.GasLimit, clone.GasLimit)
		require.DeepEqual(t, config.MaxExecutionPayment, clone.MaxExecutionPayment)
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
		require.Equal(t, settings.DefaultConfig.BuilderConfig.Enabled, payload.DefaultConfig.Builder.GetEnabled())
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
					},
				},
			},
			want: true,
		},

		{
			name: "Should be saved, default gas limit only",
			fields: fields{
				ProposeConfig: nil,
				DefaultConfig: &Option{
					GasLimit: validator.Uint64(40000000),
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

	t.Run("v2 returns per-validator GasLimit over default", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {GasLimit: validator.Uint64(55_000_000)},
			},
			DefaultConfig: &Option{GasLimit: validator.Uint64(42_000_000)},
		}
		require.Equal(t, validator.Uint64(55_000_000), ps.GasLimit(pk))
	})

	t.Run("v2 falls back to DefaultConfig.GasLimit", func(t *testing.T) {
		ps := &Settings{
			Version:       SchemaV2,
			DefaultConfig: &Option{GasLimit: validator.Uint64(42_000_000)},
		}
		require.Equal(t, validator.Uint64(42_000_000), ps.GasLimit(pk))
	})

	t.Run("v2 unset returns chain default", func(t *testing.T) {
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

	t.Run("v1 per-validator entry without builder falls back to default", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {},
			},
			DefaultConfig: &Option{BuilderConfig: &BuilderConfig{GasLimit: validator.Uint64(60_000_000)}},
		}
		require.Equal(t, validator.Uint64(60_000_000), ps.GasLimit(pk))
	})

	t.Run("v1 per-validator zero BuilderConfig.GasLimit falls back to default", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {BuilderConfig: &BuilderConfig{Enabled: true}},
			},
			DefaultConfig: &Option{BuilderConfig: &BuilderConfig{GasLimit: validator.Uint64(60_000_000)}},
		}
		require.Equal(t, validator.Uint64(60_000_000), ps.GasLimit(pk))
	})

	t.Run("v1 zero default BuilderConfig.GasLimit falls back to chain default", func(t *testing.T) {
		ps := &Settings{
			DefaultConfig: &Option{BuilderConfig: &BuilderConfig{Enabled: true}},
		}
		require.Equal(t, chainDefault, ps.GasLimit(pk))
	})

	t.Run("v1 with no builder config returns chain default", func(t *testing.T) {
		ps := &Settings{DefaultConfig: &Option{}}
		require.Equal(t, chainDefault, ps.GasLimit(pk))
	})
}

func TestSettings_SetGasLimit(t *testing.T) {
	pubkey, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err)
	pk := bytesutil.ToBytes48(pubkey)

	t.Run("nil settings rejects", func(t *testing.T) {
		var ps *Settings
		err := ps.SetGasLimit(pk, validator.Uint64(70_000_000))
		require.ErrorContains(t, "No proposer settings were found to update", err)
	})

	t.Run("writes per-validator option-level GasLimit", func(t *testing.T) {
		ps := &Settings{Version: SchemaV2}
		require.NoError(t, ps.SetGasLimit(pk, validator.Uint64(70_000_000)))
		require.Equal(t, validator.Uint64(70_000_000), ps.ProposeConfig[pk].GasLimit)
	})

	t.Run("updates existing per-validator entry", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {GasLimit: validator.Uint64(10_000_000)},
			},
		}
		require.NoError(t, ps.SetGasLimit(pk, validator.Uint64(20_000_000)))
		require.Equal(t, validator.Uint64(20_000_000), ps.ProposeConfig[pk].GasLimit)
	})

	t.Run("v1 settings accept option-level writes without builder gating", func(t *testing.T) {
		// The option-level value feeds pre-gloas registrations first, so the
		// write no longer requires an enabled builder.
		ps := &Settings{
			DefaultConfig: &Option{BuilderConfig: &BuilderConfig{Enabled: false}},
		}
		require.NoError(t, ps.SetGasLimit(pk, validator.Uint64(80_000_000)))
		require.Equal(t, validator.Uint64(80_000_000), ps.ProposeConfig[pk].GasLimit)
		require.IsNil(t, ps.ProposeConfig[pk].BuilderConfig)
	})

	t.Run("v1 per-key builder entry keeps its builder config untouched", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(10_000_000)}},
			},
		}
		require.NoError(t, ps.SetGasLimit(pk, validator.Uint64(20_000_000)))
		require.Equal(t, validator.Uint64(20_000_000), ps.ProposeConfig[pk].GasLimit)
		// The legacy builder-level value stays; option-level wins on every read.
		require.Equal(t, validator.Uint64(10_000_000), ps.ProposeConfig[pk].BuilderConfig.GasLimit)
		require.Equal(t, validator.Uint64(20_000_000), ps.GasLimit(pk))
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

	t.Run("v2 returns false for missing per-validator entry", func(t *testing.T) {
		ps := &Settings{Version: SchemaV2}
		require.Equal(t, false, ps.ResetGasLimit(pk))
	})

	t.Run("v2 resets per-validator to DefaultConfig.GasLimit", func(t *testing.T) {
		ps := &Settings{
			Version:       SchemaV2,
			DefaultConfig: &Option{GasLimit: validator.Uint64(40_000_000)},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {GasLimit: validator.Uint64(99_000_000)},
			},
		}
		require.Equal(t, true, ps.ResetGasLimit(pk))
		require.Equal(t, validator.Uint64(40_000_000), ps.ProposeConfig[pk].GasLimit)
	})

	t.Run("v2 resets per-validator to unset when no default", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {GasLimit: validator.Uint64(99_000_000)},
			},
		}
		require.Equal(t, true, ps.ResetGasLimit(pk))
		// Unset rather than pinned to today's chain default, so the key follows
		// future default gas limit increases.
		require.Equal(t, validator.Uint64(0), ps.ProposeConfig[pk].GasLimit)
		require.Equal(t, chainDefault, ps.GasLimit(pk))
	})

	t.Run("v1 returns false for missing per-validator entry", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{},
		}
		require.Equal(t, false, ps.ResetGasLimit(pk))
	})

	t.Run("v1 returns false for nil per-validator entry", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{pk: nil},
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

func TestSettings_WarnDeprecatedSchema(t *testing.T) {
	v1Settings := &Settings{
		Version: SchemaV1,
		DefaultConfig: &Option{
			BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: 30000000},
		},
	}

	t.Run("v1 on gloas-scheduled network warns", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 100
		params.OverrideBeaconConfig(cfg)
		hook := logtest.NewGlobal()
		v1Settings.WarnDeprecatedSchema()
		assert.LogsContain(t, hook, "deprecated v1 builder fields")
	})
	t.Run("v1 without gloas scheduled silent", func(t *testing.T) {
		hook := logtest.NewGlobal()
		v1Settings.WarnDeprecatedSchema()
		assert.LogsDoNotContain(t, hook, "deprecated v1 builder fields")
	})
	t.Run("v2 stamp with v1 content still warns", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 100
		params.OverrideBeaconConfig(cfg)
		hook := logtest.NewGlobal()
		mixed := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				BuilderConfig: &BuilderConfig{Enabled: true, Builders: []*BuilderEntry{{URL: "https://b.example"}}},
			},
		}
		mixed.WarnDeprecatedSchema()
		assert.LogsContain(t, hook, "deprecated v1 builder fields")
	})
	t.Run("pure v2 content silent", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 100
		params.OverrideBeaconConfig(cfg)
		hook := logtest.NewGlobal()
		v2 := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				GasLimit:      30000000,
				BuilderConfig: &BuilderConfig{Builders: []*BuilderEntry{{URL: "https://b.example"}}},
			},
		}
		v2.WarnDeprecatedSchema()
		assert.LogsDoNotContain(t, hook, "deprecated v1 builder fields")
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

	t.Run("v1 builder gas limit is not promoted; the builder config is dropped", func(t *testing.T) {
		ps := &Settings{
			DefaultConfig: &Option{
				BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(42_000_000)},
			},
		}
		require.Equal(t, true, ps.UpgradeToV2())
		require.Equal(t, SchemaV2, ps.Version)
		// v1 gas limits are not carried over: the key follows the chain default.
		require.Equal(t, validator.Uint64(0), ps.DefaultConfig.GasLimit)
		require.IsNil(t, ps.DefaultConfig.BuilderConfig)
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

	t.Run("per-validator builder gas limits are dropped, explicit top-level ones kept", func(t *testing.T) {
		pubkey2, err := hexutil.Decode("0xbedefeaa94e03438ea819bd4033c6c1bf6b04320ee2075b77273c08d02f8a61bcc303c2cdddddddddddddddddddddddd")
		require.NoError(t, err)
		pk2 := bytesutil.ToBytes48(pubkey2)
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk:  {BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(35_000_000)}},
				pk2: {GasLimit: validator.Uint64(50_000_000), BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(40_000_000)}},
			},
		}
		require.Equal(t, true, ps.UpgradeToV2())
		require.Equal(t, SchemaV2, ps.Version)
		require.Equal(t, true, ps.DefaultConfig == nil)
		require.Equal(t, validator.Uint64(0), ps.ProposeConfig[pk].GasLimit)
		require.IsNil(t, ps.ProposeConfig[pk].BuilderConfig)
		// An explicitly set top-level gas limit is not builder content and survives.
		require.Equal(t, validator.Uint64(50_000_000), ps.ProposeConfig[pk2].GasLimit)
		require.IsNil(t, ps.ProposeConfig[pk2].BuilderConfig)
	})

	t.Run("v1 content under a v2 stamp is still scrubbed", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(42_000_000)},
			},
		}
		require.Equal(t, true, ps.UpgradeToV2())
		require.Equal(t, validator.Uint64(0), ps.DefaultConfig.GasLimit)
		require.IsNil(t, ps.DefaultConfig.BuilderConfig)
		require.Equal(t, false, ps.UpgradeToV2())
	})

	t.Run("mixed config keeps its v2 fields and loses the v1 ones", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				BuilderConfig: &BuilderConfig{
					Enabled:  true,
					GasLimit: validator.Uint64(42_000_000),
					Builders: []*BuilderEntry{{URL: "https://b.example"}},
				},
			},
		}
		require.Equal(t, true, ps.UpgradeToV2())
		bc := ps.DefaultConfig.BuilderConfig
		require.NotNil(t, bc)
		require.Equal(t, false, bc.Enabled)
		require.Equal(t, validator.Uint64(0), bc.GasLimit)
		require.Equal(t, 1, len(bc.Builders))
		require.Equal(t, false, ps.UpgradeToV2())
	})

	t.Run("explicit empty builders list survives the scrub", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				BuilderConfig: &BuilderConfig{Enabled: true, Builders: []*BuilderEntry{}},
			},
		}
		require.Equal(t, true, ps.UpgradeToV2())
		require.NotNil(t, ps.DefaultConfig.BuilderConfig)
		require.NotNil(t, ps.DefaultConfig.BuilderConfig.Builders)
		require.Equal(t, 0, len(ps.DefaultConfig.BuilderConfig.Builders))
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
}

func TestSettings_TargetGasLimit(t *testing.T) {
	chainDefault := validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	pubkey, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err)
	pk := bytesutil.ToBytes48(pubkey)

	t.Run("nil settings returns chain default", func(t *testing.T) {
		var ps *Settings
		require.Equal(t, chainDefault, ps.TargetGasLimit(pk, 0))
	})

	t.Run("per-validator wins over default", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {GasLimit: validator.Uint64(55_000_000)},
			},
			DefaultConfig: &Option{GasLimit: validator.Uint64(42_000_000)},
		}
		require.Equal(t, validator.Uint64(55_000_000), ps.TargetGasLimit(pk, 0))
	})

	t.Run("falls back to default then chain default", func(t *testing.T) {
		ps := &Settings{DefaultConfig: &Option{GasLimit: validator.Uint64(42_000_000)}}
		require.Equal(t, validator.Uint64(42_000_000), ps.TargetGasLimit(pk, 0))
		require.Equal(t, chainDefault, (&Settings{}).TargetGasLimit(pk, 0))
	})

	t.Run("builder gas limits are never consulted", func(t *testing.T) {
		ps := &Settings{
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				pk: {BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(35_000_000)}},
			},
			DefaultConfig: &Option{BuilderConfig: &BuilderConfig{Enabled: true, GasLimit: validator.Uint64(40_000_000)}},
		}
		require.Equal(t, chainDefault, ps.TargetGasLimit(pk, 0))
	})
}

func TestSettings_TargetGasLimit_Schedule(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 100
	cfg.GasLimitSchedule = []params.GasLimitScheduleEntry{
		{Epoch: 100, GasLimit: 60_000_000},
		{Epoch: 200, GasLimit: 90_000_000},
	}
	params.OverrideBeaconConfig(cfg)

	pubkey, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err)
	pk := bytesutil.ToBytes48(pubkey)

	t.Run("scheduled value used when unconfigured", func(t *testing.T) {
		var ps *Settings
		require.Equal(t, validator.Uint64(60_000_000), ps.TargetGasLimit(pk, 100))
		require.Equal(t, validator.Uint64(60_000_000), ps.TargetGasLimit(pk, 199))
		require.Equal(t, validator.Uint64(90_000_000), ps.TargetGasLimit(pk, 200))
	})

	t.Run("pre-gloas ignores the schedule", func(t *testing.T) {
		var ps *Settings
		require.Equal(t, validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit), ps.TargetGasLimit(pk, 99))
	})

	t.Run("operator value is honored above the schedule with a warning", func(t *testing.T) {
		hook := logtest.NewGlobal()
		warnedGasLimitScheduleEpoch.Store(0)
		ps := &Settings{DefaultConfig: &Option{GasLimit: validator.Uint64(100_000_000)}}
		require.Equal(t, validator.Uint64(100_000_000), ps.TargetGasLimit(pk, 200))
		require.LogsContain(t, hook, "exceeds the recommended maximum")
		hook.Reset()
		require.Equal(t, validator.Uint64(100_000_000), ps.TargetGasLimit(pk, 200))
		require.LogsDoNotContain(t, hook, "exceeds the recommended maximum")
	})

	t.Run("operator value below the schedule is honored with a loud warning", func(t *testing.T) {
		hook := logtest.NewGlobal()
		warnedGasLimitBelowScheduleEpoch.Store(0)
		ps := &Settings{DefaultConfig: &Option{GasLimit: validator.Uint64(50_000_000)}}
		require.Equal(t, validator.Uint64(50_000_000), ps.TargetGasLimit(pk, 200))
		require.LogsContain(t, hook, "below the scheduled network gas limit")
		require.LogsDoNotContain(t, hook, "exceeds the recommended maximum")
		// Deduplicated within the epoch, warned again in the next one.
		hook.Reset()
		require.Equal(t, validator.Uint64(50_000_000), ps.TargetGasLimit(pk, 200))
		require.LogsDoNotContain(t, hook, "below the scheduled network gas limit")
		require.Equal(t, validator.Uint64(50_000_000), ps.TargetGasLimit(pk, 201))
		require.LogsContain(t, hook, "below the scheduled network gas limit")
	})

	t.Run("operator value matching the schedule warns nothing", func(t *testing.T) {
		hook := logtest.NewGlobal()
		warnedGasLimitScheduleEpoch.Store(0)
		warnedGasLimitBelowScheduleEpoch.Store(0)
		ps := &Settings{DefaultConfig: &Option{GasLimit: validator.Uint64(90_000_000)}}
		require.Equal(t, validator.Uint64(90_000_000), ps.TargetGasLimit(pk, 200))
		require.LogsDoNotContain(t, hook, "below the scheduled network gas limit")
		require.LogsDoNotContain(t, hook, "exceeds the recommended maximum")
	})
}

func TestSettingFromConsensus(t *testing.T) {
	// Persisted payloads may predate url-required and (url, auth_data) uniqueness:
	// url-less entries drop, (url, auth) duplicates keep the first, and an omitted
	// auth_data compares as its derived value (the url's UTF-8 bytes).
	t.Run("dedups builders", func(t *testing.T) {
		payload := &validatorpb.ProposerSettingsPayload{
			Version: SchemaV2,
			DefaultConfig: &validatorpb.ProposerOptionPayload{
				Builder: &validatorpb.BuilderConfig{
					Builders: []*validatorpb.BuilderEntry{
						{Url: "https://b.example", AuthData: []byte("first")},
						{Url: "https://b.example", AuthData: []byte("second")},
						{Url: "https://b.example", AuthData: []byte("first")},
						{Url: "https://other.example"},
						{Url: "https://other.example", AuthData: []byte("https://other.example")},
						{AuthData: []byte("url-less")},
					},
				},
			},
		}
		ps, err := SettingFromConsensus(payload)
		require.NoError(t, err)
		builders := ps.DefaultConfig.BuilderConfig.Builders
		require.Equal(t, 3, len(builders))
		require.DeepEqual(t, []byte("first"), builders[0].AuthData)
		require.DeepEqual(t, []byte("second"), builders[1].AuthData)
		require.Equal(t, "https://other.example", builders[2].URL)
	})

	t.Run("v1 explicit max_execution_payment survives ingest", func(t *testing.T) {
		key := [fieldparams.BLSPubkeyLength]byte{9}
		legacy := &Settings{
			Version: SchemaV1,
			DefaultConfig: &Option{BuilderConfig: &BuilderConfig{
				GasLimit:            validator.Uint64(30000000),
				MaxExecutionPayment: uint64ValPtr(0),
			}},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				key: {BuilderConfig: &BuilderConfig{GasLimit: validator.Uint64(25000000)}},
			},
		}

		got, err := SettingFromConsensus(legacy.ToConsensus())
		require.NoError(t, err)

		def := got.DefaultConfig.BuilderConfig
		require.Equal(t, false, def.Enabled)
		// The explicit trustless-only ceiling is preserved, not stripped.
		require.Equal(t, validator.Uint64(0), *def.MaxExecutionPayment)

		perKey := got.ProposeConfig[key].BuilderConfig
		require.Equal(t, false, perKey.Enabled)
		require.Equal(t, (*validator.Uint64)(nil), perKey.MaxExecutionPayment)
	})

	t.Run("v2 presence preserved", func(t *testing.T) {
		v2 := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{BuilderConfig: &BuilderConfig{
				MaxExecutionPayment: uint64ValPtr(0),
			}},
		}
		got, err := SettingFromConsensus(v2.ToConsensus())
		require.NoError(t, err)
		bc := got.DefaultConfig.BuilderConfig
		require.Equal(t, false, bc.Enabled)
		require.NotNil(t, bc.MaxExecutionPayment)
		require.Equal(t, validator.Uint64(0), *bc.MaxExecutionPayment)
	})
}

func TestRegistrationFor(t *testing.T) {
	key := [fieldparams.BLSPubkeyLength]byte{7}
	recipient := common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3")

	t.Run("v2 with no builder config anywhere does not register", func(t *testing.T) {
		ps := &Settings{
			Version:       SchemaV2,
			DefaultConfig: &Option{FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient}},
		}
		fr, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, false, enabled)
		require.Equal(t, recipient, fr)
	})

	t.Run("v2 inherits the default builders and fee recipient", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				GasLimit:           123,
				BuilderConfig:      &BuilderConfig{GasLimit: 456, Builders: []*BuilderEntry{{URL: "https://b.example"}}},
			},
		}
		fr, gl, enabled := ps.RegistrationFor(key)
		require.Equal(t, true, enabled)
		require.Equal(t, recipient, fr)
		// Only the explicitly set option-level gas limit is read; never the builder's.
		require.Equal(t, validator.Uint64(123), gl)
	})

	t.Run("v2 without a fee recipient at any level does not register", func(t *testing.T) {
		ps := &Settings{
			Version:       SchemaV2,
			DefaultConfig: &Option{BuilderConfig: &BuilderConfig{Builders: []*BuilderEntry{{URL: "https://b.example"}}}},
		}
		_, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, false, enabled)
	})

	t.Run("legacy enabled content registers regardless of version stamp", func(t *testing.T) {
		// Semantics are fork-keyed, not version-keyed: registrations exist only
		// pre-gloas, where legacy enabled content stays authoritative.
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{Enabled: true, MinBid: uint64ValPtr(1)},
			},
		}
		_, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, true, enabled)
	})

	t.Run("no builder content anywhere does not register", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{MinBid: uint64ValPtr(1)},
			},
		}
		_, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, false, enabled)
	})

	t.Run("v2-only extras are neutral: key keeps the enabled default's toggle", func(t *testing.T) {
		// A gloas-only preference (min_bid, no builders list) must not silently
		// opt the key out of an enabled v1 default.
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{Enabled: true},
			},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				key: {BuilderConfig: &BuilderConfig{MinBid: uint64ValPtr(5)}},
			},
		}
		_, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, true, enabled)
	})

	t.Run("per-key explicit empty builders opts out of an enabled v1 default", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{Enabled: true},
			},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				key: {BuilderConfig: &BuilderConfig{Builders: []*BuilderEntry{}}},
			},
		}
		_, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, false, enabled)
	})

	t.Run("transition config registers: enabled with explicit empty builders", func(t *testing.T) {
		// enabled:true + builders:[] means mev-boost until the fork, self-build after.
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{Enabled: true, Builders: []*BuilderEntry{}},
			},
		}
		_, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, true, enabled)
	})

	t.Run("per-key v1 disable wins over a default with builders", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{Builders: []*BuilderEntry{{URL: "https://b.example"}}},
			},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				key: {BuilderConfig: &BuilderConfig{Enabled: false}},
			},
		}
		_, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, false, enabled)
	})

	t.Run("v1 per-key disabled builder opts the key out", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV1,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{Enabled: true, GasLimit: 123},
			},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				key: {
					FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
					BuilderConfig:      &BuilderConfig{Enabled: false},
				},
			},
		}
		_, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, false, enabled)
	})

	perKeyRecipient := common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A")

	t.Run("v2 per-key fee recipient and explicit empty builders both win over the default", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{Builders: []*BuilderEntry{{URL: "https://b.example"}}},
			},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				key: {
					FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: perKeyRecipient},
					BuilderConfig:      &BuilderConfig{Builders: []*BuilderEntry{}},
				},
			},
		}
		fr, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, false, enabled)
		require.Equal(t, perKeyRecipient, fr)
	})

	t.Run("v2 option-level gas limit wins over builder-level", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{GasLimit: 123, Builders: []*BuilderEntry{{URL: "https://b.example"}}},
			},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				key: {GasLimit: 999},
			},
		}
		_, gl, enabled := ps.RegistrationFor(key)
		require.Equal(t, true, enabled)
		// The option-level value (written by UpgradeToV2 and the gas-limit API) wins.
		require.Equal(t, validator.Uint64(999), gl)
	})

	t.Run("v2 unset gas limit falls back to the chain default", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV2,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{Builders: []*BuilderEntry{{URL: "https://b.example"}}},
			},
		}
		_, gl, enabled := ps.RegistrationFor(key)
		require.Equal(t, true, enabled)
		require.Equal(t, validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit), gl)
	})

	t.Run("v1 default enabled registers with the default gas limit", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV1,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{Enabled: true, GasLimit: 456},
			},
		}
		fr, gl, enabled := ps.RegistrationFor(key)
		require.Equal(t, true, enabled)
		require.Equal(t, recipient, fr)
		require.Equal(t, validator.Uint64(456), gl)
	})

	t.Run("v1 per-key fee recipient without builder config keeps the default's toggle", func(t *testing.T) {
		ps := &Settings{
			Version: SchemaV1,
			DefaultConfig: &Option{
				FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &BuilderConfig{Enabled: true, GasLimit: 456},
			},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				key: {FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: perKeyRecipient}},
			},
		}
		fr, gl, enabled := ps.RegistrationFor(key)
		require.Equal(t, true, enabled)
		require.Equal(t, perKeyRecipient, fr)
		require.Equal(t, validator.Uint64(456), gl)
	})

	t.Run("v1 zero builder gas limit falls back to the chain default", func(t *testing.T) {
		// API-created builder configs have no builder-level gas limit; a v1
		// registration must not advertise gas limit 0.
		ps := &Settings{
			Version: SchemaV1,
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
				key: {
					FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient},
					BuilderConfig:      &BuilderConfig{Enabled: true},
				},
			},
		}
		_, gl, enabled := ps.RegistrationFor(key)
		require.Equal(t, true, enabled)
		require.Equal(t, validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit), gl)
	})

	t.Run("unset version uses v1 semantics", func(t *testing.T) {
		ps := &Settings{
			Version:       SchemaV1Unset,
			DefaultConfig: &Option{FeeRecipientConfig: &FeeRecipientConfig{FeeRecipient: recipient}},
		}
		_, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, false, enabled)
	})

	t.Run("nil settings never register", func(t *testing.T) {
		var ps *Settings
		fr, _, enabled := ps.RegistrationFor(key)
		require.Equal(t, false, enabled)
		require.Equal(t, common.HexToAddress(params.BeaconConfig().EthBurnAddressHex), fr)
	})
}

// Version-neutral settings (fee recipient, graffiti) and pure v2 builder content
// give the deprecation warning nothing to say; only v1 fields trigger it.
func TestHasLegacyBuilderContent(t *testing.T) {
	key := [fieldparams.BLSPubkeyLength]byte{9}
	ps := &Settings{
		Version:       SchemaV1,
		DefaultConfig: &Option{FeeRecipientConfig: &FeeRecipientConfig{}},
		ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
			key: {GraffitiConfig: &GraffitiConfig{Graffiti: "hi"}},
		},
	}
	require.Equal(t, false, ps.HasLegacyBuilderContent())
	ps.ProposeConfig[key].BuilderConfig = &BuilderConfig{MinBid: uint64ValPtr(1), Builders: []*BuilderEntry{}}
	require.Equal(t, false, ps.HasLegacyBuilderContent())
	ps.ProposeConfig[key].BuilderConfig.GasLimit = 30000000
	require.Equal(t, true, ps.HasLegacyBuilderContent())
	ps.ProposeConfig[key].BuilderConfig.GasLimit = 0
	ps.DefaultConfig.BuilderConfig = &BuilderConfig{Enabled: true}
	require.Equal(t, true, ps.HasLegacyBuilderContent())
	var nilSettings *Settings
	require.Equal(t, false, nilSettings.HasLegacyBuilderContent())
}

// The cutover scrubs v1 builder fields; v2 content — including an explicit
// max_execution_payment — survives it.
func TestUpgradeToV2_DropsBuilderContent(t *testing.T) {
	key := [fieldparams.BLSPubkeyLength]byte{9}
	ps := &Settings{
		Version:       SchemaV1,
		DefaultConfig: &Option{BuilderConfig: &BuilderConfig{MaxExecutionPayment: uint64ValPtr(0)}},
		ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*Option{
			key: {BuilderConfig: &BuilderConfig{Enabled: true}},
		},
	}
	require.Equal(t, true, ps.UpgradeToV2())
	require.Equal(t, SchemaV2, ps.Version)
	// The explicit trustless-only ceiling is v2 content and survives.
	require.NotNil(t, ps.DefaultConfig.BuilderConfig)
	require.Equal(t, validator.Uint64(0), *ps.DefaultConfig.BuilderConfig.MaxExecutionPayment)
	// The pure-v1 per-key config is gone entirely.
	require.IsNil(t, ps.ProposeConfig[key].BuilderConfig)
	require.Equal(t, false, ps.UpgradeToV2())
}
