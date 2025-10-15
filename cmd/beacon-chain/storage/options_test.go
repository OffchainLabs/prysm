package storage

import (
	"flag"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/urfave/cli/v2"
)

func TestBlobStoragePath_NoFlagSpecified(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.DataDirFlag.Name, cmd.DataDirFlag.Value, cmd.DataDirFlag.Usage)
	cliCtx := cli.NewContext(&app, set, nil)
	storagePath := blobStoragePath(cliCtx)

	assert.Equal(t, cmd.DefaultDataDir()+"/blobs", storagePath)
}

func TestBlobStoragePath_FlagSpecified(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(BlobStoragePathFlag.Name, "/blah/blah", BlobStoragePathFlag.Usage)
	cliCtx := cli.NewContext(&app, set, nil)
	storagePath := blobStoragePath(cliCtx)

	assert.Equal(t, "/blah/blah", storagePath)
}

func TestConfigureBlobRetentionEpoch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	specMinEpochs := params.BeaconConfig().MinEpochsForBlobsSidecarsRequest
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	cliCtx := cli.NewContext(&app, set, nil)

	// Test case: Spec default.
	epochs, err := blobRetentionEpoch(cliCtx)
	require.NoError(t, err)
	require.Equal(t, specMinEpochs, epochs)

	// manually define the flag in the set, so the following code can use set.Set
	set.Uint64(BlobRetentionEpochFlag.Name, 0, "")

	// Test case: Input epoch is greater than or equal to spec value.
	expectedChange := specMinEpochs + 1
	require.NoError(t, set.Set(BlobRetentionEpochFlag.Name, fmt.Sprintf("%d", expectedChange)))
	epochs, err = blobRetentionEpoch(cliCtx)
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(expectedChange), epochs)

	// Test case: Input epoch is less than spec value.
	expectedChange = specMinEpochs - 1
	require.NoError(t, set.Set(BlobRetentionEpochFlag.Name, fmt.Sprintf("%d", expectedChange)))
	_, err = blobRetentionEpoch(cliCtx)
	require.ErrorIs(t, err, errInvalidBlobRetentionEpochs)
}

func TestConfigureDataColumnRetentionEpoch(t *testing.T) {
	specValue := params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	cliCtx := cli.NewContext(&app, set, nil)

	// Test case: Specification value
	expected := specValue

	actual, err := dataColumnRetentionEpoch(cliCtx)
	require.NoError(t, err)
	require.Equal(t, expected, actual)

	// Manually define the flag in the set, so the following code can use set.Set
	set.Uint64(BlobRetentionEpochFlag.Name, 0, "")

	// Test case: Input epoch is greater than or equal to specification value.
	expected = specValue + 1

	err = set.Set(BlobRetentionEpochFlag.Name, fmt.Sprintf("%d", expected))
	require.NoError(t, err)

	actual, err = dataColumnRetentionEpoch(cliCtx)
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(expected), actual)

	// Test case: Input epoch is less than specification value.
	expected = specValue - 1

	err = set.Set(BlobRetentionEpochFlag.Name, fmt.Sprintf("%d", expected))
	require.NoError(t, err)

	actual, err = dataColumnRetentionEpoch(cliCtx)
	require.ErrorIs(t, err, errInvalidBlobRetentionEpochs)
	require.Equal(t, specValue, actual)
}

func TestDataColumnStoragePath_FlagSpecified(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(DataColumnStoragePathFlag.Name, "/blah/blah", DataColumnStoragePathFlag.Usage)
	cliCtx := cli.NewContext(&app, set, nil)
	storagePath := dataColumnStoragePath(cliCtx)

	assert.Equal(t, "/blah/blah", storagePath)
}
