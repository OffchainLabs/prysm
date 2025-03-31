package peerdas

import (
	"fmt"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// ConstructDataColumnSidecars constructs data column sidecars from a block, blobs and their cell proofs.
// This is a convenience method as blob and cell proofs are common inputs.
func ConstructDataColumnSidecars(block interfaces.SignedBeaconBlock, blobs [][]byte, cellProofs [][]byte) ([]*ethpb.DataColumnSidecar, error) {
	// Check if the block is at least a Fulu block.
	if block.Version() < version.Fulu {
		return nil, nil
	}

	cellsAndProofs, err := constructCellsAndProofs(blobs, cellProofs)
	if err != nil {
		return nil, err
	}

	return DataColumnSidecars(block, cellsAndProofs)
}

func constructCellsAndProofs(blobs [][]byte, cellProofs [][]byte) ([]kzg.CellsAndProofs, error) {
	numColumns := int(params.BeaconConfig().NumberOfColumns)
	if len(blobs)*numColumns != len(cellProofs) {
		return nil, fmt.Errorf("number of blobs and cell proofs do not match: %d * %d != %d", len(blobs), numColumns, len(cellProofs))
	}

	cellsAndProofs := make([]kzg.CellsAndProofs, 0, len(blobs))

	for i, blob := range blobs {
		var b kzg.Blob
		copy(b[:], blob)
		cells, err := kzg.ComputeCells(&b)
		if err != nil {
			return nil, err
		}

		var proofs []kzg.Proof
		for idx := i * numColumns; idx < (i+1)*numColumns; idx++ {
			proofs = append(proofs, kzg.Proof(cellProofs[idx]))
		}
		cellsAndProofs[i] = kzg.CellsAndProofs{
			Cells:  cells,
			Proofs: proofs,
		}
	}

	return cellsAndProofs, nil
}
