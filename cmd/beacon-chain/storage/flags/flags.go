package flags

import (
	"strings"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/pkg/errors"
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
	BlobStorageLayout = &cli.StringFlag{
		Name:  "blob-storage-layout",
		Usage: layoutFlagUsage(),
		Value: filesystem.LayoutNameFlat,
	}
)

func layoutOptions() string {
	return "available options are: " + strings.Join(filesystem.LayoutNames, ", ") + "."
}

func layoutFlagUsage() string {
	return "Dictates how to organize the blob directory structure on disk, " + layoutOptions()
}

func validateLayoutFlag(_ *cli.Context, v string) error {
	for _, l := range filesystem.LayoutNames {
		if v == l {
			return nil
		}
	}
	return errors.Errorf("invalid value '%s' for flag --%s, %s", v, BlobStorageLayout.Name, layoutOptions())
}

func init() {
	BlobStorageLayout.Action = validateLayoutFlag
}
