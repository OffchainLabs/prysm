package util

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func GenerateCellsAndProofs(t *testing.T, blobs []kzg.Blob) []kzg.CellsAndProofs {
	cellsAndProofs := make([]kzg.CellsAndProofs, len(blobs))
	for i := range blobs {
		cp, err := kzg.ComputeCellsAndKZGProofs(&blobs[i])
		require.NoError(t, err)
		cellsAndProofs[i] = cp
	}
	return cellsAndProofs
}
