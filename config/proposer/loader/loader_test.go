package loader

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/cmd/validator/flags"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/validator"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/db/iface"
	dbTest "github.com/prysmaticlabs/prysm/v5/validator/db/testing"
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
				return db.SaveProposerSettings(context.Background(), settings)
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
				return db.SaveProposerSettings(context.Background(), settings)
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
				return db.SaveProposerSettings(context.Background(), settings)
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
				return db.SaveProposerSettings(context.Background(), settings)
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
				return db.SaveProposerSettings(context.Background(), settings)
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
				return db.SaveProposerSettings(context.Background(), settings)
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
				validatorDB := dbTest.SetupDB(t, [][fieldparams.BLSPubkeyLength]byte{}, isSlashingProtectionMinimal)
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
			validatorDB := dbTest.SetupDB(t, [][fieldparams.BLSPubkeyLength]byte{}, isSlashingProtectionMinimal)
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
