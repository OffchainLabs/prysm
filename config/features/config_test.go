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

func TestConfigureTestnet(t *testing.T) {
	tests := []struct {
		name        string
		flags       map[string]interface{}
		expectedLog string
		wantErr     bool
	}{
		{
			name: "Using new network flag - mainnet",
			flags: map[string]interface{}{
				NetworkFlag.Name: "mainnet",
			},
			expectedLog: "Running on Ethereum Mainnet",
			wantErr:     false,
		},
		{
			name: "Using new network flag - sepolia",
			flags: map[string]interface{}{
				NetworkFlag.Name: "sepolia",
			},
			expectedLog: "Running on the Sepolia Beacon Chain Testnet",
			wantErr:     false,
		},
		{
			name: "Using new network flag - holesky",
			flags: map[string]interface{}{
				NetworkFlag.Name: "holesky",
			},
			expectedLog: "Running on the Holesky Beacon Chain Testnet",
			wantErr:     false,
		},
		{
			name: "Using legacy sepolia flag",
			flags: map[string]interface{}{
				SepoliaTestnet.Name: true,
			},
			expectedLog: "Running on the Sepolia Beacon Chain Testnet",
			wantErr:     false,
		},
		{
			name: "Using legacy holesky flag",
			flags: map[string]interface{}{
				HoleskyTestnet.Name: true,
			},
			expectedLog: "Running on the Holesky Beacon Chain Testnet",
			wantErr:     false,
		},
		{
			name: "Using legacy mainnet flag explicitly",
			flags: map[string]interface{}{
				Mainnet.Name: true,
			},
			expectedLog: "Running on Ethereum Mainnet",
			wantErr:     false,
		},
		{
			name:        "Default to mainnet when no flags set",
			flags:       map[string]interface{}{},
			expectedLog: "Running on Ethereum Mainnet",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := cli.App{}
			set := flag.NewFlagSet("test", 0)
			// Add the flags needed for test
			set.String(NetworkFlag.Name, "", "")
			set.Bool(Mainnet.Name, false, "")
			set.Bool(SepoliaTestnet.Name, false, "")
			set.Bool(HoleskyTestnet.Name, false, "")

			// Set the flag values based on the test case
			for name, value := range tt.flags {
				switch v := value.(type) {
				case bool:
					require.NoError(t, set.Set(name, "true"))
				case string:
					require.NoError(t, set.Set(name, v))
				}
			}

			context := cli.NewContext(&app, set, nil)
			err := configureTestnet(context)

			if !tt.wantErr {
				require.NoError(t, err)
			}
		})
	}
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
			name:    "One legacy network flag",
			args:    []string{"command", "--sepolia"},
			wantErr: false,
		},
		{
			name:    "Two legacy network flags",
			args:    []string{"command", "--sepolia", "--holesky"},
			wantErr: true,
		},
		{
			name:    "All legacy network flags",
			args:    []string{"command", "--sepolia", "--holesky", "--mainnet"},
			wantErr: true,
		},
		{
			name:    "New network flag with valid value (mainnet)",
			args:    []string{"command", "--network=mainnet"},
			wantErr: false,
		},
		{
			name:    "New network flag with valid value (sepolia)",
			args:    []string{"command", "--network=sepolia"},
			wantErr: false,
		},
		{
			name:    "New network flag with valid value (holesky)",
			args:    []string{"command", "--network=holesky"},
			wantErr: false,
		},
		{
			name:    "New network flag with invalid value",
			args:    []string{"command", "--network=invalid-network"},
			wantErr: true,
		},
		{
			name:    "Both new and legacy network flags (matching)",
			args:    []string{"command", "--network=sepolia", "--sepolia"},
			wantErr: true,
		},
		{
			name:    "Both new and legacy network flags (different)",
			args:    []string{"command", "--network=mainnet", "--sepolia"},
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
