package options

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/node"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/das/flags"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

func BeaconNodeOptions(c *cli.Context) ([]node.Option, error) {
	var oldestBackfillSlot *primitives.Slot
	if c.IsSet(flags.BackfillOldestSlot.Name) {
		uv := c.Uint64(flags.BackfillOldestSlot.Name)
		sv := primitives.Slot(uv)
		oldestBackfillSlot = &sv
	}
	blobRetentionEpochs := primitives.Epoch(c.Uint64(flags.BlobRetentionEpochFlag.Name))
	opt := func(n *node.BeaconNode) error {
		n.SyncNeedsWaiter = func() (das.SyncNeeds, error) {
			clock, err := n.ClockWaiter.WaitForClock(c.Context)
			if err != nil {
				return das.SyncNeeds{}, errors.Wrap(err, "sync needs WaitForClock")
			}
			return das.NewSyncNeeds(
				clock.CurrentSlot,
				oldestBackfillSlot,
				blobRetentionEpochs,
			)
		}
		return nil
	}
	return []node.Option{opt}, nil
}
