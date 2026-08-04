package features

import (
	"flag"
	"testing"

	validatorflags "github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/urfave/cli/v2"
)

func TestInitFeatureConfig(t *testing.T) {
	defer Init(&Flags{})
	cfg := &Flags{
		EnableDoppelGanger: true,
	}
	Init(cfg)
	c := Get()
	assert.Equal(t, true, c.EnableDoppelGanger)
}

func TestInitWithReset(t *testing.T) {
	defer Init(&Flags{})
	Init(&Flags{
		EnableDoppelGanger: true,
	})
	assert.Equal(t, true, Get().EnableDoppelGanger)

	// Overwrite previously set value (value that didn't come by default).
	resetCfg := InitWithReset(&Flags{
		EnableDoppelGanger: false,
	})
	assert.Equal(t, false, Get().EnableDoppelGanger)

	// Reset must get to previously set configuration (not to default config values).
	resetCfg()
	assert.Equal(t, true, Get().EnableDoppelGanger)
}

func TestConfigureValidator_RESTApiEnabledByEndpoint(t *testing.T) {
	defer Init(&Flags{})
	app := cli.App{}

	// Explicitly setting the REST endpoint implies the REST API is enabled,
	// even without --enable-beacon-rest-api.
	set := flag.NewFlagSet("test", 0)
	set.String(validatorflags.BeaconRESTApiProviderFlag.Name, "", "test")
	require.NoError(t, set.Set(validatorflags.BeaconRESTApiProviderFlag.Name, "http://127.0.0.1:3500"))
	context := cli.NewContext(&app, set, nil)
	require.NoError(t, ConfigureValidator(context))
	assert.Equal(t, true, Get().EnableBeaconRESTApi)

	// Not setting the endpoint or the enable flag leaves REST disabled.
	set = flag.NewFlagSet("test", 0)
	set.String(validatorflags.BeaconRESTApiProviderFlag.Name, "http://127.0.0.1:3500", "test")
	context = cli.NewContext(&app, set, nil)
	require.NoError(t, ConfigureValidator(context))
	assert.Equal(t, false, Get().EnableBeaconRESTApi)
}

func TestConfigureBeaconConfig(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.Bool(saveInvalidBlockTempFlag.Name, true, "test")
	context := cli.NewContext(&app, set, nil)
	require.NoError(t, ConfigureBeaconChain(context))
	c := Get()
	assert.Equal(t, true, c.SaveInvalidBlock)
}

func TestConfigureBeaconConfig_DisableProgressiveSSZ(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	ctx := cli.NewContext(&app, set, nil)
	set.Bool(DisableProgressiveSSZ.Name, false, "test")
	require.NoError(t, set.Set(DisableProgressiveSSZ.Name, "true"))
	require.NoError(t, ConfigureBeaconChain(ctx))
	assert.Equal(t, true, Get().DisableProgressiveSSZ)
}

func TestConfigureBeaconConfig_ReorgLatePayloads(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	context := cli.NewContext(&app, set, nil)
	require.NoError(t, ConfigureBeaconChain(context))
	assert.Equal(t, false, Get().ReorgLatePayloads)
	assert.Equal(t, true, reorgLatePayloads.Hidden)

	set = flag.NewFlagSet("test", 0)
	set.Bool(reorgLatePayloads.Name, true, "test")
	context = cli.NewContext(&app, set, nil)
	require.NoError(t, ConfigureBeaconChain(context))
	assert.Equal(t, true, Get().ReorgLatePayloads)
}

func TestValidateNetworkFlags(t *testing.T) {
	// Define the test cases
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "No network flags",
			args:    []string{"command"},
			wantErr: false,
		},
		{
			name:    "One network flag",
			args:    []string{"command", "--sepolia"},
			wantErr: false,
		},
		{
			name:    "Two network flags",
			args:    []string{"command", "--sepolia", "--holesky"},
			wantErr: true,
		},
		{
			name:    "All network flags",
			args:    []string{"command", "--sepolia", "--holesky", "--mainnet"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new CLI app with the ValidateNetworkFlags function as the Before action
			app := &cli.App{
				Before: ValidateNetworkFlags,
				Action: func(c *cli.Context) error {
					return nil
				},
				// Set the network flags for the app
				Flags: NetworkFlags,
			}
			err := app.Run(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNetworkFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_parseBlacklistedRoots(t *testing.T) {
	strings := []string{"0xf98e27558ac9ba27ab7d0d3b97d5742a4a68b5e3d7f33c520eda14c39df6368c",
		"0x31604da144e1047b250ee87c5049774870e4a140e8eb0087f38ebc4584c2e7",
		"0x4fcf04e7e4075962360a91be0c54c1d3f67237aa2ce07d83b0479971af3f7e78",
	}
	hook := test.NewGlobal()
	resCfg := InitWithReset(&Flags{
		BlacklistedRoots: parseBlacklistedRoots(strings),
	})
	defer resCfg()

	expected := [][32]byte{
		{0xf9, 0x8e, 0x27, 0x55, 0x8a, 0xc9, 0xba, 0x27, 0xab, 0x7d, 0x0d, 0x3b, 0x97, 0xd5, 0x74, 0x2a, 0x4a, 0x68, 0xb5, 0xe3, 0xd7, 0xf3, 0x3c, 0x52, 0x0e, 0xda, 0x14, 0xc3, 0x9d, 0xf6, 0x36, 0x8c},
		{0x4f, 0xcf, 0x04, 0xe7, 0xe4, 0x07, 0x59, 0x62, 0x36, 0x0a, 0x91, 0xbe, 0x0c, 0x54, 0xc1, 0xd3, 0xf6, 0x72, 0x37, 0xaa, 0x2c, 0xe0, 0x7d, 0x83, 0xb0, 0x47, 0x99, 0x71, 0xaf, 0x3f, 0x7e, 0x78},
	}
	require.LogsContain(t, hook, "Failed to parse blacklisted root")
	require.LogsContain(t, hook, strings[1])
	require.Equal(t, len(expected), len(Get().BlacklistedRoots))
	for _, root := range expected {
		require.Equal(t, true, BlacklistedBlock(root))
	}
}
