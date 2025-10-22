package peerdas_test

import (
	"encoding/binary"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	pb "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
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
			actual := peerdas.MinimumColumnCountToReconstruct()
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

		minimum := peerdas.MinimumColumnCountToReconstruct()
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

func TestReconstructBlobs(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().ElectraForkEpoch + 4096*2

	require.NoError(t, kzg.Start())
	var emptyBlock blocks.ROBlock
	fs := util.SlotAtEpoch(t, params.BeaconConfig().FuluForkEpoch)

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

	t.Run("consecutive duplicates", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3)

		// [0, 1, 1, 3, 4, ...]
		verifiedRoSidecars[2] = verifiedRoSidecars[1]

		_, err := peerdas.ReconstructBlobs(emptyBlock, verifiedRoSidecars, []int{0})
		require.ErrorIs(t, err, peerdas.ErrDataColumnSidecarsNotSortedByIndex)
	})

	t.Run("non-consecutive duplicates", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3)

		// [0, 1, 2, 1, 4, ...]
		verifiedRoSidecars[3] = verifiedRoSidecars[1]

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
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3, util.WithParentRoot([fieldparams.RootLength]byte{1}), util.WithSlot(fs))
		roBlock, _, _ := util.GenerateTestFuluBlockWithSidecars(t, 3, util.WithParentRoot([fieldparams.RootLength]byte{2}), util.WithSlot(fs))

		_, err := peerdas.ReconstructBlobs(roBlock, verifiedRoSidecars, []int{0})
		require.ErrorContains(t, peerdas.ErrRootMismatch.Error(), err)
	})

	t.Run("nominal", func(t *testing.T) {
		const blobCount = 3
		numberOfColumns := params.BeaconConfig().NumberOfColumns

		roBlock, roBlobSidecars := util.GenerateTestElectraBlockWithSidecar(t, [fieldparams.RootLength]byte{}, 42, blobCount)

		// Compute cells and proofs from blob sidecars.
		var wg errgroup.Group
		blobs := make([][]byte, blobCount)
		inputCellsAndProofs := make([]kzg.CellsAndProofs, blobCount)
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
				inputCellsAndProofs[i] = cp

				return nil
			})
		}

		err := wg.Wait()
		require.NoError(t, err)

		// Flatten proofs.
		cellProofs := make([][]byte, 0, blobCount*numberOfColumns)
		for _, cp := range inputCellsAndProofs {
			for _, proof := range cp.Proofs {
				cellProofs = append(cellProofs, proof[:])
			}
		}

		// Compute celles and proofs from the blobs and cell proofs.
		cellsAndProofs, err := peerdas.ComputeCellsAndProofsFromFlat(blobs, cellProofs)
		require.NoError(t, err)

		// Construct data column sidears from the signed block and cells and proofs.
		roDataColumnSidecars, err := peerdas.DataColumnSidecars(cellsAndProofs, peerdas.PopulateFromBlock(roBlock))
		require.NoError(t, err)

		// Convert to verified data column sidecars.
		verifiedRoSidecars := make([]blocks.VerifiedRODataColumn, 0, len(roDataColumnSidecars))
		for _, roDataColumnSidecar := range roDataColumnSidecars {
			verifiedRoSidecar := blocks.NewVerifiedRODataColumn(roDataColumnSidecar)
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

func TestComputeCellsAndProofsFromFlat(t *testing.T) {
	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	t.Run("mismatched blob and proof counts", func(t *testing.T) {
		numberOfColumns := params.BeaconConfig().NumberOfColumns

		// Create one blob but proofs for two blobs
		blobs := [][]byte{{}}

		// Create proofs for 2 blobs worth of columns
		cellProofs := make([][]byte, 2*numberOfColumns)

		_, err := peerdas.ComputeCellsAndProofsFromFlat(blobs, cellProofs)
		require.ErrorIs(t, err, peerdas.ErrBlobsCellsProofsMismatch)
	})

	t.Run("nominal", func(t *testing.T) {
		const blobCount = 2
		numberOfColumns := params.BeaconConfig().NumberOfColumns

		// Generate test blobs
		_, roBlobSidecars := util.GenerateTestElectraBlockWithSidecar(t, [fieldparams.RootLength]byte{}, 42, blobCount)

		// Extract blobs and compute expected cells and proofs
		blobs := make([][]byte, blobCount)
		expectedCellsAndProofs := make([]kzg.CellsAndProofs, blobCount)
		var wg errgroup.Group

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

				expectedCellsAndProofs[i] = cp
				return nil
			})
		}

		err := wg.Wait()
		require.NoError(t, err)

		// Flatten proofs
		cellProofs := make([][]byte, 0, blobCount*numberOfColumns)
		for _, cp := range expectedCellsAndProofs {
			for _, proof := range cp.Proofs {
				cellProofs = append(cellProofs, proof[:])
			}
		}

		// Test ComputeCellsAndProofs
		actualCellsAndProofs, err := peerdas.ComputeCellsAndProofsFromFlat(blobs, cellProofs)
		require.NoError(t, err)
		require.Equal(t, blobCount, len(actualCellsAndProofs))

		// Verify the results match expected
		for i := range blobCount {
			require.Equal(t, len(expectedCellsAndProofs[i].Cells), len(actualCellsAndProofs[i].Cells))
			require.Equal(t, len(expectedCellsAndProofs[i].Proofs), len(actualCellsAndProofs[i].Proofs))

			// Compare cells
			for j, expectedCell := range expectedCellsAndProofs[i].Cells {
				require.Equal(t, expectedCell, actualCellsAndProofs[i].Cells[j])
			}

			// Compare proofs
			for j, expectedProof := range expectedCellsAndProofs[i].Proofs {
				require.Equal(t, expectedProof, actualCellsAndProofs[i].Proofs[j])
			}
		}
	})
}

func TestComputeCellsAndProofsFromStructured(t *testing.T) {
	t.Run("nil blob and proof", func(t *testing.T) {
		_, err := peerdas.ComputeCellsAndProofsFromStructured([]*pb.BlobAndProofV2{nil})
		require.ErrorIs(t, err, peerdas.ErrNilBlobAndProof)
	})

	t.Run("nominal", func(t *testing.T) {
		// Start the trusted setup.
		err := kzg.Start()
		require.NoError(t, err)

		const blobCount = 2

		// Generate test blobs
		_, roBlobSidecars := util.GenerateTestElectraBlockWithSidecar(t, [fieldparams.RootLength]byte{}, 42, blobCount)

		// Extract blobs and compute expected cells and proofs
		blobsAndProofs := make([]*pb.BlobAndProofV2, blobCount)
		expectedCellsAndProofs := make([]kzg.CellsAndProofs, blobCount)

		var wg errgroup.Group
		for i := range blobCount {
			blob := roBlobSidecars[i].Blob

			wg.Go(func() error {
				var kzgBlob kzg.Blob
				count := copy(kzgBlob[:], blob)
				require.Equal(t, len(kzgBlob), count)

				cellsAndProofs, err := kzg.ComputeCellsAndKZGProofs(&kzgBlob)
				if err != nil {
					return errors.Wrapf(err, "compute cells and kzg proofs for blob %d", i)
				}
				expectedCellsAndProofs[i] = cellsAndProofs

				kzgProofs := make([][]byte, 0, len(cellsAndProofs.Proofs))
				for _, proof := range cellsAndProofs.Proofs {
					kzgProofs = append(kzgProofs, proof[:])
				}

				blobAndProof := &pb.BlobAndProofV2{
					Blob:      blob,
					KzgProofs: kzgProofs,
				}
				blobsAndProofs[i] = blobAndProof

				return nil
			})
		}

		err = wg.Wait()
		require.NoError(t, err)

		// Test ComputeCellsAndProofs
		actualCellsAndProofs, err := peerdas.ComputeCellsAndProofsFromStructured(blobsAndProofs)
		require.NoError(t, err)
		require.Equal(t, blobCount, len(actualCellsAndProofs))

		// Verify the results match expected
		for i := range blobCount {
			require.Equal(t, len(expectedCellsAndProofs[i].Cells), len(actualCellsAndProofs[i].Cells))
			require.Equal(t, len(expectedCellsAndProofs[i].Proofs), len(actualCellsAndProofs[i].Proofs))
			require.Equal(t, len(expectedCellsAndProofs[i].Proofs), cap(actualCellsAndProofs[i].Proofs))

			// Compare cells
			for j, expectedCell := range expectedCellsAndProofs[i].Cells {
				require.Equal(t, expectedCell, actualCellsAndProofs[i].Cells[j])
			}

			// Compare proofs
			for j, expectedProof := range expectedCellsAndProofs[i].Proofs {
				require.Equal(t, expectedProof, actualCellsAndProofs[i].Proofs[j])
			}
		}
	})
}
