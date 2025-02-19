package features

import (
	"flag"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
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

func TestConfigureBeaconConfig(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.Bool(saveInvalidBlockTempFlag.Name, true, "test")
	context := cli.NewContext(&app, set, nil)
	require.NoError(t, ConfigureBeaconChain(context))
	c := Get()
	assert.Equal(t, true, c.SaveInvalidBlock)
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
