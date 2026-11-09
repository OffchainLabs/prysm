package backfill

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/node"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync/backfill"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/sync/backfill/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/urfave/cli/v2"
)

// BeaconNodeOptions sets the appropriate functional opts on the *node.BeaconNode value, to decouple options
// from flag parsing.
func BeaconNodeOptions(c *cli.Context) ([]node.Option, error) {
	opt := func(node *node.BeaconNode) (err error) {
		// Archive mode is meaningless without the blocks backfill downloads.
		enabled := c.Bool(flags.EnableExperimentalBackfill.Name) || features.Get().EnableArchive
		bno := []backfill.ServiceOption{
			backfill.WithBatchSize(c.Uint64(flags.BackfillBatchSize.Name)),
			backfill.WithWorkerCount(c.Int(flags.BackfillWorkerCount.Name)),
			backfill.WithEnableBackfill(enabled),
		}
		node.BackfillOpts = bno
		return nil
	}
	return []node.Option{opt}, nil
}
