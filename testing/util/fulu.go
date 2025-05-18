package util

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

func GenerateCellsAndProofs(t testing.TB, blobs []kzg.Blob) []kzg.CellsAndProofs {
	cellsAndProofs := make([]kzg.CellsAndProofs, len(blobs))
	var wg errgroup.Group

	for i, blob := range blobs {
		wg.Go(func() error {
			cp, err := kzg.ComputeCellsAndKZGProofs(&blob)
			if err != nil {
				return errors.Wrapf(err, "compute cells and kzg proofs for blob %d", i)
			}

			cellsAndProofs[i] = cp
			return nil
		})
	}

	err := wg.Wait()
	require.NoError(t, err)

	return cellsAndProofs
}
