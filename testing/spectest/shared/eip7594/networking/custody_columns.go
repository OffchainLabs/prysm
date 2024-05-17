package networking

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"gopkg.in/yaml.v3"
)

type Config struct {
	NodeId             *big.Int `yaml:"node_id"`
	CustodySubnetCount uint64   `yaml:"custody_subnet_count"`
	Expected           []uint64 `yaml:"result"`
}

// RunCustodyColumnsTest executes custody columns spec tests.
func RunCustodyColumnsTest(t *testing.T, config string) {
	err := utils.SetConfig(t, config)
	require.NoError(t, err, "failed to set config")

	// Retrieve the test vector folders.
	testFolders, testsFolderPath := utils.TestFolders(t, config, "eip7594", "networking/get_custody_columns/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("no test folders found for %s", testsFolderPath)
	}

	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			var (
				config        Config
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

			// Compute the custodied columns.
			actual, err := peerdas.CustodyColumns(nodeId, config.CustodySubnetCount)
			require.NoError(t, err, "failed to compute the custody columns")

			// Compare the results.
			require.Equal(t, len(config.Expected), len(actual), "expected %d custody columns, got %d", len(config.Expected), len(actual))

			for _, result := range config.Expected {
				ok := actual[result]
				require.Equal(t, true, ok, "expected column %d to be in custody columns", result)
			}
		})
	}
}
