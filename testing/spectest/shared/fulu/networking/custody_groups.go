package networking

import (
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"gopkg.in/yaml.v3"
)

// RunCustodyGroupsTest executes custody groups spec tests.
func RunCustodyGroupsTest(t *testing.T, config string) {
	type configuration struct {
		NodeId            *big.Int `yaml:"node_id"`
		CustodyGroupCount uint64   `yaml:"custody_group_count"`
		Expected          []uint64 `yaml:"result"`
	}

	err := utils.SetConfig(t, config)
	require.NoError(t, err, "failed to set config")

	// Retrieve the test vector folders.
	testFolders, testsFolderPath := utils.TestFolders(t, config, "fulu", "networking/get_custody_groups/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("no test folders found for %s", testsFolderPath)
	}

	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			var (
				config        configuration
				nodeIdBytes32 [32]byte
			)

			// Load the test vector.
			file, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "meta.yaml")
			require.NoError(t, err, "failed to retrieve the `meta.yaml` YAML file")

			// Unmarshal the test vector.
			err = yaml.Unmarshal(file, &config)
			require.NoError(t, err, "failed to unmarshal the YAML file")

			// Get the node ID.
			nodeIdBytes := make([]byte, 32)
			config.NodeId.FillBytes(nodeIdBytes)
			copy(nodeIdBytes32[:], nodeIdBytes)
			nodeId := enode.ID(nodeIdBytes32)

			// Compute the custody groups.
			actual, err := peerdas.CustodyGroups(nodeId, config.CustodyGroupCount)
			require.NoError(t, err, "failed to compute the custody groups")

			// Compare the results.
			require.Equal(t, len(config.Expected), len(actual))

			for i := range config.Expected {
				require.Equal(t, config.Expected[i], actual[i], "at position %d", i)
			}
		})
	}
}

// RunComputeColumnsForCustodyGroupTest executes compute columns for custody group spec tests.
func RunComputeColumnsForCustodyGroupTest(t *testing.T, config string) {
	type configuration struct {
		CustodyGroup uint64   `yaml:"custody_group"`
		Expected     []uint64 `yaml:"result"`
	}

	err := utils.SetConfig(t, config)
	require.NoError(t, err, "failed to set config")

	// Retrieve the test vector folders.
	testFolders, testsFolderPath := utils.TestFolders(t, config, "fulu", "networking/compute_columns_for_custody_group/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("no test folders found for %s", testsFolderPath)
	}

	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			var config configuration

			// Load the test vector.
			file, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "meta.yaml")
			require.NoError(t, err, "failed to retrieve the `meta.yaml` YAML file")

			// Unmarshal the test vector.
			err = yaml.Unmarshal(file, &config)
			require.NoError(t, err, "failed to unmarshal the YAML file")

			// Compute the custody columns.
			actual, err := peerdas.ComputeColumnsForCustodyGroup(config.CustodyGroup)
			require.NoError(t, err, "failed to compute the custody columns")

			// Compare the results.
			require.Equal(t, len(config.Expected), len(actual), "expected %d custody columns, got %d", len(config.Expected), len(actual))

			for i := range config.Expected {
				require.Equal(t, config.Expected[i], actual[i], "expected column at index %i differs from actual column", i)
			}
		})
	}
}
