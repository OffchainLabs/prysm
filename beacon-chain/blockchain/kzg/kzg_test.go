package kzg

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/crypto/random"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestComputeCells(t *testing.T) {
	require.NoError(t, Start())

	t.Run("valid blob", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		cells, err := ComputeCells(&blob)
		require.NoError(t, err)
		require.Equal(t, 128, len(cells))
	})
}

func TestComputeBlobKZGProof(t *testing.T) {
	require.NoError(t, Start())

	t.Run("valid blob and commitment", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		commitment, err := BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		proof, err := ComputeBlobKZGProof(&blob, commitment)
		require.NoError(t, err)
		require.Equal(t, BytesPerProof, len(proof))
		require.NotEqual(t, Proof{}, proof, "proof should not be empty")
	})
}

func TestComputeCellsAndKZGProofs(t *testing.T) {
	require.NoError(t, Start())

	t.Run("valid blob returns matching cells and proofs", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		cells, proofs, err := ComputeCellsAndKZGProofs(&blob)
		require.NoError(t, err)
		require.Equal(t, 128, len(cells))
		require.Equal(t, 128, len(proofs))
		require.Equal(t, len(cells), len(proofs), "cells and proofs should have matching lengths")
	})
}

func TestVerifyCellKZGProofBatch(t *testing.T) {
	require.NoError(t, Start())

	t.Run("valid proof batch", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		commitment, err := BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		cells, proofs, err := ComputeCellsAndKZGProofs(&blob)
		require.NoError(t, err)

		// Verify a subset of cells
		cellIndices := []uint64{0, 1, 2, 3, 4}
		selectedCells := make([]Cell, len(cellIndices))
		commitmentsBytes := make([]Bytes48, len(cellIndices))
		proofsBytes := make([]Bytes48, len(cellIndices))

		for i, idx := range cellIndices {
			selectedCells[i] = cells[idx]
			copy(commitmentsBytes[i][:], commitment[:])
			copy(proofsBytes[i][:], proofs[idx][:])
		}

		valid, err := VerifyCellKZGProofBatch(commitmentsBytes, cellIndices, selectedCells, proofsBytes)
		require.NoError(t, err)
		require.Equal(t, true, valid)
	})

	t.Run("invalid proof should fail", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		commitment, err := BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		cells, _, err := ComputeCellsAndKZGProofs(&blob)
		require.NoError(t, err)

		// Use invalid proofs
		cellIndices := []uint64{0}
		selectedCells := []Cell{cells[0]}
		commitmentsBytes := make([]Bytes48, 1)
		copy(commitmentsBytes[0][:], commitment[:])

		// Create an invalid proof
		invalidProof := Bytes48{}
		proofsBytes := []Bytes48{invalidProof}

		valid, err := VerifyCellKZGProofBatch(commitmentsBytes, cellIndices, selectedCells, proofsBytes)
		require.NotNil(t, err)
		require.Equal(t, false, valid)
	})
}

func TestRecoverCells(t *testing.T) {
	require.NoError(t, Start())

	t.Run("recover from partial cells", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		cells, err := ComputeCells(&blob)
		require.NoError(t, err)

		// Use half of the cells
		partialIndices := make([]uint64, 64)
		partialCells := make([]Cell, 64)
		for i := range 64 {
			partialIndices[i] = uint64(i)
			partialCells[i] = cells[i]
		}

		recoveredCells, err := RecoverCells(partialIndices, partialCells)
		require.NoError(t, err)
		require.Equal(t, 128, len(recoveredCells))

		// Verify recovered cells match original
		for i := range cells {
			require.Equal(t, cells[i], recoveredCells[i])
		}
	})

	t.Run("insufficient cells should fail", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		cells, err := ComputeCells(&blob)
		require.NoError(t, err)

		// Use only 32 cells (less than 50% required)
		partialIndices := make([]uint64, 32)
		partialCells := make([]Cell, 32)
		for i := range 32 {
			partialIndices[i] = uint64(i)
			partialCells[i] = cells[i]
		}

		_, err = RecoverCells(partialIndices, partialCells)
		require.NotNil(t, err)
	})
}

func TestRecoverCellsAndKZGProofs(t *testing.T) {
	require.NoError(t, Start())

	t.Run("recover cells and proofs from partial cells", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		cells, proofs, err := ComputeCellsAndKZGProofs(&blob)
		require.NoError(t, err)

		// Use half of the cells
		partialIndices := make([]uint64, 64)
		partialCells := make([]Cell, 64)
		for i := range 64 {
			partialIndices[i] = uint64(i)
			partialCells[i] = cells[i]
		}

		recoveredCells, recoveredProofs, err := RecoverCellsAndKZGProofs(partialIndices, partialCells)
		require.NoError(t, err)
		require.Equal(t, 128, len(recoveredCells))
		require.Equal(t, 128, len(recoveredProofs))
		require.Equal(t, len(recoveredCells), len(recoveredProofs), "recovered cells and proofs should have matching lengths")

		// Verify recovered cells match original
		for i := range cells {
			require.Equal(t, cells[i], recoveredCells[i])
			require.Equal(t, proofs[i], recoveredProofs[i])
		}
	})

	t.Run("insufficient cells should fail", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		cells, err := ComputeCells(&blob)
		require.NoError(t, err)

		// Use only 32 cells (less than 50% required)
		partialIndices := make([]uint64, 32)
		partialCells := make([]Cell, 32)
		for i := range 32 {
			partialIndices[i] = uint64(i)
			partialCells[i] = cells[i]
		}

		_, _, err = RecoverCellsAndKZGProofs(partialIndices, partialCells)
		require.NotNil(t, err)
	})
}

func TestBlobToKZGCommitment(t *testing.T) {
	require.NoError(t, Start())

	t.Run("valid blob", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		commitment, err := BlobToKZGCommitment(&blob)
		require.NoError(t, err)
		require.Equal(t, 48, len(commitment))

		// Verify commitment is deterministic
		commitment2, err := BlobToKZGCommitment(&blob)
		require.NoError(t, err)
		require.Equal(t, commitment, commitment2)
	})
}
