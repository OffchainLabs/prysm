package loader

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/db/iface"
	dbTest "github.com/OffchainLabs/prysm/v7/validator/db/testing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/urfave/cli/v2"
)

func TestProposerSettingsLoader(t *testing.T) {
	hook := logtest.NewGlobal()
	type proposerSettingsFlag struct {
		dir        string
		url        string
		defaultfee string
		defaultgas string
	}

	type args struct {
		proposerSettingsFlagValues *proposerSettingsFlag
	}
	tests := []struct {
		name                         string
		args                         args
		want                         func() *proposer.Settings
		urlResponse                  string
		wantInitErr                  string
		wantErr                      string
		wantLog                      string
		withdb                       func(db iface.ValidatorDB) error
		validatorRegistrationEnabled bool
		skipDBSavedCheck             bool
	}{
		{
			name: "graffiti in db without fee recipient",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							GraffitiConfig: &proposer.GraffitiConfig{
								Graffiti: "specific graffiti",
							},
						},
					},
				}
			},
			withdb: func(db iface.ValidatorDB) error {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				settings := &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							GraffitiConfig: &proposer.GraffitiConfig{
								Graffiti: "specific graffiti",
							},
						},
					},
				}
				return db.SaveProposerSettings(t.Context(), settings)
			},
		},
		{
			name: "graffiti from file",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-graffiti-settings.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							GraffitiConfig: &proposer.GraffitiConfig{
								Graffiti: "some graffiti",
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(30000000),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(40000000),
						},
					},
				}
			},
		},
		{
			name: "db settings override file settings if file default config is missing",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/proposer-config-only.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0xae967917c465db8578ca9024c205720b1a3651A9"),
						},
					},
				}
			},
			withdb: func(db iface.ValidatorDB) error {
				settings := &proposer.Settings{
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0xae967917c465db8578ca9024c205720b1a3651A9"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
				return db.SaveProposerSettings(t.Context(), settings)
			},
		},
		{
			name: "db settings override file settings if file proposer config is missing and enable builder is true",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/default-only-proposer-config.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(40000000),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0xae967917c465db8578ca9024c205720b1a3651A9"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			withdb: func(db iface.ValidatorDB) error {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				settings := &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(40000000),
							},
						},
					},
				}
				return db.SaveProposerSettings(t.Context(), settings)
			},
			validatorRegistrationEnabled: true,
		},
		{
			name: "Empty json file loaded throws a warning",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/empty.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				return nil
			},
			wantLog:          "No proposer settings were provided",
			skipDBSavedCheck: true,
		},
		{
			name: "Happy Path default only proposer settings file with builder settings,",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/default-only-proposer-config.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				return &proposer.Settings{
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0xae967917c465db8578ca9024c205720b1a3651A9"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
		},
		{
			name: "Happy Path Config file File, bad checksum",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config-badchecksum.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0xae967917c465db8578ca9024c205720b1a3651A9"),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0xae967917c465db8578ca9024c205720b1a3651A9"),
						},
					},
				}
			},
			wantErr: "",
			wantLog: "is not a checksum Ethereum address",
		},
		{
			name: "Happy Path Config file File multiple fee recipients",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config-multiple.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				key2, err := hexutil.Decode("0xb057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7b")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
							},
						},
						bytesutil.ToBytes48(key2): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x60155530FCE8a85ec7055A5F8b2bE214B3DaeFd4"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(35000000),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(40000000),
						},
					},
				}
			},
			wantErr: "",
		},
		{
			name: "Happy Path Config URL File",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "./testdata/good-prepare-beacon-proposer-config.json",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
			},
			wantErr: "",
		},
		{
			name: "Happy Path Config YAML file with custom Gas Limit",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config.yaml",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: 40000000,
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  false,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			wantErr: "",
		},
		{
			name: "Happy Path Suggested Fee ",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "0x6e35733c5af9B61374A128e6F85f553aF09ff89A",
				},
			},
			want: func() *proposer.Settings {
				return &proposer.Settings{
					ProposeConfig: nil,
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
			},
			wantErr: "",
		},
		{
			name: "Happy Path Suggested Fee , validator registration enabled",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "0x6e35733c5af9B61374A128e6F85f553aF09ff89A",
				},
			},
			want: func() *proposer.Settings {
				return &proposer.Settings{
					ProposeConfig: nil,
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			wantErr:                      "",
			validatorRegistrationEnabled: true,
		},
		{
			name: "Happy Path Suggested Fee , validator registration enabled and default gas",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "0x6e35733c5af9B61374A128e6F85f553aF09ff89A",
					defaultgas: "50000000",
				},
			},
			want: func() *proposer.Settings {
				return &proposer.Settings{
					ProposeConfig: nil,
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: 50000000,
						},
					},
				}
			},
			wantErr:                      "",
			validatorRegistrationEnabled: true,
		},
		{
			name: "File with default gas that overrides",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config.yaml",
					url:        "",
					defaultfee: "",
					defaultgas: "50000000",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: 50000000,
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  false,
							GasLimit: validator.Uint64(50000000),
						},
					},
				}
			},
			wantErr: "",
		},
		{
			name: "Suggested Fee does not Override Config",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config.json",
					url:        "",
					defaultfee: "0x6e35733c5af9B61374A128e6F85f553aF09ff89B",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
			},
			wantErr: "",
		},
		{
			name: "Suggested Fee with validator registration does not Override Config",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config.json",
					url:        "",
					defaultfee: "0x6e35733c5af9B61374A128e6F85f553aF09ff89B",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			wantErr:                      "",
			validatorRegistrationEnabled: true,
		},
		{
			name: "Enable Builder flag overrides empty config",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			validatorRegistrationEnabled: true,
		},
		{
			name: "Enable Builder flag does override completed builder config",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config.yaml",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(40000000),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			validatorRegistrationEnabled: true,
		},
		{
			name: "Only Enable Builder flag",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				return &proposer.Settings{
					DefaultConfig: &proposer.Option{
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			validatorRegistrationEnabled: true,
			skipDBSavedCheck:             true,
		},
		{
			name: "No Flags but saved to DB with builder and override removed builder data",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
			},
			withdb: func(db iface.ValidatorDB) error {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				settings := &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(40000000),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
				return db.SaveProposerSettings(t.Context(), settings)
			},
		},
		{
			name: "Enable builder flag but saved to DB without builder data now includes builder data",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			withdb: func(db iface.ValidatorDB) error {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				settings := &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
				return db.SaveProposerSettings(t.Context(), settings)
			},
			validatorRegistrationEnabled: true,
		},
		{
			name: "No flags, but saved to database",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
			},
			withdb: func(db iface.ValidatorDB) error {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				settings := &proposer.Settings{
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
				return db.SaveProposerSettings(t.Context(), settings)
			},
		},
		{
			name: "No flags set means empty config",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				return nil
			},
			wantErr:          "",
			skipDBSavedCheck: true,
		},
		{
			name: "Bad File Path",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/bad-prepare-beacon-proposer-config.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				return nil
			},
			wantErr: "failed to unmarshal yaml file",
		},
		{
			name: "Both URL and Dir flags used resulting in error",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config.json",
					url:        "./testdata/good-prepare-beacon-proposer-config.json",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				return &proposer.Settings{}
			},
			wantInitErr: "cannot specify both",
		},
		{
			name: "Bad Gas value in JSON",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/bad-gas-value-proposer-settings.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *proposer.Settings {
				return nil
			},
			wantErr: "failed to unmarshal yaml file",
		},
	}
	for _, tt := range tests {
		for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
			t.Run(fmt.Sprintf("%v-minimal:%v", tt.name, isSlashingProtectionMinimal), func(t *testing.T) {
				app := cli.App{}
				set := flag.NewFlagSet("test", 0)
				if tt.args.proposerSettingsFlagValues.dir != "" {
					set.String(flags.ProposerSettingsFlag.Name, tt.args.proposerSettingsFlagValues.dir, "")
					require.NoError(t, set.Set(flags.ProposerSettingsFlag.Name, tt.args.proposerSettingsFlagValues.dir))
				}
				if tt.args.proposerSettingsFlagValues.url != "" {
					content, err := os.ReadFile(tt.args.proposerSettingsFlagValues.url)
					require.NoError(t, err)
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(200)
						w.Header().Set("Content-Type", "application/json")
						_, err := fmt.Fprintf(w, "%s", content)
						require.NoError(t, err)
					}))
					defer srv.Close()

					set.String(flags.ProposerSettingsURLFlag.Name, tt.args.proposerSettingsFlagValues.url, "")
					require.NoError(t, set.Set(flags.ProposerSettingsURLFlag.Name, srv.URL))
				}
				if tt.args.proposerSettingsFlagValues.defaultfee != "" {
					set.String(flags.SuggestedFeeRecipientFlag.Name, tt.args.proposerSettingsFlagValues.defaultfee, "")
					require.NoError(t, set.Set(flags.SuggestedFeeRecipientFlag.Name, tt.args.proposerSettingsFlagValues.defaultfee))
				}
				if tt.args.proposerSettingsFlagValues.defaultgas != "" {
					set.String(flags.BuilderGasLimitFlag.Name, tt.args.proposerSettingsFlagValues.defaultgas, "")
					require.NoError(t, set.Set(flags.BuilderGasLimitFlag.Name, tt.args.proposerSettingsFlagValues.defaultgas))
				}
				if tt.validatorRegistrationEnabled {
					set.Bool(flags.EnableBuilderFlag.Name, true, "")
				}
				cliCtx := cli.NewContext(&app, set, nil)
				validatorDB := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{}, isSlashingProtectionMinimal)
				if tt.withdb != nil {
					err := tt.withdb(validatorDB)
					require.NoError(t, err)
				}
				loader, err := NewProposerSettingsLoader(
					cliCtx,
					validatorDB,
					WithBuilderConfig(),
					WithGasLimit(),
				)
				if tt.wantInitErr != "" {
					require.ErrorContains(t, tt.wantInitErr, err)
					return
				} else {
					require.NoError(t, err)
				}
				got, err := loader.Load(cliCtx)
				if tt.wantErr != "" {
					require.ErrorContains(t, tt.wantErr, err)
					return
				} else {
					require.NoError(t, err)
				}
				if tt.wantLog != "" {
					assert.LogsContain(t, hook,
						tt.wantLog,
					)
				}
				w := tt.want()
				require.DeepEqual(t, w, got)
				if !tt.skipDBSavedCheck {
					dbSettings, err := validatorDB.ProposerSettings(cliCtx.Context)
					require.NoError(t, err)
					require.DeepEqual(t, w, dbSettings)
				}
			})
		}
	}
}

func Test_ProposerSettingsLoaderWithOnlyBuilder_DoesNotSaveInDB(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("minimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			app := cli.App{}
			set := flag.NewFlagSet("test", 0)
			set.Bool(flags.EnableBuilderFlag.Name, true, "")
			cliCtx := cli.NewContext(&app, set, nil)
			validatorDB := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{}, isSlashingProtectionMinimal)
			loader, err := NewProposerSettingsLoader(
				cliCtx,
				validatorDB,
				WithBuilderConfig(),
				WithGasLimit(),
			)
			require.NoError(t, err)
			got, err := loader.Load(cliCtx)
			require.NoError(t, err)
			_, err = validatorDB.ProposerSettings(cliCtx.Context)
			require.ErrorContains(t, "no proposer settings found in bucket", err)
			want := &proposer.Settings{
				DefaultConfig: &proposer.Option{
					BuilderConfig: &proposer.BuilderConfig{
						Enabled:  true,
						GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						Relays:   nil,
					},
				},
			}
			require.DeepEqual(t, want, got)
		})
	}
}

func Test_ProposerSettingsLoader_GasLimitWithoutBuilder(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("minimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			app := cli.App{}
			set := flag.NewFlagSet("test", 0)
			set.String(flags.SuggestedFeeRecipientFlag.Name, "", "")
			require.NoError(t, set.Set(flags.SuggestedFeeRecipientFlag.Name, "0x6e35733c5af9B61374A128e6F85f553aF09ff89A"))
			set.String(flags.BuilderGasLimitFlag.Name, "", "")
			require.NoError(t, set.Set(flags.BuilderGasLimitFlag.Name, "12345678"))
			cliCtx := cli.NewContext(&app, set, nil)
			validatorDB := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{}, isSlashingProtectionMinimal)
			loader, err := NewProposerSettingsLoader(
				cliCtx,
				validatorDB,
				WithBuilderConfig(),
				WithGasLimit(),
			)
			require.NoError(t, err)
			got, err := loader.Load(cliCtx)
			require.NoError(t, err)
			require.NotNil(t, got)
			require.NotNil(t, got.DefaultConfig)
			require.NotNil(t, got.DefaultConfig.BuilderConfig)
			require.Equal(t, false, got.DefaultConfig.BuilderConfig.Enabled)
			require.Equal(t, validator.Uint64(12345678), got.DefaultConfig.BuilderConfig.GasLimit)
		})
	}
}

func Test_ProposerSettingsLoader_DoesNotMigrateAtLoad(t *testing.T) {
	makeCliCtx := func(t *testing.T) *cli.Context {
		app := cli.App{}
		set := flag.NewFlagSet("test", 0)
		set.String(flags.SuggestedFeeRecipientFlag.Name, "", "")
		require.NoError(t, set.Set(flags.SuggestedFeeRecipientFlag.Name, "0x6e35733c5af9B61374A128e6F85f553aF09ff89A"))
		set.String(flags.BuilderGasLimitFlag.Name, "", "")
		require.NoError(t, set.Set(flags.BuilderGasLimitFlag.Name, "12345678"))
		return cli.NewContext(&app, set, nil)
	}

	t.Run("gloas-configured + --suggested-gas-limit stays v1 (no load-time migration)", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 100
		params.OverrideBeaconConfig(cfg)

		cliCtx := makeCliCtx(t)
		validatorDB := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{}, false)
		loader, err := NewProposerSettingsLoader(
			cliCtx,
			validatorDB,
			WithBuilderConfig(),
			WithGasLimit(),
		)
		require.NoError(t, err)
		got, err := loader.Load(cliCtx)
		require.NoError(t, err)
		require.NotNil(t, got)
		// Migration is deferred; settings stay in v1 form at load time.
		require.Equal(t, uint32(0), got.Version)
		require.Equal(t, validator.Uint64(0), got.DefaultConfig.GasLimit)
		require.NotNil(t, got.DefaultConfig.BuilderConfig)
		require.Equal(t, validator.Uint64(12345678), got.DefaultConfig.BuilderConfig.GasLimit)
	})

	t.Run("non-gloas network + --suggested-gas-limit stays v1", func(t *testing.T) {
		cliCtx := makeCliCtx(t)
		validatorDB := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{}, false)
		loader, err := NewProposerSettingsLoader(
			cliCtx,
			validatorDB,
			WithBuilderConfig(),
			WithGasLimit(),
		)
		require.NoError(t, err)
		got, err := loader.Load(cliCtx)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, uint32(0), got.Version)
		require.Equal(t, validator.Uint64(0), got.DefaultConfig.GasLimit)
		require.NotNil(t, got.DefaultConfig.BuilderConfig)
		require.Equal(t, validator.Uint64(12345678), got.DefaultConfig.BuilderConfig.GasLimit)
	})

	t.Run("gloas-configured + explicit version: 1 in DB stays v1 at load time", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 100
		params.OverrideBeaconConfig(cfg)

		cliCtx := makeCliCtx(t)
		validatorDB := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{}, false)
		seed := &proposer.Settings{
			Version: proposer.SchemaV1,
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{
					FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
				},
				BuilderConfig: &proposer.BuilderConfig{
					Enabled:  false,
					GasLimit: validator.Uint64(99000000),
				},
			},
		}
		require.NoError(t, validatorDB.SaveProposerSettings(cliCtx.Context, seed))

		loader, err := NewProposerSettingsLoader(
			cliCtx,
			validatorDB,
			WithBuilderConfig(),
			WithGasLimit(),
		)
		require.NoError(t, err)
		got, err := loader.Load(cliCtx)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, proposer.SchemaV1, got.Version)
		require.Equal(t, validator.Uint64(0), got.DefaultConfig.GasLimit)
		require.NotNil(t, got.DefaultConfig.BuilderConfig)
		// CLI --suggested-gas-limit applied to BuilderConfig.GasLimit in v1.
		require.Equal(t, validator.Uint64(12345678), got.DefaultConfig.BuilderConfig.GasLimit)
	})

	t.Run("gloas-aware network: no gas signal anywhere stays v1 (runtime uses chain default)", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 100
		params.OverrideBeaconConfig(cfg)

		app := cli.App{}
		set := flag.NewFlagSet("test", 0)
		set.String(flags.SuggestedFeeRecipientFlag.Name, "", "")
		require.NoError(t, set.Set(flags.SuggestedFeeRecipientFlag.Name, "0x6e35733c5af9B61374A128e6F85f553aF09ff89A"))
		cliCtx := cli.NewContext(&app, set, nil)
		validatorDB := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{}, false)

		loader, err := NewProposerSettingsLoader(
			cliCtx,
			validatorDB,
			WithBuilderConfig(),
			WithGasLimit(),
		)
		require.NoError(t, err)
		got, err := loader.Load(cliCtx)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, uint32(0), got.Version)
		require.Equal(t, validator.Uint64(0), got.DefaultConfig.GasLimit)
	})
}

func Test_warnUnusedSchemaFields(t *testing.T) {
	key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err)

	t.Run("v1 silent", func(t *testing.T) {
		hook := logtest.NewGlobal()
		warnUnusedSchemaFields(&proposer.Settings{
			Version: proposer.SchemaV1,
			DefaultConfig: &proposer.Option{
				BuilderConfig: &proposer.BuilderConfig{Enabled: true, GasLimit: 30000000},
			},
		})
		assert.LogsDoNotContain(t, hook, "per-validator 'gas_limit'")
	})
	t.Run("v2 default-only gas_limit silent", func(t *testing.T) {
		hook := logtest.NewGlobal()
		warnUnusedSchemaFields(&proposer.Settings{
			Version: proposer.SchemaV2,
			DefaultConfig: &proposer.Option{
				GasLimit: 30000000,
			},
		})
		assert.LogsDoNotContain(t, hook, "per-validator 'gas_limit'")
	})
	t.Run("v2 per-validator gas_limit warns", func(t *testing.T) {
		hook := logtest.NewGlobal()
		warnUnusedSchemaFields(&proposer.Settings{
			Version: proposer.SchemaV2,
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
				bytesutil.ToBytes48(key1): {
					GasLimit: 30000000,
				},
			},
		})
		assert.LogsContain(t, hook, "per-validator 'gas_limit'")
	})
}

func Test_mergeProposerSettings_VersionPrecedence(t *testing.T) {
	t.Run("loaded.Version wins when non-zero", func(t *testing.T) {
		merged := mergeProposerSettings(
			&validatorpb.ProposerSettingsPayload{Version: proposer.SchemaV2},
			&validatorpb.ProposerSettingsPayload{Version: proposer.SchemaV1},
			&flagOptions{},
		)
		require.Equal(t, uint32(proposer.SchemaV2), merged.Version)
	})
	t.Run("db.Version used when loaded.Version is 0", func(t *testing.T) {
		merged := mergeProposerSettings(
			&validatorpb.ProposerSettingsPayload{},
			&validatorpb.ProposerSettingsPayload{Version: proposer.SchemaV1},
			&flagOptions{},
		)
		require.Equal(t, uint32(proposer.SchemaV1), merged.Version)
	})
	t.Run("loaded.Version used when db is nil", func(t *testing.T) {
		merged := mergeProposerSettings(
			&validatorpb.ProposerSettingsPayload{Version: proposer.SchemaV2},
			nil,
			&flagOptions{},
		)
		require.Equal(t, uint32(proposer.SchemaV2), merged.Version)
	})
}

func Test_mergeProposerSettings_CreatesDefaultFromGasLimitFlag(t *testing.T) {
	gl := validator.Uint64(12345678)
	merged := mergeProposerSettings(
		&validatorpb.ProposerSettingsPayload{},
		nil,
		&flagOptions{gasLimit: &gl},
	)
	require.NotNil(t, merged.DefaultConfig)
	require.NotNil(t, merged.DefaultConfig.Builder)
	require.Equal(t, false, merged.DefaultConfig.Builder.Enabled)
	require.Equal(t, gl, merged.DefaultConfig.Builder.GasLimit)
}

func Test_mergeProposerSettings_V2GasLimitOnlyGoesToOption(t *testing.T) {
	gl := validator.Uint64(12345678)
	merged := mergeProposerSettings(
		nil,
		&validatorpb.ProposerSettingsPayload{Version: proposer.SchemaV2},
		&flagOptions{gasLimit: &gl},
	)
	require.NotNil(t, merged.DefaultConfig)
	require.IsNil(t, merged.DefaultConfig.Builder)
	require.Equal(t, gl, merged.DefaultConfig.GasLimit)
}

func Test_mergeProposerSettings_VersionGatesBuilderReset(t *testing.T) {
	v1Builder := func() *validatorpb.BuilderConfig {
		return &validatorpb.BuilderConfig{Enabled: true, GasLimit: 40000000, Relays: []string{"r"}}
	}
	t.Run("v1 db without enable-builder drops DB builder", func(t *testing.T) {
		db := &validatorpb.ProposerSettingsPayload{
			Version:       proposer.SchemaV1,
			DefaultConfig: &validatorpb.ProposerOptionPayload{FeeRecipient: "0x", Builder: v1Builder()},
		}
		merged := mergeProposerSettings(nil, db, &flagOptions{})
		require.IsNil(t, merged.DefaultConfig.Builder)
	})
	t.Run("v2 db without enable-builder preserves DB builder", func(t *testing.T) {
		db := &validatorpb.ProposerSettingsPayload{
			Version:       proposer.SchemaV2,
			DefaultConfig: &validatorpb.ProposerOptionPayload{FeeRecipient: "0x", Builder: v1Builder()},
		}
		merged := mergeProposerSettings(nil, db, &flagOptions{})
		require.NotNil(t, merged.DefaultConfig.Builder)
		require.Equal(t, validator.Uint64(40000000), merged.DefaultConfig.Builder.GasLimit)
	})
}
