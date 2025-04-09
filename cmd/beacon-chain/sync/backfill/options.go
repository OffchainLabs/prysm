package backfill

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/node"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/sync/backfill"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/sync/backfill/flags"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/urfave/cli/v2"
)

// BeaconNodeOptions sets the appropriate functional opts on the *node.BeaconNode value, to decouple options
// from flag parsing.
func BeaconNodeOptions(c *cli.Context) ([]node.Option, error) {
	opt := func(node *node.BeaconNode) (err error) {
		bno := []backfill.ServiceOption{
			backfill.WithBatchSize(c.Uint64(flags.BackfillBatchSize.Name)),
			backfill.WithWorkerCount(c.Int(flags.BackfillWorkerCount.Name)),
			backfill.WithEnableBackfill(c.Bool(flags.EnableExperimentalBackfill.Name)),
		}
		// The zero value of this uint flag would be genesis, so we use IsSet to differentiate nil from zero case.
		if c.IsSet(flags.BackfillOldestSlot.Name) {
			uv := c.Uint64(flags.BackfillOldestSlot.Name)
			bno = append(bno, backfill.WithMinimumSlot(primitives.Slot(uv)))
		}
		node.BackfillOpts = bno
		return nil
	}
	return []node.Option{opt}, nil
}
