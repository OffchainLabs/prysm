package peerdas

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// Helper function to unblind data column sidecars from a block and a blobs bundle v2.
func ConstructDataColumnSidecars(block interfaces.SignedBeaconBlock, blobs [][]byte, proofs [][]byte) ([]*ethpb.DataColumnSidecar, error) {
	// Check if the block is at least a Fulu block.
	if block.Version() < version.Fulu {
		return nil, nil
	}

	cellsAndProofs, err := ConstructCellsAndProofs(blobs, proofs)
	if err != nil {
		return nil, err
	}

	return DataColumnSidecars(block, cellsAndProofs)
}

func ConstructCellsAndProofs(blobs [][]byte, cellProofs [][]byte) ([]kzg.CellsAndProofs, error) {
	cellsAndProofs := make([]kzg.CellsAndProofs, 0, len(blobs))
	numColumns := int(params.BeaconConfig().NumberOfColumns)

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
