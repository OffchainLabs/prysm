package kzg

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/crypto/random"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
)

func GenerateCommitmentAndProof(blob GoKZG.Blob) (GoKZG.KZGCommitment, GoKZG.KZGProof, error) {
	commitment, err := kzgContext.BlobToKZGCommitment(&blob, 0)
	if err != nil {
		return GoKZG.KZGCommitment{}, GoKZG.KZGProof{}, err
	}
	proof, err := kzgContext.ComputeBlobKZGProof(&blob, commitment, 0)
	if err != nil {
		return GoKZG.KZGCommitment{}, GoKZG.KZGProof{}, err
	}
	return commitment, proof, err
}

func TestVerify(t *testing.T) {
	blobSidecars := make([]blocks.ROBlob, 0)
	require.NoError(t, Verify(blobSidecars...))
}

func TestBytesToAny(t *testing.T) {
	bytes := []byte{0x01, 0x02}
	blob := GoKZG.Blob{0x01, 0x02}
	commitment := GoKZG.KZGCommitment{0x01, 0x02}
	proof := GoKZG.KZGProof{0x01, 0x02}
	require.DeepEqual(t, blob, *bytesToBlob(bytes))
	require.DeepEqual(t, commitment, bytesToCommitment(bytes))
	require.DeepEqual(t, proof, bytesToKZGProof(bytes))
}

func TestGenerateCommitmentAndProof(t *testing.T) {
	require.NoError(t, Start())
	blob := random.GetRandBlob(123)
	commitment, proof, err := GenerateCommitmentAndProof(blob)
	require.NoError(t, err)
	expectedCommitment := GoKZG.KZGCommitment{180, 218, 156, 194, 59, 20, 10, 189, 186, 254, 132, 93, 7, 127, 104, 172, 238, 240, 237, 70, 83, 89, 1, 152, 99, 0, 165, 65, 143, 62, 20, 215, 230, 14, 205, 95, 28, 245, 54, 25, 160, 16, 178, 31, 232, 207, 38, 85}
	expectedProof := GoKZG.KZGProof{128, 110, 116, 170, 56, 111, 126, 87, 229, 234, 211, 42, 110, 150, 129, 206, 73, 142, 167, 243, 90, 149, 240, 240, 236, 204, 143, 182, 229, 249, 81, 27, 153, 171, 83, 70, 144, 250, 42, 1, 188, 215, 71, 235, 30, 7, 175, 86}
	require.Equal(t, expectedCommitment, commitment)
	require.Equal(t, expectedProof, proof)
}

func TestVerifyBlobKZGProofBatch(t *testing.T) {
	// Initialize KZG for testing
	require.NoError(t, Start())

	t.Run("valid single blob batch", func(t *testing.T) {
		blob := random.GetRandBlob(123)
		commitment, proof, err := GenerateCommitmentAndProof(blob)
		require.NoError(t, err)

		blobs := [][]byte{blob[:]}
		commitments := [][]byte{commitment[:]}
		proofs := [][]byte{proof[:]}

		err = VerifyBlobKZGProofBatch(blobs, commitments, proofs)
		require.NoError(t, err)
	})

	t.Run("valid multiple blob batch", func(t *testing.T) {
		blobCount := 3
		blobs := make([][]byte, blobCount)
		commitments := make([][]byte, blobCount)
		proofs := make([][]byte, blobCount)

		for i := 0; i < blobCount; i++ {
			blob := random.GetRandBlob(int64(i))
			commitment, proof, err := GenerateCommitmentAndProof(blob)
			require.NoError(t, err)

			blobs[i] = blob[:]
			commitments[i] = commitment[:]
			proofs[i] = proof[:]
		}

		err := VerifyBlobKZGProofBatch(blobs, commitments, proofs)
		require.NoError(t, err)
	})

	t.Run("empty inputs should pass", func(t *testing.T) {
		err := VerifyBlobKZGProofBatch([][]byte{}, [][]byte{}, [][]byte{})
		require.NoError(t, err)
	})

	t.Run("mismatched input lengths", func(t *testing.T) {
		blob := random.GetRandBlob(123)
		commitment, proof, err := GenerateCommitmentAndProof(blob)
		require.NoError(t, err)

		// Test different mismatch scenarios
		err = VerifyBlobKZGProofBatch(
			[][]byte{blob[:]},
			[][]byte{},
			[][]byte{proof[:]},
		)
		require.ErrorContains(t, "number of blobs (1), commitments (0), and proofs (1) must match", err)

		err = VerifyBlobKZGProofBatch(
			[][]byte{blob[:], blob[:]},
			[][]byte{commitment[:]},
			[][]byte{proof[:], proof[:]},
		)
		require.ErrorContains(t, "number of blobs (2), commitments (1), and proofs (2) must match", err)
	})

	t.Run("invalid commitment should fail", func(t *testing.T) {
		blob := random.GetRandBlob(123)
		_, proof, err := GenerateCommitmentAndProof(blob)
		require.NoError(t, err)

		// Use a different blob's commitment (mismatch)
		differentBlob := random.GetRandBlob(456)
		wrongCommitment, _, err := GenerateCommitmentAndProof(differentBlob)
		require.NoError(t, err)

		blobs := [][]byte{blob[:]}
		commitments := [][]byte{wrongCommitment[:]}
		proofs := [][]byte{proof[:]}

		err = VerifyBlobKZGProofBatch(blobs, commitments, proofs)
		// Single blob optimization uses different error message
		require.ErrorContains(t, "can't verify opening proof", err)
	})

	t.Run("invalid proof should fail", func(t *testing.T) {
		blob := random.GetRandBlob(123)
		commitment, _, err := GenerateCommitmentAndProof(blob)
		require.NoError(t, err)

		// Use wrong proof
		invalidProof := make([]byte, 48) // All zeros

		blobs := [][]byte{blob[:]}
		commitments := [][]byte{commitment[:]}
		proofs := [][]byte{invalidProof}

		err = VerifyBlobKZGProofBatch(blobs, commitments, proofs)
		require.ErrorContains(t, "short buffer", err)
	})

	t.Run("mixed valid and invalid proofs should fail", func(t *testing.T) {
		// First blob - valid
		blob1 := random.GetRandBlob(123)
		commitment1, proof1, err := GenerateCommitmentAndProof(blob1)
		require.NoError(t, err)

		// Second blob - invalid proof
		blob2 := random.GetRandBlob(456)
		commitment2, _, err := GenerateCommitmentAndProof(blob2)
		require.NoError(t, err)
		invalidProof := make([]byte, 48) // All zeros

		blobs := [][]byte{blob1[:], blob2[:]}
		commitments := [][]byte{commitment1[:], commitment2[:]}
		proofs := [][]byte{proof1[:], invalidProof}

		err = VerifyBlobKZGProofBatch(blobs, commitments, proofs)
		require.ErrorContains(t, "batch verification", err)
	})

	t.Run("batch KZG proof verification failed", func(t *testing.T) {
		// Create multiple blobs with mismatched commitments and proofs to trigger batch verification failure
		blob1 := random.GetRandBlob(123)
		blob2 := random.GetRandBlob(456)

		// Generate valid proof for blob1
		commitment1, proof1, err := GenerateCommitmentAndProof(blob1)
		require.NoError(t, err)

		// Generate valid proof for blob2 but use wrong commitment (from blob1)
		_, proof2, err := GenerateCommitmentAndProof(blob2)
		require.NoError(t, err)

		// Use blob2 data with blob1's commitment and blob2's proof - this should cause batch verification to fail
		blobs := [][]byte{blob1[:], blob2[:]}
		commitments := [][]byte{commitment1[:], commitment1[:]} // Wrong commitment for blob2
		proofs := [][]byte{proof1[:], proof2[:]}

		err = VerifyBlobKZGProofBatch(blobs, commitments, proofs)
		require.ErrorContains(t, "batch KZG proof verification failed", err)
	})
}

func TestVerifyCellKZGProofBatchFromBlobData(t *testing.T) {
	// Initialize KZG for testing
	require.NoError(t, Start())

	t.Run("valid single blob cell verification", func(t *testing.T) {
		numberOfColumns := uint64(128)

		// Generate blob and commitment
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])
		commitment, err := BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		// Compute cells and proofs
		cellsAndProofs, err := ComputeCellsAndKZGProofs(&blob)
		require.NoError(t, err)

		// Create flattened cell proofs (like execution client format)
		cellProofs := make([][]byte, numberOfColumns)
		for i := range numberOfColumns {
			cellProofs[i] = cellsAndProofs.Proofs[i][:]
		}

		blobs := [][]byte{blob[:]}
		commitments := [][]byte{commitment[:]}

		err = VerifyCellKZGProofBatchFromBlobData(blobs, commitments, cellProofs, numberOfColumns)
		require.NoError(t, err)
	})

	t.Run("valid multiple blob cell verification", func(t *testing.T) {
		numberOfColumns := uint64(128)
		blobCount := 2

		blobs := make([][]byte, blobCount)
		commitments := make([][]byte, blobCount)
		var allCellProofs [][]byte

		for i := range blobCount {
			// Generate blob and commitment
			randBlob := random.GetRandBlob(int64(i))
			var blob Blob
			copy(blob[:], randBlob[:])
			commitment, err := BlobToKZGCommitment(&blob)
			require.NoError(t, err)

			// Compute cells and proofs
			cellsAndProofs, err := ComputeCellsAndKZGProofs(&blob)
			require.NoError(t, err)

			blobs[i] = blob[:]
			commitments[i] = commitment[:]

			// Add cell proofs for this blob
			for j := range numberOfColumns {
				allCellProofs = append(allCellProofs, cellsAndProofs.Proofs[j][:])
			}
		}

		err := VerifyCellKZGProofBatchFromBlobData(blobs, commitments, allCellProofs, numberOfColumns)
		require.NoError(t, err)
	})

	t.Run("empty inputs should pass", func(t *testing.T) {
		err := VerifyCellKZGProofBatchFromBlobData([][]byte{}, [][]byte{}, [][]byte{}, 128)
		require.NoError(t, err)
	})

	t.Run("mismatched blob and commitment count", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		err := VerifyCellKZGProofBatchFromBlobData(
			[][]byte{blob[:]},
			[][]byte{}, // Empty commitments
			[][]byte{},
			128,
		)
		require.ErrorContains(t, "expected 128 cell proofs", err)
	})

	t.Run("wrong cell proof count", func(t *testing.T) {
		numberOfColumns := uint64(128)

		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])
		commitment, err := BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		blobs := [][]byte{blob[:]}
		commitments := [][]byte{commitment[:]}

		// Wrong number of cell proofs - should be 128 for 1 blob, but provide 10
		wrongCellProofs := make([][]byte, 10)

		err = VerifyCellKZGProofBatchFromBlobData(blobs, commitments, wrongCellProofs, numberOfColumns)
		require.ErrorContains(t, "expected 128 cell proofs, got 10", err)
	})

	t.Run("invalid cell proofs should fail", func(t *testing.T) {
		numberOfColumns := uint64(128)

		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])
		commitment, err := BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		blobs := [][]byte{blob[:]}
		commitments := [][]byte{commitment[:]}

		// Create invalid cell proofs (all zeros)
		invalidCellProofs := make([][]byte, numberOfColumns)
		for i := range numberOfColumns {
			invalidCellProofs[i] = make([]byte, 48) // All zeros
		}

		err = VerifyCellKZGProofBatchFromBlobData(blobs, commitments, invalidCellProofs, numberOfColumns)
		require.ErrorContains(t, "cell batch verification", err)
	})

	t.Run("mismatched commitment should fail", func(t *testing.T) {
		numberOfColumns := uint64(128)

		// Generate blob and correct cell proofs
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])
		cellsAndProofs, err := ComputeCellsAndKZGProofs(&blob)
		require.NoError(t, err)

		// Generate wrong commitment from different blob
		randBlob2 := random.GetRandBlob(456)
		var differentBlob Blob
		copy(differentBlob[:], randBlob2[:])
		wrongCommitment, err := BlobToKZGCommitment(&differentBlob)
		require.NoError(t, err)

		cellProofs := make([][]byte, numberOfColumns)
		for i := range numberOfColumns {
			cellProofs[i] = cellsAndProofs.Proofs[i][:]
		}

		blobs := [][]byte{blob[:]}
		commitments := [][]byte{wrongCommitment[:]}

		err = VerifyCellKZGProofBatchFromBlobData(blobs, commitments, cellProofs, numberOfColumns)
		require.ErrorContains(t, "cell KZG proof batch verification failed", err)
	})

	t.Run("invalid blob data that should cause ComputeCells to fail", func(t *testing.T) {
		numberOfColumns := uint64(128)

		// Create invalid blob (not properly formatted)
		invalidBlobData := make([]byte, 10) // Too short
		commitment := make([]byte, 48)      // Dummy commitment
		cellProofs := make([][]byte, numberOfColumns)
		for i := range numberOfColumns {
			cellProofs[i] = make([]byte, 48)
		}

		blobs := [][]byte{invalidBlobData}
		commitments := [][]byte{commitment}

		err := VerifyCellKZGProofBatchFromBlobData(blobs, commitments, cellProofs, numberOfColumns)
		require.NotNil(t, err)
		require.ErrorContains(t, "blobs len (10) differs from expected (131072)", err)
	})

	t.Run("invalid commitment size should fail", func(t *testing.T) {
		numberOfColumns := uint64(128)

		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		// Create invalid commitment (wrong size)
		invalidCommitment := make([]byte, 32) // Should be 48 bytes
		cellProofs := make([][]byte, numberOfColumns)
		for i := range numberOfColumns {
			cellProofs[i] = make([]byte, 48)
		}

		blobs := [][]byte{blob[:]}
		commitments := [][]byte{invalidCommitment}

		err := VerifyCellKZGProofBatchFromBlobData(blobs, commitments, cellProofs, numberOfColumns)
		require.ErrorContains(t, "commitments len (32) differs from expected (48)", err)
	})

	t.Run("invalid cell proof size should fail", func(t *testing.T) {
		numberOfColumns := uint64(128)

		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])
		commitment, err := BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		// Create invalid cell proofs (wrong size)
		invalidCellProofs := make([][]byte, numberOfColumns)
		for i := range numberOfColumns {
			if i == 0 {
				invalidCellProofs[i] = make([]byte, 32) // Wrong size - should be 48
			} else {
				invalidCellProofs[i] = make([]byte, 48)
			}
		}

		blobs := [][]byte{blob[:]}
		commitments := [][]byte{commitment[:]}

		err = VerifyCellKZGProofBatchFromBlobData(blobs, commitments, invalidCellProofs, numberOfColumns)
		require.ErrorContains(t, "proofs len (32) differs from expected (48)", err)
	})

	t.Run("multiple blobs with mixed invalid commitments", func(t *testing.T) {
		numberOfColumns := uint64(128)
		blobCount := 2

		blobs := make([][]byte, blobCount)
		commitments := make([][]byte, blobCount)
		var allCellProofs [][]byte

		// First blob - valid
		randBlob1 := random.GetRandBlob(123)
		var blob1 Blob
		copy(blob1[:], randBlob1[:])
		commitment1, err := BlobToKZGCommitment(&blob1)
		require.NoError(t, err)
		blobs[0] = blob1[:]
		commitments[0] = commitment1[:]

		// Second blob - use invalid commitment size
		randBlob2 := random.GetRandBlob(456)
		var blob2 Blob
		copy(blob2[:], randBlob2[:])
		blobs[1] = blob2[:]
		commitments[1] = make([]byte, 32) // Wrong size

		// Add cell proofs for both blobs
		for i := 0; i < blobCount; i++ {
			for j := uint64(0); j < numberOfColumns; j++ {
				allCellProofs = append(allCellProofs, make([]byte, 48))
			}
		}

		err = VerifyCellKZGProofBatchFromBlobData(blobs, commitments, allCellProofs, numberOfColumns)
		require.ErrorContains(t, "commitments len (32) differs from expected (48)", err)
	})

	t.Run("multiple blobs with mixed invalid cell proof sizes", func(t *testing.T) {
		numberOfColumns := uint64(128)
		blobCount := 2

		blobs := make([][]byte, blobCount)
		commitments := make([][]byte, blobCount)
		var allCellProofs [][]byte

		for i := 0; i < blobCount; i++ {
			randBlob := random.GetRandBlob(int64(i))
			var blob Blob
			copy(blob[:], randBlob[:])
			commitment, err := BlobToKZGCommitment(&blob)
			require.NoError(t, err)

			blobs[i] = blob[:]
			commitments[i] = commitment[:]

			// Add cell proofs - make some invalid in the second blob
			for j := uint64(0); j < numberOfColumns; j++ {
				if i == 1 && j == 64 {
					// Invalid proof size in middle of second blob's proofs
					allCellProofs = append(allCellProofs, make([]byte, 20))
				} else {
					allCellProofs = append(allCellProofs, make([]byte, 48))
				}
			}
		}

		err := VerifyCellKZGProofBatchFromBlobData(blobs, commitments, allCellProofs, numberOfColumns)
		require.ErrorContains(t, "proofs len (20) differs from expected (48)", err)
	})
}
