package storage

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
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

type mockStringFlagGetter struct {
	v string
}

func (m mockStringFlagGetter) String(name string) string {
	return m.v
}

func TestDetectLayout(t *testing.T) {
	fakeRoot := "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	require.Equal(t, true, filesystem.IsBlockRootDir(fakeRoot))
	withFlatRoot := func(t *testing.T, dir string) {
		require.NoError(t, os.MkdirAll(path.Join(dir, fakeRoot), 0o755))
	}
	withByEpoch := func(t *testing.T, dir string) {
		require.NoError(t, os.MkdirAll(path.Join(dir, filesystem.PeriodicEpochBaseDir), 0o755))
	}

	cases := []struct {
		name        string
		expected    string
		expectedErr error
		setup       func(t *testing.T, dir string)
		getter      mockStringFlagGetter
	}{
		{
			name:     "no blobs dir",
			expected: filesystem.LayoutNameByEpoch,
		},
		{
			name:     "blobs dir without root dirs",
			expected: filesystem.LayoutNameByEpoch,
			// empty subdirectory under blobs which doesn't match the block root pattern
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.MkdirAll(path.Join(dir, "some-dir"), 0o755))
			},
		},
		{
			name:     "blobs dir with root dir",
			setup:    withFlatRoot,
			expected: filesystem.LayoutNameFlat,
		},
		{
			name:     "blobs dir with root dir overridden by flag",
			setup:    withFlatRoot,
			expected: filesystem.LayoutNameByEpoch,
			getter:   mockStringFlagGetter{v: filesystem.LayoutNameByEpoch},
		},
		{
			name:     "only has by-epoch dir",
			setup:    withByEpoch,
			expected: filesystem.LayoutNameByEpoch,
		},
		{
			name: "contains by-epoch dir and root dirs",
			setup: func(t *testing.T, dir string) {
				withFlatRoot(t, dir)
				withByEpoch(t, dir)
			},
			expected: filesystem.LayoutNameFlat,
		},
		{
			name: "unreadable dir",
			// It isn't detectLayout's job to detect any errors reading the directory,
			// so it ignores errors from the os.Open call. But we can also get errors
			// from readdirnames, but this is hard to simulate in a test. So in the test
			// write a file in place of the dir, which will succeed in the Open call, but
			// fail when read as a directory. This is why the expected error is syscall.ENOTDIR
			// (syscall error code from using readdirnames syscall on an ordinary file).
			setup: func(t *testing.T, dir string) {
				parent := filepath.Dir(dir)
				require.NoError(t, os.MkdirAll(parent, 0o755))
				require.NoError(t, os.WriteFile(dir, []byte{}, 0o755))
			},
			expectedErr: syscall.ENOTDIR,
		},
		{
			name: "empty blobs dir",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.MkdirAll(dir, 0o755))
			},
			expected: filesystem.LayoutNameByEpoch,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := strings.Replace(t.Name(), " ", "_", -1)
			dir = path.Join(os.TempDir(), dir)
			if tc.setup != nil {
				tc.setup(t, dir)
			}
			if tc.expectedErr != nil {
				t.Log("hi")
			}
			layout, err := detectLayout(dir, tc.getter)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, layout)

			assert.Equal(t, tc.expectedErr, err)
			assert.Equal(t, tc.expected, layout)
		})
	}
}
