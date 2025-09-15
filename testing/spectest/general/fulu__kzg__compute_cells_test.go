package general

import (
	"path"
	"testing"

	kzgPrysm "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ghodss/yaml"
)

func TestComputeCells(t *testing.T) {
	type input struct {
		Blob string `json:"blob"`
	}

	type data struct {
		Input  input    `json:"input"`
		Output []string `json:"output"`
	}

	require.NoError(t, kzgPrysm.Start())
	testFolders, testFolderPath := utils.TestFolders(t, "general", "fulu", "kzg/compute_cells/kzg-mainnet")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", "general", "fulu", "kzg/compute_cells/kzg-mainnet")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			file, err := util.BazelFileBytes(path.Join(testFolderPath, folder.Name(), "data.yaml"))
			require.NoError(t, err)
			test := &data{}
			require.NoError(t, yaml.Unmarshal(file, test))

			blob, err := hexutil.Decode(test.Input.Blob)
			require.NoError(t, err)
			if len(blob) != fieldparams.BlobLength {
				require.IsNil(t, test.Output)
				return
			}
			b := kzgPrysm.Blob(blob)

			// Recover the cells and proofs for the corresponding blob
			cells, err := kzgPrysm.ComputeCells(&b)
			if test.Output != nil {
				require.NoError(t, err)
				cs := make([]string, 0, len(cells))
				for _, c := range cells {
					cs = append(cs, hexutil.Encode(c[:]))
				}
				require.DeepEqual(t, test.Output, cs)
			} else {
				require.NotNil(t, err)
			}
		})
	}
}
