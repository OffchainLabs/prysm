package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/node"
	"github.com/OffchainLabs/prysm/v7/cmd"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var (
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
		Name:        "blob-storage-layout",
		Usage:       layoutFlagUsage(),
		DefaultText: fmt.Sprintf("\"%s\", unless a different existing layout is detected", filesystem.LayoutNameByEpoch),
	}
	DataColumnStoragePathFlag = &cli.PathFlag{
		Name:  "data-column-path",
		Usage: "Location for data column storage. Default location will be a 'data-columns' directory next to the beacon db.",
	}
)

// Flags is the list of CLI flags for configuring blob storage.
var Flags = []cli.Flag{
	BlobStoragePathFlag,
	BlobRetentionEpochFlag,
	BlobStorageLayout,
	DataColumnStoragePathFlag,
}

func layoutOptions() string {
	return "available options are: " + strings.Join(filesystem.LayoutNames, ", ") + "."
}

func layoutFlagUsage() string {
	return "Dictates how to organize the blob directory structure on disk, " + layoutOptions()
}

func validateLayoutFlag(_ *cli.Context, v string) error {
	if slices.Contains(filesystem.LayoutNames, v) {
		return nil
	}
	return errors.Errorf("invalid value '%s' for flag --%s, %s", v, BlobStorageLayout.Name, layoutOptions())
}

// BeaconNodeOptions sets configuration values on the node.BeaconNode value at node startup.
// Note: we can't get the right context from cli.Context, because the beacon node setup code uses this context to
// create a cancellable context. If we switch to using App.RunContext, we can set up this cancellation in the cmd
// package instead, and allow the functional options to tap into context cancellation.
func BeaconNodeOptions(c *cli.Context) ([]node.Option, error) {
	blobRetentionEpoch, err := blobRetentionEpoch(c)
	if err != nil {
		return nil, errors.Wrap(err, "blob retention epoch")
	}

	blobPath := blobStoragePath(c)
	layout, err := detectLayout(blobPath, c)
	if err != nil {
		return nil, errors.Wrap(err, "detecting blob storage layout")
	}
	if layout == filesystem.LayoutNameFlat {
		log.Warnf("Existing '%s' blob storage layout detected. Consider setting the flag --%s=%s for faster startup and more reliable pruning. Setting this flag will automatically migrate your existing blob storage to the newer layout on the next restart.",

			filesystem.LayoutNameFlat, BlobStorageLayout.Name, filesystem.LayoutNameByEpoch)
	}
	blobStorageOptions := node.WithBlobStorageOptions(
		filesystem.WithBlobRetentionEpochs(blobRetentionEpoch),
		filesystem.WithBasePath(blobPath),
		filesystem.WithLayout(layout), // This is validated in the Action func for BlobStorageLayout.
	)

	dataColumnRetentionEpoch, err := dataColumnRetentionEpoch(c)
	if err != nil {
		return nil, errors.Wrap(err, "data column retention epoch")
	}

	dataColumnStorageOption := node.WithDataColumnStorageOptions(
		filesystem.WithDataColumnRetentionEpochs(dataColumnRetentionEpoch),
		filesystem.WithDataColumnBasePath(dataColumnStoragePath(c)),
	)

	opts := []node.Option{blobStorageOptions, dataColumnStorageOption}
	return opts, nil
}

// stringFlagGetter makes testing detectLayout easier
// because we don't need to mess with FlagSets and cli types.
type stringFlagGetter interface {
	String(name string) string
}

// detectLayout determines which layout to use based on explicit user flags or by probing the
// blob directory to determine the previously used layout.
// - explicit: If the user has specified a layout flag, that layout is returned.
// - flat: If directories that look like flat layout's block root paths are present.
// - by-epoch: default if neither of the above is true.
func detectLayout(dir string, c stringFlagGetter) (string, error) {
	explicit := c.String(BlobStorageLayout.Name)
	if explicit != "" {
		return explicit, nil
	}

	dir = filepath.Clean(dir)
	// nosec: this path is provided by the node operator via flag
	base, err := os.Open(dir) // #nosec G304
	if err != nil {
		// 'blobs' directory does not exist yet, so default to by-epoch.
		return filesystem.LayoutNameByEpoch, nil
	}
	defer func() {
		if err := base.Close(); err != nil {
			log.WithError(err).Errorf("Could not close blob storage directory")
		}
	}()

	// When we go looking for existing by-root directories, we only need to find one directory
	// but one of those directories could be the `by-epoch` layout's top-level directory,
	// and it seems possible that on some platforms we could get extra system directories that I don't
	// know how to anticipate (looking at you, Windows), so I picked 16 as a small number with a generous
	// amount of wiggle room to be confident that we'll likely see a by-root director if one exists.
	entries, err := base.Readdirnames(16)
	if err != nil {
		// We can get this error if the directory exists and is empty
		if errors.Is(err, io.EOF) {
			return filesystem.LayoutNameByEpoch, nil
		}
		return "", errors.Wrap(err, "reading blob storage directory")
	}
	for _, entry := range entries {
		if filesystem.IsBlockRootDir(entry) {
			return filesystem.LayoutNameFlat, nil
		}
	}
	return filesystem.LayoutNameByEpoch, nil
}

func blobStoragePath(c *cli.Context) string {
	blobsPath := c.Path(BlobStoragePathFlag.Name)
	if blobsPath == "" {
		// append a "blobs" subdir to the end of the data dir path
		blobsPath = filepath.Join(c.String(cmd.DataDirFlag.Name), "blobs")
	}
	return blobsPath
}

func dataColumnStoragePath(c *cli.Context) string {
	dataColumnsPath := c.Path(DataColumnStoragePathFlag.Name)
	if dataColumnsPath == "" {
		// append a "data-columns" subdir to the end of the data dir path
		dataColumnsPath = filepath.Join(c.String(cmd.DataDirFlag.Name), "data-columns")
	}

	return dataColumnsPath
}

var errInvalidBlobRetentionEpochs = errors.New("value is smaller than spec minimum")

// blobRetentionEpoch returns the spec default MIN_EPOCHS_FOR_BLOB_SIDECARS_REQUEST
// or a user-specified flag overriding this value. If a user-specified override is
// smaller than the spec default, an error will be returned.
func blobRetentionEpoch(cliCtx *cli.Context) (primitives.Epoch, error) {
	spec := params.BeaconConfig().MinEpochsForBlobsSidecarsRequest
	if !cliCtx.IsSet(BlobRetentionEpochFlag.Name) {
		return spec, nil
	}

	re := primitives.Epoch(cliCtx.Uint64(BlobRetentionEpochFlag.Name))
	// Validate the epoch value against the spec default.
	if re < params.BeaconConfig().MinEpochsForBlobsSidecarsRequest {
		return spec, errors.Wrapf(errInvalidBlobRetentionEpochs, "%s=%d, spec=%d", BlobRetentionEpochFlag.Name, re, spec)
	}

	return re, nil
}

// dataColumnRetentionEpoch returns the spec default MIN_EPOCHS_FOR_DATA_COLUMN_SIDECARS_REQUEST
// or a user-specified flag overriding this value. If a user-specified override is
// smaller than the spec default, an error will be returned.
func dataColumnRetentionEpoch(cliCtx *cli.Context) (primitives.Epoch, error) {
	defaultValue := params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest
	if !cliCtx.IsSet(BlobRetentionEpochFlag.Name) {
		return defaultValue, nil
	}

	// We use on purpose the same retention flag for both blobs and data columns.
	customValue := primitives.Epoch(cliCtx.Uint64(BlobRetentionEpochFlag.Name))

	// Validate the epoch value against the spec default.
	if customValue < defaultValue {
		return defaultValue, errors.Wrapf(errInvalidBlobRetentionEpochs, "%s=%d, spec=%d", BlobRetentionEpochFlag.Name, customValue, defaultValue)
	}

	return customValue, nil
}

func init() {
	BlobStorageLayout.Action = validateLayoutFlag
}
