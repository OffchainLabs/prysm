package peerdas_test

import (
	"encoding/binary"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

func TestMinimumColumnsCountToReconstruct(t *testing.T) {
	const expected = uint64(64)
	actual := peerdas.MinimumColumnCountToReconstruct()
	require.Equal(t, expected, actual)
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

func TestReconstructBlobSidecars(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().ElectraForkEpoch + 4096*2

	require.NoError(t, kzg.Start())
	var emptyBlock blocks.ROBlock
	fs := util.SlotAtEpoch(t, params.BeaconConfig().FuluForkEpoch)

	t.Run("no index", func(t *testing.T) {
		actual, err := peerdas.ReconstructBlobSidecars(emptyBlock, nil, nil)
		require.NoError(t, err)
		require.IsNil(t, actual)
	})

	t.Run("empty input", func(t *testing.T) {
		_, err := peerdas.ReconstructBlobSidecars(emptyBlock, nil, []int{0})
		require.ErrorIs(t, err, peerdas.ErrNotEnoughDataColumnSidecars)
	})

	t.Run("not sorted", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3)

		// Arbitrarily change the order of the sidecars.
		verifiedRoSidecars[3], verifiedRoSidecars[2] = verifiedRoSidecars[2], verifiedRoSidecars[3]

		_, err := peerdas.ReconstructBlobSidecars(emptyBlock, verifiedRoSidecars, []int{0})
		require.ErrorIs(t, err, peerdas.ErrDataColumnSidecarsNotSortedByIndex)
	})

	t.Run("consecutive duplicates", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3)

		// [0, 1, 1, 3, 4, ...]
		verifiedRoSidecars[2] = verifiedRoSidecars[1]

		_, err := peerdas.ReconstructBlobSidecars(emptyBlock, verifiedRoSidecars, []int{0})
		require.ErrorIs(t, err, peerdas.ErrDataColumnSidecarsNotSortedByIndex)
	})

	t.Run("non-consecutive duplicates", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3)

		// [0, 1, 2, 1, 4, ...]
		verifiedRoSidecars[3] = verifiedRoSidecars[1]

		_, err := peerdas.ReconstructBlobSidecars(emptyBlock, verifiedRoSidecars, []int{0})
		require.ErrorIs(t, err, peerdas.ErrDataColumnSidecarsNotSortedByIndex)
	})

	t.Run("not enough columns", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3)

		inputSidecars := verifiedRoSidecars[:fieldparams.CellsPerBlob-1]
		_, err := peerdas.ReconstructBlobSidecars(emptyBlock, inputSidecars, []int{0})
		require.ErrorIs(t, err, peerdas.ErrNotEnoughDataColumnSidecars)
	})

	t.Run("index too high", func(t *testing.T) {
		const blobCount = 3

		roBlock, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, blobCount)

		_, err := peerdas.ReconstructBlobSidecars(roBlock, verifiedRoSidecars, []int{1, blobCount})
		require.ErrorIs(t, err, peerdas.ErrBlobIndexTooHigh)
	})

	t.Run("not committed to the same block", func(t *testing.T) {
		_, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3, util.WithParentRoot([fieldparams.RootLength]byte{1}), util.WithSlot(fs))
		roBlock, _, _ := util.GenerateTestFuluBlockWithSidecars(t, 3, util.WithParentRoot([fieldparams.RootLength]byte{2}), util.WithSlot(fs))

		_, err := peerdas.ReconstructBlobSidecars(roBlock, verifiedRoSidecars, []int{0})
		require.ErrorContains(t, peerdas.ErrRootMismatch.Error(), err)
	})

	t.Run("nominal", func(t *testing.T) {
		const blobCount = 3

		roBlock, roBlobSidecars := util.GenerateTestElectraBlockWithSidecar(t, [fieldparams.RootLength]byte{}, 42, blobCount)

		// Compute cells and proofs from blob sidecars.
		var wg errgroup.Group
		blobs := make([][]byte, blobCount)
		inputCellsPerBlob := make([][]kzg.Cell, blobCount)
		inputProofsPerBlob := make([][]kzg.Proof, blobCount)
		for i := range blobCount {
			blob := roBlobSidecars[i].Blob
			blobs[i] = blob

			wg.Go(func() error {
				var kzgBlob kzg.Blob
				count := copy(kzgBlob[:], blob)
				require.Equal(t, len(kzgBlob), count)

				cells, proofs, err := kzg.ComputeCellsAndKZGProofs(&kzgBlob)
				if err != nil {
					return errors.Wrapf(err, "compute cells and kzg proofs for blob %d", i)
				}

				// It is safe for multiple goroutines to concurrently write to the same slice,
				// as long as they are writing to different indices, which is the case here.
				inputCellsPerBlob[i] = cells
				inputProofsPerBlob[i] = proofs

				return nil
			})
		}

		err := wg.Wait()
		require.NoError(t, err)

		// Flatten proofs.
		cellProofs := make([][]byte, 0, blobCount*fieldparams.NumberOfColumns)
		for _, proofs := range inputProofsPerBlob {
			for _, proof := range proofs {
				cellProofs = append(cellProofs, proof[:])
			}
		}

		// Compute celles and proofs from the blobs and cell proofs.
		cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(blobs, cellProofs)
		require.NoError(t, err)

		// Construct data column sidears from the signed block and cells and proofs.
		roDataColumnSidecars, err := peerdas.DataColumnSidecars(cellsPerBlob, proofsPerBlob, peerdas.PopulateFromBlock(roBlock))
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
			reconstructedVerifiedRoBlobSidecars, err := peerdas.ReconstructBlobSidecars(roBlock, verifiedRoSidecars, indices)
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
			reconstructedVerifiedRoBlobSidecars, err := peerdas.ReconstructBlobSidecars(roBlock, filteredSidecars, indices)
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

func TestReconstructBlobs(t *testing.T) {
	setupFuluForkEpoch(t)
	require.NoError(t, kzg.Start())

	t.Run("empty indices with blobCount > 0", func(t *testing.T) {
		setup := setupTestBlobs(t, 3)

		// Call with empty indices - should return all blobs
		reconstructedBlobs, err := peerdas.ReconstructBlobs(setup.verifiedRoDataColumnSidecars, []int{}, setup.blobCount)
		require.NoError(t, err)
		require.Equal(t, setup.blobCount, len(reconstructedBlobs))

		// Verify each blob matches
		for i := 0; i < setup.blobCount; i++ {
			require.DeepEqual(t, setup.blobs[i][:], reconstructedBlobs[i])
		}
	})

	t.Run("specific indices", func(t *testing.T) {
		setup := setupTestBlobs(t, 3)

		// Request only blobs at indices 0 and 2
		indices := []int{0, 2}
		reconstructedBlobs, err := peerdas.ReconstructBlobs(setup.verifiedRoDataColumnSidecars, indices, setup.blobCount)
		require.NoError(t, err)
		require.Equal(t, len(indices), len(reconstructedBlobs))

		// Verify requested blobs match
		for i, blobIndex := range indices {
			require.DeepEqual(t, setup.blobs[blobIndex][:], reconstructedBlobs[i])
		}
	})

	t.Run("blob count mismatch", func(t *testing.T) {
		setup := setupTestBlobs(t, 3)

		// Pass wrong blob count
		wrongBlobCount := 5
		_, err := peerdas.ReconstructBlobs(setup.verifiedRoDataColumnSidecars, []int{0}, wrongBlobCount)
		require.ErrorContains(t, "blob count mismatch", err)
	})

	t.Run("empty data columns", func(t *testing.T) {
		_, err := peerdas.ReconstructBlobs([]blocks.VerifiedRODataColumn{}, []int{0}, 1)
		require.ErrorIs(t, err, peerdas.ErrNotEnoughDataColumnSidecars)
	})

	t.Run("index too high", func(t *testing.T) {
		setup := setupTestBlobs(t, 3)

		// Request blob index that's too high
		_, err := peerdas.ReconstructBlobs(setup.verifiedRoDataColumnSidecars, []int{setup.blobCount}, setup.blobCount)
		require.ErrorIs(t, err, peerdas.ErrBlobIndexTooHigh)
	})

	t.Run("not enough columns", func(t *testing.T) {
		setup := setupTestBlobs(t, 3)

		// Only provide 63 columns (need at least 64)
		inputSidecars := setup.verifiedRoDataColumnSidecars[:fieldparams.CellsPerBlob-1]
		_, err := peerdas.ReconstructBlobs(inputSidecars, []int{0}, setup.blobCount)
		require.ErrorIs(t, err, peerdas.ErrNotEnoughDataColumnSidecars)
	})

	t.Run("not sorted", func(t *testing.T) {
		setup := setupTestBlobs(t, 3)

		// Swap two sidecars to make them unsorted
		setup.verifiedRoDataColumnSidecars[3], setup.verifiedRoDataColumnSidecars[2] = setup.verifiedRoDataColumnSidecars[2], setup.verifiedRoDataColumnSidecars[3]

		_, err := peerdas.ReconstructBlobs(setup.verifiedRoDataColumnSidecars, []int{0}, setup.blobCount)
		require.ErrorIs(t, err, peerdas.ErrDataColumnSidecarsNotSortedByIndex)
	})

	t.Run("with reconstruction needed", func(t *testing.T) {
		setup := setupTestBlobs(t, 3)

		// Keep only even-indexed columns (will need reconstruction)
		filteredSidecars := filterEvenIndexedSidecars(setup.verifiedRoDataColumnSidecars)

		// Reconstruct all blobs
		reconstructedBlobs, err := peerdas.ReconstructBlobs(filteredSidecars, []int{}, setup.blobCount)
		require.NoError(t, err)
		require.Equal(t, setup.blobCount, len(reconstructedBlobs))

		// Verify all blobs match
		for i := range setup.blobCount {
			require.DeepEqual(t, setup.blobs[i][:], reconstructedBlobs[i])
		}
	})

	t.Run("no reconstruction needed - all non-extended columns present", func(t *testing.T) {
		setup := setupTestBlobs(t, 3)

		// Use all columns (no reconstruction needed since we have all non-extended columns 0-63)
		reconstructedBlobs, err := peerdas.ReconstructBlobs(setup.verifiedRoDataColumnSidecars, []int{1}, setup.blobCount)
		require.NoError(t, err)
		require.Equal(t, 1, len(reconstructedBlobs))

		// Verify blob matches
		require.DeepEqual(t, setup.blobs[1][:], reconstructedBlobs[0])
	})

	t.Run("reconstruct only requested blob indices", func(t *testing.T) {
		// This test verifies the optimization: when reconstruction is needed and specific
		// blob indices are requested, we only reconstruct those blobs, not all of them.
		setup := setupTestBlobs(t, 6)

		// Keep only even-indexed columns (will need reconstruction)
		// This ensures we don't have all non-extended columns (0-63)
		filteredSidecars := filterEvenIndexedSidecars(setup.verifiedRoDataColumnSidecars)

		// Request only specific blob indices (not all of them)
		requestedIndices := []int{1, 3, 5}
		reconstructedBlobs, err := peerdas.ReconstructBlobs(filteredSidecars, requestedIndices, setup.blobCount)
		require.NoError(t, err)

		// Should only get the requested blobs back (not all 6)
		require.Equal(t, len(requestedIndices), len(reconstructedBlobs),
			"should only reconstruct requested blobs, not all blobs")

		// Verify each requested blob matches the original
		for i, blobIndex := range requestedIndices {
			require.DeepEqual(t, setup.blobs[blobIndex][:], reconstructedBlobs[i],
				"blob at index %d should match", blobIndex)
		}
	})
}

func TestComputeCellsAndProofsFromFlat(t *testing.T) {
	const numberOfColumns = fieldparams.NumberOfColumns
	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	t.Run("mismatched blob and proof counts", func(t *testing.T) {
		// Create one blob but proofs for two blobs
		blobs := [][]byte{{}}

		// Create proofs for 2 blobs worth of columns
		cellProofs := make([][]byte, 2*numberOfColumns)

		_, _, err := peerdas.ComputeCellsAndProofsFromFlat(blobs, cellProofs)
		require.ErrorIs(t, err, peerdas.ErrBlobsCellsProofsMismatch)
	})

	t.Run("nominal", func(t *testing.T) {
		const blobCount = 2

		// Generate test blobs
		_, roBlobSidecars := util.GenerateTestElectraBlockWithSidecar(t, [fieldparams.RootLength]byte{}, 42, blobCount)

		// Extract blobs and compute expected cells and proofs
		blobs := make([][]byte, blobCount)
		expectedCellsPerBlob := make([][]kzg.Cell, blobCount)
		expectedProofsPerBlob := make([][]kzg.Proof, blobCount)
		var wg errgroup.Group

		for i := range blobCount {
			blob := roBlobSidecars[i].Blob
			blobs[i] = blob

			wg.Go(func() error {
				var kzgBlob kzg.Blob
				count := copy(kzgBlob[:], blob)
				require.Equal(t, len(kzgBlob), count)

				cells, proofs, err := kzg.ComputeCellsAndKZGProofs(&kzgBlob)
				if err != nil {
					return errors.Wrapf(err, "compute cells and kzg proofs for blob %d", i)
				}

				expectedCellsPerBlob[i] = cells
				expectedProofsPerBlob[i] = proofs
				return nil
			})
		}

		err := wg.Wait()
		require.NoError(t, err)

		// Flatten proofs
		cellProofs := make([][]byte, 0, blobCount*numberOfColumns)
		for _, proofs := range expectedProofsPerBlob {
			for _, proof := range proofs {
				cellProofs = append(cellProofs, proof[:])
			}
		}

		// Test ComputeCellsAndProofs
		actualCellsPerBlob, actualProofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(blobs, cellProofs)
		require.NoError(t, err)
		require.Equal(t, blobCount, len(actualCellsPerBlob))

		// Verify the results match expected
		for i := range blobCount {
			require.Equal(t, len(expectedCellsPerBlob[i]), len(actualCellsPerBlob[i]))
			require.Equal(t, len(expectedProofsPerBlob[i]), len(actualProofsPerBlob[i]))

			// Compare cells
			for j, expectedCell := range expectedCellsPerBlob[i] {
				require.Equal(t, expectedCell, actualCellsPerBlob[i][j])
			}

			// Compare proofs
			for j, expectedProof := range expectedProofsPerBlob[i] {
				require.Equal(t, expectedProof, actualProofsPerBlob[i][j])
			}
		}
	})
}

func TestComputeCellsAndProofsFromStructured(t *testing.T) {
	t.Run("nil blob and proof", func(t *testing.T) {
		_, _, err := peerdas.ComputeCellsAndProofsFromStructured([]*pb.BlobAndProofV2{nil})
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
		expectedCellsPerBlob := make([][]kzg.Cell, blobCount)
		expectedProofsPerBlob := make([][]kzg.Proof, blobCount)

		var wg errgroup.Group
		for i := range blobCount {
			blob := roBlobSidecars[i].Blob

			wg.Go(func() error {
				var kzgBlob kzg.Blob
				count := copy(kzgBlob[:], blob)
				require.Equal(t, len(kzgBlob), count)

				cells, proofs, err := kzg.ComputeCellsAndKZGProofs(&kzgBlob)
				if err != nil {
					return errors.Wrapf(err, "compute cells and kzg proofs for blob %d", i)
				}
				expectedCellsPerBlob[i] = cells
				expectedProofsPerBlob[i] = proofs

				kzgProofs := make([][]byte, 0, len(proofs))
				for _, proof := range proofs {
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
		actualCellsPerBlob, actualProofsPerBlob, err := peerdas.ComputeCellsAndProofsFromStructured(blobsAndProofs)
		require.NoError(t, err)
		require.Equal(t, blobCount, len(actualCellsPerBlob))

		// Verify the results match expected
		for i := range blobCount {
			require.Equal(t, len(expectedCellsPerBlob[i]), len(actualCellsPerBlob[i]))
			require.Equal(t, len(expectedProofsPerBlob[i]), len(actualProofsPerBlob[i]))
			require.Equal(t, len(expectedProofsPerBlob[i]), cap(actualProofsPerBlob[i]))

			// Compare cells
			for j, expectedCell := range expectedCellsPerBlob[i] {
				require.Equal(t, expectedCell, actualCellsPerBlob[i][j])
			}

			// Compare proofs
			for j, expectedProof := range expectedProofsPerBlob[i] {
				require.Equal(t, expectedProof, actualProofsPerBlob[i][j])
			}
		}
	})
}
