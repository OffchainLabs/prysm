package loader

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/urfave/cli/v2"

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
			name: "unversioned file with v2 builder fields is inferred as v2",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir: "./testdata/good-v2-proposer-config-unversioned.json",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				u64 := func(v uint64) *validator.Uint64 { u := validator.Uint64(v); return &u }
				return &proposer.Settings{
					Version: proposer.SchemaV2,
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &proposer.BuilderConfig{
								MinBid: u64(500000000),
								Builders: []*proposer.BuilderEntry{
									{URL: "https://builder-a.example"},
								},
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
			name: "v2 file with builders list loads at v2 and dedups duplicate builder urls",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir: "./testdata/good-v2-proposer-config.json",
				},
			},
			want: func() *proposer.Settings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				u64 := func(v uint64) *validator.Uint64 { u := validator.Uint64(v); return &u }
				return &proposer.Settings{
					Version: proposer.SchemaV2,
					ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
						bytesutil.ToBytes48(key1): {
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							GasLimit: 40000000,
							BuilderConfig: &proposer.BuilderConfig{
								MinBid: u64(500000000),
								Builders: []*proposer.BuilderEntry{
									{URL: "https://builder-a.example", MaxExecutionPayment: u64(1000000000)},
									{URL: "https://builder-b.example"},
								},
							},
						},
					},
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						GasLimit:      30000000,
						BuilderConfig: &proposer.BuilderConfig{Builders: []*proposer.BuilderEntry{}},
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
			require.Equal(t, false, got.DefaultConfig.BuilderConfig.IsEnabled())
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
	t.Run("v1 content merged into a v2 db coexists; version never regresses", func(t *testing.T) {
		merged := mergeProposerSettings(
			&validatorpb.ProposerSettingsPayload{
				DefaultConfig: &validatorpb.ProposerOptionPayload{
					Builder: &validatorpb.BuilderConfig{Enabled: true, GasLimit: 30000000},
				},
			},
			&validatorpb.ProposerSettingsPayload{Version: proposer.SchemaV2},
			&flagOptions{},
		)
		require.Equal(t, uint32(proposer.SchemaV2), merged.Version)
		// Semantics are fork-keyed: legacy content stays for pre-gloas reads and
		// is stripped by the post-fork cleanup, not by the merge.
		require.NotNil(t, merged.DefaultConfig.Builder)
		require.Equal(t, true, merged.DefaultConfig.Builder.Enabled)
	})
	t.Run("file per-key section replaces the DB's entirely", func(t *testing.T) {
		dbPayload := &validatorpb.ProposerSettingsPayload{
			Version: proposer.SchemaV2,
			ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{
				"0xaa": {FeeRecipient: "0x1111111111111111111111111111111111111111"},
				"0xbb": {FeeRecipient: "0x2222222222222222222222222222222222222222", GasLimit: 45000000},
			},
		}
		filePayload := &validatorpb.ProposerSettingsPayload{
			Version: proposer.SchemaV2,
			ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{
				"0xaa": {FeeRecipient: "0x3333333333333333333333333333333333333333"},
			},
		}
		merged := mergeProposerSettings(filePayload, dbPayload, &flagOptions{})
		require.Equal(t, 1, len(merged.ProposerConfig))
		require.Equal(t, "0x3333333333333333333333333333333333333333", merged.ProposerConfig["0xaa"].FeeRecipient)
		// Restarting with a file resets DB-resident keys the file does not name.
		require.IsNil(t, merged.ProposerConfig["0xbb"])
	})
	t.Run("db per-key section kept when the file has none", func(t *testing.T) {
		dbPayload := &validatorpb.ProposerSettingsPayload{
			Version: proposer.SchemaV2,
			ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{
				"0xaa": {FeeRecipient: "0x1111111111111111111111111111111111111111"},
			},
		}
		filePayload := &validatorpb.ProposerSettingsPayload{
			Version:       proposer.SchemaV2,
			DefaultConfig: &validatorpb.ProposerOptionPayload{FeeRecipient: "0x4444444444444444444444444444444444444444"},
		}
		merged := mergeProposerSettings(filePayload, dbPayload, &flagOptions{})
		require.Equal(t, 1, len(merged.ProposerConfig))
		require.Equal(t, "0x1111111111111111111111111111111111111111", merged.ProposerConfig["0xaa"].FeeRecipient)
	})
}

// Restarting with the same v1 file after migration persisted v2 to the DB keeps
// the v2 version and promotes the file's content so its gas limits stay readable.
func TestSettingsLoader_V1FileAfterMigratedDB(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 100
	params.OverrideBeaconConfig(cfg)

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(flags.ProposerSettingsFlag.Name, "", "")
	require.NoError(t, set.Set(flags.ProposerSettingsFlag.Name, "./testdata/good-prepare-beacon-proposer-config-multiple.json"))
	cliCtx := cli.NewContext(&app, set, nil)

	validatorDB := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{}, false)
	migrated := &proposer.Settings{
		Version:       proposer.SchemaV2,
		DefaultConfig: &proposer.Option{GasLimit: 40000000},
	}
	require.NoError(t, validatorDB.SaveProposerSettings(cliCtx.Context, migrated))

	hook := logtest.NewGlobal()
	loader, err := NewProposerSettingsLoader(cliCtx, validatorDB, WithBuilderConfig(), WithGasLimit())
	require.NoError(t, err)
	got, err := loader.Load(cliCtx)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.Equal(t, proposer.SchemaV2, got.Version)
	// The v1 file's builder content survives the merge for pre-gloas reads;
	// the post-fork cleanup is what strips it.
	require.NotNil(t, got.DefaultConfig.BuilderConfig)
	assert.LogsDoNotContain(t, hook, "deprecated v1 schema")

	key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err)
	// Pre-gloas reads still resolve the v1 builder gas limit as a fallback.
	require.Equal(t, validator.Uint64(60000000), got.GasLimit(bytesutil.ToBytes48(key1)))

	// The cutover scrubs the v1 content even under the v2 stamp, then no-ops.
	require.Equal(t, true, got.UpgradeToV2())
	require.IsNil(t, got.DefaultConfig.BuilderConfig)
	require.Equal(t, validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit), got.GasLimit(bytesutil.ToBytes48(key1)))
	require.Equal(t, false, got.UpgradeToV2())
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
	require.Equal(t, false, merged.DefaultConfig.Builder.GetEnabled())
	require.Equal(t, gl, merged.DefaultConfig.Builder.GasLimit)
}

func Test_mergeProposerSettings_V2GasLimitIsLegacyContent(t *testing.T) {
	gl := validator.Uint64(12345678)
	merged := mergeProposerSettings(
		nil,
		&validatorpb.ProposerSettingsPayload{Version: proposer.SchemaV2},
		&flagOptions{gasLimit: &gl},
	)
	// The flag writes only legacy builder-level content, so post-fork
	// resolution and the gas limit schedule are never overridden by it.
	require.NotNil(t, merged.DefaultConfig)
	require.Equal(t, validator.Uint64(0), merged.DefaultConfig.GasLimit)
	require.NotNil(t, merged.DefaultConfig.Builder)
	require.Equal(t, gl, merged.DefaultConfig.Builder.GasLimit)
}

func Test_mergeProposerSettings_VersionGatesBuilderReset(t *testing.T) {
	v1Builder := func() *validatorpb.BuilderConfig {
		return &validatorpb.BuilderConfig{Enabled: true, GasLimit: 40000000}
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
	t.Run("v2 --enable-builder still forces the legacy toggle and warns", func(t *testing.T) {
		hook := logtest.NewGlobal()
		opts := &flagOptions{builderConfig: &proposer.BuilderConfig{Enabled: true}}
		db := &validatorpb.ProposerSettingsPayload{
			Version:       proposer.SchemaV2,
			DefaultConfig: &validatorpb.ProposerOptionPayload{FeeRecipient: "0x"},
		}
		merged := mergeProposerSettings(nil, db, opts)
		require.NotNil(t, merged.DefaultConfig.Builder)
		require.Equal(t, true, merged.DefaultConfig.Builder.Enabled)
		assert.LogsContain(t, hook, "no effect after the gloas fork")
	})
	t.Run("v1 builder content merged into v2 coexists until the post-fork cleanup", func(t *testing.T) {
		file := &validatorpb.ProposerSettingsPayload{
			DefaultConfig: &validatorpb.ProposerOptionPayload{
				FeeRecipient: "0x",
				Builder:      &validatorpb.BuilderConfig{GasLimit: 30000000},
			},
		}
		db := &validatorpb.ProposerSettingsPayload{Version: proposer.SchemaV2}
		merged := mergeProposerSettings(file, db, &flagOptions{})
		require.NotNil(t, merged.DefaultConfig.Builder)
		require.Equal(t, validator.Uint64(30000000), merged.DefaultConfig.Builder.GasLimit)
	})
}

func Test_mergeProposerSettings_V2LoadedOverridesDB(t *testing.T) {
	t.Run("loaded default and per-proposer config win over db", func(t *testing.T) {
		db := &validatorpb.ProposerSettingsPayload{
			Version:       proposer.SchemaV2,
			DefaultConfig: &validatorpb.ProposerOptionPayload{FeeRecipient: "0xdb", GasLimit: 1},
			ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{
				"0xkey": {FeeRecipient: "0xdbkey", GasLimit: 2},
			},
		}
		loaded := &validatorpb.ProposerSettingsPayload{
			Version:       proposer.SchemaV2,
			DefaultConfig: &validatorpb.ProposerOptionPayload{FeeRecipient: "0xloaded", GasLimit: 3},
			ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{
				"0xkey": {FeeRecipient: "0xloadedkey", GasLimit: 4},
			},
		}
		merged := mergeProposerSettings(loaded, db, &flagOptions{})
		require.Equal(t, "0xloaded", merged.DefaultConfig.FeeRecipient)
		require.Equal(t, validator.Uint64(3), merged.DefaultConfig.GasLimit)
		require.Equal(t, "0xloadedkey", merged.ProposerConfig["0xkey"].FeeRecipient)
		require.Equal(t, validator.Uint64(4), merged.ProposerConfig["0xkey"].GasLimit)
	})
	t.Run("db default and per-proposer config used when loaded is nil", func(t *testing.T) {
		db := &validatorpb.ProposerSettingsPayload{
			Version:       proposer.SchemaV2,
			DefaultConfig: &validatorpb.ProposerOptionPayload{FeeRecipient: "0xdb", GasLimit: 1},
			ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{
				"0xkey": {FeeRecipient: "0xdbkey", GasLimit: 2},
			},
		}
		merged := mergeProposerSettings(nil, db, &flagOptions{})
		require.Equal(t, "0xdb", merged.DefaultConfig.FeeRecipient)
		require.Equal(t, validator.Uint64(1), merged.DefaultConfig.GasLimit)
		require.Equal(t, "0xdbkey", merged.ProposerConfig["0xkey"].FeeRecipient)
		require.Equal(t, validator.Uint64(2), merged.ProposerConfig["0xkey"].GasLimit)
	})
}

func Test_mergeProposerSettings_V2GasLimitNeverOverridesOptions(t *testing.T) {
	gl := validator.Uint64(12345678)
	db := &validatorpb.ProposerSettingsPayload{
		Version:       proposer.SchemaV2,
		DefaultConfig: &validatorpb.ProposerOptionPayload{FeeRecipient: "0xdb", GasLimit: 1},
		ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{
			"0xkey": {FeeRecipient: "0xdbkey", GasLimit: 2},
		},
	}
	merged := mergeProposerSettings(nil, db, &flagOptions{gasLimit: &gl})
	// Explicit v2 option-level values are the operator's; the legacy flag
	// no longer stomps them at any level.
	require.Equal(t, validator.Uint64(1), merged.DefaultConfig.GasLimit)
	require.Equal(t, validator.Uint64(2), merged.ProposerConfig["0xkey"].GasLimit)
	require.Equal(t, gl, merged.DefaultConfig.Builder.GasLimit)
}

func Test_markExplicitEmptyBuilders(t *testing.T) {
	entry := &validatorpb.BuilderEntry{Url: "https://a.example"}
	t.Run("explicit empty list gains the marker", func(t *testing.T) {
		p := &validatorpb.ProposerSettingsPayload{
			DefaultConfig: &validatorpb.ProposerOptionPayload{
				Builder: &validatorpb.BuilderConfig{Builders: []*validatorpb.BuilderEntry{}},
			},
		}
		markExplicitEmptyBuilders(p)
		require.Equal(t, true, p.DefaultConfig.Builder.BuildersSet)
	})
	t.Run("nonempty list gains the marker too", func(t *testing.T) {
		p := &validatorpb.ProposerSettingsPayload{
			ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{
				"0xaa": {Builder: &validatorpb.BuilderConfig{Builders: []*validatorpb.BuilderEntry{entry}}},
			},
		}
		markExplicitEmptyBuilders(p)
		require.Equal(t, true, p.ProposerConfig["0xaa"].Builder.BuildersSet)
	})
	t.Run("absent list stays unmarked", func(t *testing.T) {
		p := &validatorpb.ProposerSettingsPayload{
			DefaultConfig:  &validatorpb.ProposerOptionPayload{Builder: &validatorpb.BuilderConfig{Enabled: true}},
			ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{"0xaa": {}, "0xbb": nil},
		}
		markExplicitEmptyBuilders(p)
		require.Equal(t, false, p.DefaultConfig.Builder.BuildersSet)
	})
}

func Test_inferSchemaVersion(t *testing.T) {
	u64 := func(v uint64) *validator.Uint64 { u := validator.Uint64(v); return &u }
	v2Cases := map[string]*validatorpb.BuilderConfig{
		"builders list":         {Builders: []*validatorpb.BuilderEntry{{Url: "https://a.example"}}},
		"builders set marker":   {BuildersSet: true},
		"min_bid":               {MinBid: u64(1)},
		"builder_boost_factor":  {BuilderBoostFactor: u64(100)},
		"max_execution_payment": {MaxExecutionPayment: u64(0)},
	}
	for name, bc := range v2Cases {
		t.Run("unversioned with "+name+" infers v2", func(t *testing.T) {
			p := &validatorpb.ProposerSettingsPayload{
				DefaultConfig: &validatorpb.ProposerOptionPayload{Builder: bc},
			}
			inferSchemaVersion(p)
			require.Equal(t, uint32(proposer.SchemaV2), p.Version)
		})
	}
	t.Run("per-key v2 content infers v2", func(t *testing.T) {
		p := &validatorpb.ProposerSettingsPayload{
			ProposerConfig: map[string]*validatorpb.ProposerOptionPayload{
				"0xaa": {Builder: &validatorpb.BuilderConfig{MinBid: u64(1)}},
			},
		}
		inferSchemaVersion(p)
		require.Equal(t, uint32(proposer.SchemaV2), p.Version)
	})
	t.Run("pure v1 content stays unversioned", func(t *testing.T) {
		p := &validatorpb.ProposerSettingsPayload{
			DefaultConfig: &validatorpb.ProposerOptionPayload{
				Builder: &validatorpb.BuilderConfig{Enabled: true, GasLimit: 30000000},
			},
		}
		inferSchemaVersion(p)
		require.Equal(t, uint32(proposer.SchemaV1Unset), p.Version)
	})
	t.Run("explicit version is never overridden", func(t *testing.T) {
		p := &validatorpb.ProposerSettingsPayload{
			Version: proposer.SchemaV1,
			DefaultConfig: &validatorpb.ProposerOptionPayload{
				Builder: &validatorpb.BuilderConfig{MinBid: u64(1)},
			},
		}
		inferSchemaVersion(p)
		require.Equal(t, uint32(proposer.SchemaV1), p.Version)
	})
	t.Run("no builder content stays unversioned", func(t *testing.T) {
		p := &validatorpb.ProposerSettingsPayload{
			DefaultConfig: &validatorpb.ProposerOptionPayload{FeeRecipient: "0x"},
		}
		inferSchemaVersion(p)
		require.Equal(t, uint32(proposer.SchemaV1Unset), p.Version)
	})
}
