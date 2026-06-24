package flags

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/urfave/cli/v2"
)

var (
	BackfillOldestSlot = &cli.Uint64Flag{
		Name: "backfill-oldest-slot",
		Usage: "Specifies the oldest slot that backfill should download. " +
			"If this value is greater than current_slot - MIN_EPOCHS_FOR_BLOCK_REQUESTS, it will be ignored with a warning log.",
	}
	BlobRetentionEpochFlag = &cli.Uint64Flag{
		Name:    "blob-retention-epochs",
		Usage:   "Override the default blob retention period (measured in epochs). The node will exit with an error at startup if the value is less than the default of 4096 epochs.",
		Value:   uint64(params.BeaconConfig().MinEpochsForBlobsSidecarsRequest),
		Aliases: []string{"extend-blob-retention-epoch"},
	}
)

var Flags = []cli.Flag{
	BackfillOldestSlot,
	BlobRetentionEpochFlag,
}
