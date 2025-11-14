package general

import (
	"path"
	"testing"

	kzgPrysm "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ghodss/yaml"
)

func TestComputeCellsAndKzgProofs(t *testing.T) {
	type input struct {
		Blob string `json:"blob"`
	}

	type data struct {
		Input  input      `json:"input"`
		Output [][]string `json:"output"`
	}
	require.NoError(t, kzgPrysm.Start())
	testFolders, testFolderPath := utils.TestFolders(t, "general", "fulu", "kzg/compute_cells_and_kzg_proofs/kzg-mainnet")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", "general", "fulu", "kzg/compute_cells_and_kzg_proofs/kzg-mainnet")
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

			cells, proofs, err := kzgPrysm.ComputeCellsAndKZGProofs(&b)
			if test.Output != nil {
				require.NoError(t, err)
				var combined [][]string
				csRaw := make([]string, 0, len(cells))
				for _, c := range cells {
					csRaw = append(csRaw, hexutil.Encode(c[:]))
				}
				psRaw := make([]string, 0, len(proofs))
				for _, p := range proofs {
					psRaw = append(psRaw, hexutil.Encode(p[:]))
				}
				combined = append(combined, csRaw)
				combined = append(combined, psRaw)
				require.DeepEqual(t, test.Output, combined)
			} else {
				require.NotNil(t, err)
			}
		})
	}
}
