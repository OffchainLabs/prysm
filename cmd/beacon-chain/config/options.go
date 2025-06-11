package config

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/node"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/urfave/cli/v2"
)

var (
	GenesisValidatorsRootFlag = &cli.StringFlag{
		Name:  "genesis-validators-root",
		Usage: "Set genesis validators root in BeaconChainConfig. Expects hex encoded value with 0x prefix",
	}
)

func BeaconNodeOptions(c *cli.Context) ([]node.Option, error) {
	opts := []node.Option{}
	if c.IsSet(GenesisValidatorsRootFlag.Name) {
		var gvr [32]byte
		input := []byte(c.String(GenesisValidatorsRootFlag.Name))
		if err := hexutil.UnmarshalFixedText("GenesisValidatorsRoot", input, gvr[:]); err != nil {
			return nil, err
		}
		opts = append(opts, node.WithConfigOptions(params.WithGenesisValidatorsRoot(gvr)))
	}

	return opts, nil
}
