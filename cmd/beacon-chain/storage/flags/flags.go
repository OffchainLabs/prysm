package flags

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/urfave/cli/v2"
)

var (
	// BlobStoragePathFlag defines a flag to start the beacon chain from a give genesis state file.
	BlobStoragePathFlag = &cli.PathFlag{
		Name:  "blob-path",
		Usage: "Location for blob storage. Default location will be a 'blobs' directory next to the beacon db.",
	}
	BlobRetentionEpochFlag = &cli.Uint64Flag{
		Name:    "blob-retention-epochs",
		Usage:   "Override the default blob retention period (measured in epochs). The node will exit with an error at startup if the value is less than the default of 4096 epochs.",
		Value:   uint64(params.BeaconConfig().MinEpochsForBlobsSidecarsRequest),
		Aliases: []string{"extend-blob-retention-epoch"},
	}
)
