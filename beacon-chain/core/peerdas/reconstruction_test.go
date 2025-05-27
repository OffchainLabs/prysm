package peerdas_test

import (
	"encoding/binary"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

func TestMinimumColumnsCountToReconstruct(t *testing.T) {
	testCases := []struct {
		name            string
		numberOfColumns uint64
		expected        uint64
	}{
		{
			name:            "numberOfColumns=128",
			numberOfColumns: 128,
			expected:        64,
		},
		{
			name:            "numberOfColumns=129",
			numberOfColumns: 129,
			expected:        65,
		},
		{
			name:            "numberOfColumns=130",
			numberOfColumns: 130,
			expected:        65,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set the total number of columns.
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.NumberOfColumns = tc.numberOfColumns
			params.OverrideBeaconConfig(cfg)

			// Compute the minimum number of columns needed to reconstruct.
			actual := peerdas.MinimumColumnsCountToReconstruct()
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestReconstructDataColumnSidecars(t *testing.T) {
	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	t.Run("empty input", func(t *testing.T) {
		_, err := peerdas.ReconstructDataColumnSidecars(nil)
		require.ErrorIs(t, err, peerdas.ErrNotEnoughDataColumnSidecars)
	})

	t.Run("columns lengths differ", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3)

		// Arbitrarily alter the column with index 3
		verifiedRoSidecars[3].Column = verifiedRoSidecars[3].Column[1:]

		_, err := peerdas.ReconstructDataColumnSidecars(verifiedRoSidecars)
		require.ErrorIs(t, err, peerdas.ErrColumnLengthsDiffer)
	})

	t.Run("roots differ", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3, util.WithParentRoot([fieldparams.RootLength]byte{1}))
		_, _, verifiedRoSidecarsAlter := util.GenerateTestFuluBlockWithSidecars(t, 3, util.WithParentRoot([fieldparams.RootLength]byte{2}))

		// Arbitrarily alter the column with index 3
		verifiedRoSidecars[3] = verifiedRoSidecarsAlter[3]
		_, err := peerdas.ReconstructDataColumnSidecars(verifiedRoSidecars)
		require.ErrorIs(t, err, peerdas.ErrBlockRootMismatch)
	})

	const blobCount = 6
	signedBeaconBlockPb := util.NewBeaconBlockFulu()
	block := signedBeaconBlockPb.Block

	commitments := make([][]byte, 0, blobCount)
	for i := range uint64(blobCount) {
		var commitment [fieldparams.KzgCommitmentSize]byte
		binary.BigEndian.PutUint64(commitment[:], i)
		commitments = append(commitments, commitment[:])
	}

	block.Body.BlobKzgCommitments = commitments

	t.Run("not enough columns to enable reconstruction", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3)

		minimum := peerdas.MinimumColumnsCountToReconstruct()
		_, err := peerdas.ReconstructDataColumnSidecars(verifiedRoSidecars[:minimum-1])
		require.ErrorIs(t, err, peerdas.ErrNotEnoughDataColumnSidecars)
	})

	t.Run("nominal", func(t *testing.T) {
		// Build a full set of verified data column sidecars.
		_, _, inputVerifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3)

		// Arbitrarily keep only the even sicars.
		filteredVerifiedRoSidecars := make([]blocks.VerifiedRODataColumn, 0, len(inputVerifiedRoSidecars)/2)
		for i := 0; i < len(inputVerifiedRoSidecars); i += 2 {
			filteredVerifiedRoSidecars = append(filteredVerifiedRoSidecars, inputVerifiedRoSidecars[i])
		}

		// Reconstruct the data column sidecars.
		reconstructedVerifiedRoSidecars, err := peerdas.ReconstructDataColumnSidecars(filteredVerifiedRoSidecars)
		require.NoError(t, err)

		// Verify that the reconstructed sidecars are equal to the original ones.
		require.DeepSSZEqual(t, inputVerifiedRoSidecars, reconstructedVerifiedRoSidecars)
	})
}

func TestConstructDataColumnSidecars(t *testing.T) {
	const (
		blobCount    = 3
		cellsPerBlob = fieldparams.CellsPerBlob
	)

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	roBlock, _, baseVerifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, blobCount)

	// Extract blobs and proofs from the sidecars.
	blobs := make([][]byte, 0, blobCount)
	cellProofs := make([][]byte, 0, cellsPerBlob)
	for blobIndex := range blobCount {
		blob := make([]byte, 0, cellsPerBlob)
		for columnIndex := range cellsPerBlob {
			cell := baseVerifiedRoSidecars[columnIndex].Column[blobIndex]
			blob = append(blob, cell...)
		}

		blobs = append(blobs, blob)

		for columnIndex := range numberOfColumns {
			cellProof := baseVerifiedRoSidecars[columnIndex].KzgProofs[blobIndex]
			cellProofs = append(cellProofs, cellProof)
		}
	}

	actual, err := peerdas.ConstructDataColumnSidecars(roBlock, blobs, cellProofs)
	require.NoError(t, err)

	// Extract the base verified ro sidecars into sidecars.
	expected := make([]*ethpb.DataColumnSidecar, 0, len(baseVerifiedRoSidecars))
	for _, verifiedRoSidecar := range baseVerifiedRoSidecars {
		expected = append(expected, verifiedRoSidecar.DataColumnSidecar)
	}

	require.DeepSSZEqual(t, expected, actual)
}

func TestReconstructBlobs(t *testing.T) {
	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	var emptyBlock blocks.ROBlock

	t.Run("no index", func(t *testing.T) {
		actual, err := peerdas.ReconstructBlobs(emptyBlock, nil, nil)
		require.NoError(t, err)
		require.IsNil(t, actual)
	})

	t.Run("empty input", func(t *testing.T) {
		_, err := peerdas.ReconstructBlobs(emptyBlock, nil, []int{0})
		require.ErrorIs(t, err, peerdas.ErrNotEnoughDataColumnSidecars)
	})

	t.Run("not sorted", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3)

		// Arbitrarily change the order of the sidecars.
		verifiedRoSidecars[3], verifiedRoSidecars[2] = verifiedRoSidecars[2], verifiedRoSidecars[3]

		_, err := peerdas.ReconstructBlobs(emptyBlock, verifiedRoSidecars, []int{0})
		require.ErrorIs(t, err, peerdas.ErrDataColumnSidecarsNotSortedByIndex)
	})

	t.Run("not enough columns", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3)

		inputSidecars := verifiedRoSidecars[:fieldparams.CellsPerBlob-1]
		_, err := peerdas.ReconstructBlobs(emptyBlock, inputSidecars, []int{0})
		require.ErrorIs(t, err, peerdas.ErrNotEnoughDataColumnSidecars)
	})

	t.Run("index too high", func(t *testing.T) {
		const blobCount = 3

		roBlock, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, blobCount)

		_, err := peerdas.ReconstructBlobs(roBlock, verifiedRoSidecars, []int{1, blobCount})
		require.ErrorIs(t, err, peerdas.ErrBlobIndexTooHigh)
	})

	t.Run("not committed to the same block", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3, util.WithParentRoot([fieldparams.RootLength]byte{1}))
		roBlock, _, _ := util.GenerateTestFuluBlockWithSidecars(t, 3, util.WithParentRoot([fieldparams.RootLength]byte{2}))

		_, err = peerdas.ReconstructBlobs(roBlock, verifiedRoSidecars, []int{0})
		require.ErrorContains(t, peerdas.ErrRootMismatch.Error(), err)
	})

	t.Run("nominal", func(t *testing.T) {
		const blobCount = 3
		numberOfColumns := params.BeaconConfig().NumberOfColumns

		roBlock, roBlobSidecars := util.GenerateTestElectraBlockWithSidecar(t, [fieldparams.RootLength]byte{}, 42, blobCount)

		// Compute cells and proofs from blob sidecars.
		var wg errgroup.Group
		blobs := make([][]byte, blobCount)
		cellsAndProofs := make([]kzg.CellsAndProofs, blobCount)
		for i := range blobCount {
			blob := roBlobSidecars[i].Blob
			blobs[i] = blob

			wg.Go(func() error {
				var kzgBlob kzg.Blob
				count := copy(kzgBlob[:], blob)
				require.Equal(t, len(kzgBlob), count)

				cp, err := kzg.ComputeCellsAndKZGProofs(&kzgBlob)
				if err != nil {
					return errors.Wrapf(err, "compute cells and kzg proofs for blob %d", i)
				}

				// It is safe for multiple goroutines to concurrently write to the same slice,
				// as long as they are writing to different indices, which is the case here.
				cellsAndProofs[i] = cp

				return nil
			})
		}

		err := wg.Wait()
		require.NoError(t, err)

		// Flatten proofs.
		cellProofs := make([][]byte, 0, blobCount*numberOfColumns)
		for _, cp := range cellsAndProofs {
			for _, proof := range cp.Proofs {
				cellProofs = append(cellProofs, proof[:])
			}
		}

		// Construct data column sidecars.
		// It is OK to use the public function `ConstructDataColumnSidecars`, as long as
		// `TestConstructDataColumnSidecars` tests pass.
		dataColumnSidecars, err := peerdas.ConstructDataColumnSidecars(roBlock, blobs, cellProofs)
		require.NoError(t, err)

		// Convert to verified data column sidecars.
		verifiedRoSidecars := make([]blocks.VerifiedRODataColumn, 0, len(dataColumnSidecars))
		for _, dataColumnSidecar := range dataColumnSidecars {
			roSidecar, err := blocks.NewRODataColumn(dataColumnSidecar)
			require.NoError(t, err)

			verifiedRoSidecar := blocks.NewVerifiedRODataColumn(roSidecar)
			verifiedRoSidecars = append(verifiedRoSidecars, verifiedRoSidecar)
		}

		indices := []int{2, 0}

		t.Run("no reconstruction needed", func(t *testing.T) {
			// Reconstruct blobs.
			reconstructedVerifiedRoBlobSidecars, err := peerdas.ReconstructBlobs(roBlock, verifiedRoSidecars, indices)
			require.NoError(t, err)

			// Compare blobs.
			for i, blobIndex := range indices {
				expected := roBlobSidecars[blobIndex]
				actual := reconstructedVerifiedRoBlobSidecars[i].ROBlob

				require.DeepSSZEqual(t, expected, actual)
			}
		})

		t.Run("reconstruction needed", func(t *testing.T) {
			// Arbitrarily keep only the even sidecars.
			filteredSidecars := make([]blocks.VerifiedRODataColumn, 0, len(verifiedRoSidecars)/2)
			for i := 0; i < len(verifiedRoSidecars); i += 2 {
				filteredSidecars = append(filteredSidecars, verifiedRoSidecars[i])
			}

			// Reconstruct blobs.
			reconstructedVerifiedRoBlobSidecars, err := peerdas.ReconstructBlobs(roBlock, filteredSidecars, indices)
			require.NoError(t, err)

			// Compare blobs.
			for i, blobIndex := range indices {
				expected := roBlobSidecars[blobIndex]
				actual := reconstructedVerifiedRoBlobSidecars[i].ROBlob

				require.DeepSSZEqual(t, expected, actual)
			}
		})

	})

}
