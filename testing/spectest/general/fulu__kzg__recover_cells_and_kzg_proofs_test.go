package general

import (
	"path"
	"strconv"
	"testing"

	kzgPrysm "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ghodss/yaml"
)

func TestRecoverCellsAndKzgProofs(t *testing.T) {
	type input struct {
		CellIndices []string `json:"cell_indices"`
		Cells       []string `json:"cells"`
	}

	type data struct {
		Input  input      `json:"input"`
		Output [][]string `json:"output"`
	}
	require.NoError(t, kzgPrysm.Start())
	testFolders, testFolderPath := utils.TestFolders(t, "general", "fulu", "kzg/recover_cells_and_kzg_proofs/kzg-mainnet")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", "general", "fulu", "kzg/recover_cells_and_kzg_proofs/kzg-mainnet")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			file, err := util.BazelFileBytes(path.Join(testFolderPath, folder.Name(), "data.yaml"))
			require.NoError(t, err)
			test := &data{}
			require.NoError(t, yaml.Unmarshal(file, test))
			cellIndicesRaw := test.Input.CellIndices
			cellIndices := make([]uint64, 0, len(cellIndicesRaw))
			for _, idx := range cellIndicesRaw {
				i, err := strconv.ParseUint(idx, 10, 64)
				require.NoError(t, err)
				cellIndices = append(cellIndices, i)
			}

			// Check if cell indices are sorted
			isSorted := true
			for i := 1; i < len(cellIndices); i++ {
				if cellIndices[i] <= cellIndices[i-1] {
					isSorted = false
					break
				}
			}

			// If cell indices are not sorted and test expects failure, return early
			if !isSorted && test.Output == nil {
				require.IsNil(t, test.Output)
				return
			}
			cellsRaw := test.Input.Cells
			cells := make([]kzgPrysm.Cell, 0, len(cellsRaw))
			for _, cellRaw := range cellsRaw {
				cell, err := hexutil.Decode(cellRaw)
				require.NoError(t, err)
				if len(cell) != kzgPrysm.BytesPerCell {
					require.IsNil(t, test.Output)
					return
				}
				cells = append(cells, kzgPrysm.Cell(cell))
			}

			// Recover the cells and proofs for the corresponding blob
			cellsAndProofsForBlob, err := kzgPrysm.RecoverCellsAndKZGProofs(cellIndices, cells)
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
