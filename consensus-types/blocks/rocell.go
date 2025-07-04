package blocks

import (
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

// ROCell represents a read-only data cell.
type ROCell struct {
	*ethpb.CellSidecar
}

func roCellNilCheck(cell *ethpb.CellSidecar) error {
	// Check if the cell is nil.
	if cell == nil {
		return errNilCell
	}
	//
	if len(cell.TxHash) == 0 {
		return errNilTxHash
	}

	return nil
}

// NewROCell creates a new ROCell
func NewROCell(cell *ethpb.CellSidecar) (ROCell, error) {
	if err := roCellNilCheck(cell); err != nil {
		return ROCell{}, err
	}

	return ROCell{CellSidecar: cell}, nil
}

// VerifiedROCell represents an ROCell that has undergone full verification (eg commitment check).
type VerifiedROCell struct {
	ROCell
}

// NewVerifiedROCell "upgrades" an ROCell to a VerifiedROCell. This method should only be used by the verification package.
func NewVerifiedROCell(roCell ROCell) VerifiedROCell {
	return VerifiedROCell{ROCell: roCell}
}
