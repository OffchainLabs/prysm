package genesis

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/node"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/sync/checkpoint"
	"github.com/OffchainLabs/prysm/v6/genesis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var (
	// StatePath defines a flag to start the beacon chain from a give genesis state file.
	StatePath = &cli.PathFlag{
		Name: "genesis-state",
		Usage: "Load a genesis state from ssz file. Testnet genesis files can be found in the " +
			"eth2-clients/eth2-testnets repository on github.",
	}
	BeaconAPIURL = &cli.StringFlag{
		Name: "genesis-beacon-api-url",
		Usage: "URL of a synced beacon node to trust for obtaining genesis state. " +
			"As an additional safety measure, it is strongly recommended to only use this option in conjunction with " +
			"--weak-subjectivity-checkpoint flag",
	}
)

// BeaconNodeOptions handles options for customizing the source of the genesis state.
func BeaconNodeOptions(c *cli.Context) ([]node.Option, error) {
	statePath := c.Path(StatePath.Name)
	if statePath != "" {
		opt := func(node *node.BeaconNode) (err error) {
			provider, err := genesis.NewFileProvider(statePath)
			if err != nil {
				return errors.Wrap(err, "error preparing to initialize genesis db state from local ssz files")
			}
			node.GenesisProviders = append(node.GenesisProviders, provider)
			return nil
		}
		return []node.Option{opt}, nil
	}

	remoteURL := c.String(BeaconAPIURL.Name)
	if remoteURL == "" && c.String(checkpoint.RemoteURL.Name) != "" {
		log.Infof("Using checkpoint sync url %s for value in --%s flag", c.String(checkpoint.RemoteURL.Name), BeaconAPIURL.Name)
		remoteURL = c.String(checkpoint.RemoteURL.Name)
	}
	if remoteURL != "" {
		opt := func(node *node.BeaconNode) error {
			provider, err := genesis.NewAPIProvider(remoteURL)
			if err != nil {
				return errors.Wrap(err, "error constructing beacon node api client for genesis state init")
			}

			node.GenesisProviders = append(node.GenesisProviders, provider)
			return nil
		}
		return []node.Option{opt}, nil
	}

	return nil, nil
}
