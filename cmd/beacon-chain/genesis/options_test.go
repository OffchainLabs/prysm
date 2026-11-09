package genesis

import (
	"flag"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/node"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/genesis"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/urfave/cli/v2"
)

func TestBeaconNodeOptions(t *testing.T) {
	tests := []struct {
		name         string
		beaconAPIURL string
		wantProvider genesis.Provider
	}{
		{
			name:         "ephemery downloads the genesis state",
			wantProvider: &genesis.URLProvider{},
		},
		{
			name:         "an explicit beacon api url wins over ephemery",
			beaconAPIURL: "http://localhost:3500",
			wantProvider: &genesis.APIProvider{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := flag.NewFlagSet("test", 0)
			set.Bool(features.EphemeryTestnet.Name, true, "test")
			set.String(BeaconAPIURL.Name, tt.beaconAPIURL, "test")
			opts, err := BeaconNodeOptions(cli.NewContext(&cli.App{}, set, nil))
			require.NoError(t, err)
			require.Equal(t, 1, len(opts))

			bn := &node.BeaconNode{}
			require.NoError(t, opts[0](bn))
			require.Equal(t, 1, len(bn.GenesisProviders))
			require.Equal(t, fmt.Sprintf("%T", tt.wantProvider), fmt.Sprintf("%T", bn.GenesisProviders[0]))
		})
	}
}
