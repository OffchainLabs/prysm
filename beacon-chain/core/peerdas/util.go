package peerdas

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
)

// ConstructDataColumnSidecars constructs data column sidecars from a block, blobs and their cell proofs.
// This is a convenience method as blob and cell proofs are common inputs.
func ConstructDataColumnSidecars(block interfaces.SignedBeaconBlock, blobs [][]byte, cellProofs [][]byte) ([]*ethpb.DataColumnSidecar, error) {
	// Check if the block is at least a Fulu block.
	if block.Version() < version.Fulu {
		return nil, nil
	}

	numberOfColumns := params.BeaconConfig().NumberOfColumns
	if uint64(len(blobs))*numberOfColumns != uint64(len(cellProofs)) {
		return nil, fmt.Errorf("number of blobs and cell proofs do not match: %d * %d != %d", len(blobs), numberOfColumns, len(cellProofs))
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
		for idx := uint64(i) * numberOfColumns; idx < (uint64(i)+1)*numberOfColumns; idx++ {
			proofs = append(proofs, kzg.Proof(cellProofs[idx]))
		}

		cellsAndProofs = append(cellsAndProofs, kzg.CellsAndProofs{
			Cells:  cells,
			Proofs: proofs,
		})
	}

	dataColumnSidecars, err := DataColumnSidecars(block, cellsAndProofs)
	if err != nil {
		return nil, errors.Wrap(err, "data column sidcars")
	}

	return dataColumnSidecars, nil
}
