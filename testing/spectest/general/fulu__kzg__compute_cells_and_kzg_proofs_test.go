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

			cellsAndProofsForBlob, err := kzgPrysm.ComputeCellsAndKZGProofs(&b)
			if test.Output != nil {
				require.NoError(t, err)
				var combined [][]string
				cs := cellsAndProofsForBlob.Cells
				csRaw := make([]string, 0, len(cs))
				for _, c := range cs {
					csRaw = append(csRaw, hexutil.Encode(c[:]))
				}
				ps := cellsAndProofsForBlob.Proofs
				psRaw := make([]string, 0, len(ps))
				for _, p := range ps {
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
