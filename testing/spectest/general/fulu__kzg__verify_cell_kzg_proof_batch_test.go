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

func TestVerifyCellKZGProofBatch(t *testing.T) {
	type input struct {
		Commitments []string `json:"commitments"`
		CellIndices []string `json:"cell_indices"`
		Cells       []string `json:"cells"`
		Proofs      []string `json:"proofs"`
	}

	type data struct {
		Input  input `json:"input"`
		Output bool  `json:"output"`
	}
	require.NoError(t, kzgPrysm.Start())
	testFolders, testFolderPath := utils.TestFolders(t, "general", "fulu", "kzg/verify_cell_kzg_proof_batch/kzg-mainnet")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", "general", "fulu", "kzg/verify_cell_kzg_proof_batch/kzg-mainnet")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			file, err := util.BazelFileBytes(path.Join(testFolderPath, folder.Name(), "data.yaml"))
			require.NoError(t, err)
			test := &data{}
			require.NoError(t, yaml.Unmarshal(file, test))

			commitmentsRaw := test.Input.Commitments
			commitments := make([]kzgPrysm.Bytes48, 0, len(commitmentsRaw))
			for _, commitmentRaw := range commitmentsRaw {
				commitment, err := hexutil.Decode(commitmentRaw)
				require.NoError(t, err)
				if len(commitment) != 48 {
					require.Equal(t, false, test.Output)
					return
				}
				commitments = append(commitments, kzgPrysm.Bytes48(commitment))
			}
			cellIndicesRaw := test.Input.CellIndices
			cellIndices := make([]uint64, 0, len(cellIndicesRaw))
			for _, idx := range cellIndicesRaw {
				i, err := strconv.ParseUint(idx, 10, 64)
				require.NoError(t, err)
				cellIndices = append(cellIndices, i)
			}
			cellsRaw := test.Input.Cells
			cells := make([]kzgPrysm.Cell, 0, len(cellsRaw))
			for _, cellRaw := range cellsRaw {
				cell, err := hexutil.Decode(cellRaw)
				require.NoError(t, err)
				if len(cell) != kzgPrysm.BytesPerCell {
					require.Equal(t, false, test.Output)
					return
				}
				cells = append(cells, kzgPrysm.Cell(cell))
			}
			proofsRaw := test.Input.Proofs
			proofs := make([]kzgPrysm.Bytes48, 0, len(proofsRaw))
			for _, proofRaw := range proofsRaw {
				proof, err := hexutil.Decode(proofRaw)
				require.NoError(t, err)
				if len(proof) != 48 {
					require.Equal(t, false, test.Output)
					return
				}
				proofs = append(proofs, kzgPrysm.Bytes48(proof))
			}
			ok, err := kzgPrysm.VerifyCellKZGProofBatch(commitments, cellIndices, cells, proofs)
			if test.Output {
				require.Equal(t, true, ok)
				require.NoError(t, err)
			} else {
				require.Equal(t, false, ok)
			}
		})
	}
}
