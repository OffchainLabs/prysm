package peerdas

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// CanSelfReconstruct returns true if the node can self-reconstruct all the data columns from its custody group count.
func CanSelfReconstruct(custodyGroupCount uint64) bool {
	total := params.BeaconConfig().NumberOfCustodyGroups
	// If total is odd, then we need total / 2 + 1 columns to reconstruct.
	// If total is even, then we need total / 2 columns to reconstruct.
	custodyGroupsNeeded := total/2 + total%2
	return custodyGroupCount >= custodyGroupsNeeded
}

// RecoverCellsAndProofs recovers the cells and proofs from the data column sidecars.
func RecoverCellsAndProofs(
	dataColumnSideCars []*ethpb.DataColumnSidecar,
	blockRoot [fieldparams.RootLength]byte,
) ([]kzg.CellsAndProofs, error) {
	var wg errgroup.Group

	dataColumnSideCarsCount := len(dataColumnSideCars)

	if dataColumnSideCarsCount == 0 {
		return nil, errors.New("no data column sidecars")
	}

	// Check if all columns have the same length.
	blobCount := len(dataColumnSideCars[0].Column)
	for _, sidecar := range dataColumnSideCars {
		length := len(sidecar.Column)

		if length != blobCount {
			return nil, errors.New("columns do not have the same length")
		}
	}

	// Recover cells and compute proofs in parallel.
	recoveredCellsAndProofs := make([]kzg.CellsAndProofs, blobCount)

	for blobIndex := 0; blobIndex < blobCount; blobIndex++ {
		bIndex := blobIndex
		wg.Go(func() error {
			cellsIndices := make([]uint64, 0, dataColumnSideCarsCount)
			cells := make([]kzg.Cell, 0, dataColumnSideCarsCount)

			for _, sidecar := range dataColumnSideCars {
				// Build the cell indices.
				cellsIndices = append(cellsIndices, sidecar.Index)

				// Get the cell.
				column := sidecar.Column
				cell := column[bIndex]

				cells = append(cells, kzg.Cell(cell))
			}

			// Recover the cells and proofs for the corresponding blob
			cellsAndProofs, err := kzg.RecoverCellsAndKZGProofs(cellsIndices, cells)

			if err != nil {
				return errors.Wrapf(err, "recover cells and KZG proofs for blob %d", bIndex)
			}

			recoveredCellsAndProofs[bIndex] = cellsAndProofs
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return nil, err
	}

	return recoveredCellsAndProofs, nil
}

// DataColumnSidecarsForReconstruct is a TEMPORARY function until there is an official specification for it.
// It is scheduled for deletion.
func DataColumnSidecarsForReconstruct(
	blobKzgCommitments [][]byte,
	signedBlockHeader *ethpb.SignedBeaconBlockHeader,
	kzgCommitmentsInclusionProof [][]byte,
	cellsAndProofs []kzg.CellsAndProofs,
) ([]*ethpb.DataColumnSidecar, error) {
	// Each CellsAndProofs corresponds to a Blob
	// So we can get the BlobCount by checking the length of CellsAndProofs
	blobsCount := len(cellsAndProofs)
	if blobsCount == 0 {
		return nil, nil
	}

	// Get the column sidecars.
	sidecars := make([]*ethpb.DataColumnSidecar, 0, fieldparams.NumberOfColumns)
	for columnIndex := range fieldparams.NumberOfColumns {
		column := make([]kzg.Cell, 0, blobsCount)
		kzgProofOfColumn := make([]kzg.Proof, 0, blobsCount)

		for rowIndex := range blobsCount {
			cellsForRow := cellsAndProofs[rowIndex].Cells
			proofsForRow := cellsAndProofs[rowIndex].Proofs

			if len(cellsForRow) != fieldparams.NumberOfColumns {
				return nil, errors.Errorf("cells don't have the expected size: expected %d - actual %d", fieldparams.NumberOfColumns, len(cellsForRow))
			}

			cell := cellsForRow[columnIndex]
			column = append(column, cell)

			if len(proofsForRow) != fieldparams.NumberOfColumns {
				return nil, errors.Errorf("proofs don't have the expected size: expected %d - actual %d", fieldparams.NumberOfColumns, len(proofsForRow))
			}

			kzgProof := proofsForRow[columnIndex]
			kzgProofOfColumn = append(kzgProofOfColumn, kzgProof)
		}

		columnBytes := make([][]byte, 0, blobsCount)
		for i := range column {
			columnBytes = append(columnBytes, column[i][:])
		}

		kzgProofOfColumnBytes := make([][]byte, 0, blobsCount)
		for _, kzgProof := range kzgProofOfColumn {
			copiedProof := kzgProof
			kzgProofOfColumnBytes = append(kzgProofOfColumnBytes, copiedProof[:])
		}

		sidecar := &ethpb.DataColumnSidecar{
			Index:                        uint64(columnIndex),
			Column:                       columnBytes,
			KzgCommitments:               blobKzgCommitments,
			KzgProofs:                    kzgProofOfColumnBytes,
			SignedBlockHeader:            signedBlockHeader,
			KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
		}

		sidecars = append(sidecars, sidecar)
	}

	return sidecars, nil
}
