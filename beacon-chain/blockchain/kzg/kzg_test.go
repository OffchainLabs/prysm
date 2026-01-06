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

	t.Run("empty inputs should return true", func(t *testing.T) {
		// Empty slices should be considered valid
		commitmentsBytes := []Bytes48{}
		cellIndices := []uint64{}
		cells := []Cell{}
		proofsBytes := []Bytes48{}

		valid, err := VerifyCellKZGProofBatch(commitmentsBytes, cellIndices, cells, proofsBytes)
		require.NoError(t, err)
		require.Equal(t, true, valid)
	})

	t.Run("mismatched input lengths should fail", func(t *testing.T) {
		randBlob := random.GetRandBlob(123)
		var blob Blob
		copy(blob[:], randBlob[:])

		commitment, err := BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		cells, proofs, err := ComputeCellsAndKZGProofs(&blob)
		require.NoError(t, err)

		// Create mismatched length inputs
		cellIndices := []uint64{0, 1, 2}
		selectedCells := []Cell{cells[0], cells[1], cells[2]}
		commitmentsBytes := make([]Bytes48, 3)
		for i := range commitmentsBytes {
			copy(commitmentsBytes[i][:], commitment[:])
		}

		// Only 2 proofs instead of 3
		proofsBytes := make([]Bytes48, 2)
		copy(proofsBytes[0][:], proofs[0][:])
		copy(proofsBytes[1][:], proofs[1][:])

		valid, err := VerifyCellKZGProofBatch(commitmentsBytes, cellIndices, selectedCells, proofsBytes)
		require.NotNil(t, err)
		require.Equal(t, false, valid)
		require.Equal(t, "input slices must have equal length", err.Error())
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

func TestComputeChunkBounds(t *testing.T) {
	t.Run("evenly divisible items", func(t *testing.T) {
		chunks := computeChunkBounds(100, 4)
		require.Equal(t, 4, len(chunks))
		require.Equal(t, chunkBounds{start: 0, end: 25}, chunks[0])
		require.Equal(t, chunkBounds{start: 25, end: 50}, chunks[1])
		require.Equal(t, chunkBounds{start: 50, end: 75}, chunks[2])
		require.Equal(t, chunkBounds{start: 75, end: 100}, chunks[3])
	})

	t.Run("items with remainder distributed to first chunks", func(t *testing.T) {
		chunks := computeChunkBounds(10, 3)
		require.Equal(t, 3, len(chunks))
		require.Equal(t, chunkBounds{start: 0, end: 4}, chunks[0])  // gets extra item
		require.Equal(t, chunkBounds{start: 4, end: 7}, chunks[1])  // gets extra item
		require.Equal(t, chunkBounds{start: 7, end: 10}, chunks[2]) // normal size
	})

	t.Run("fewer items than workers returns min(items, workers) chunks", func(t *testing.T) {
		chunks := computeChunkBounds(3, 5)
		require.Equal(t, 3, len(chunks)) // Only 3 chunks, not 5
		require.Equal(t, chunkBounds{start: 0, end: 1}, chunks[0])
		require.Equal(t, chunkBounds{start: 1, end: 2}, chunks[1])
		require.Equal(t, chunkBounds{start: 2, end: 3}, chunks[2])
	})

	t.Run("single worker gets all items", func(t *testing.T) {
		chunks := computeChunkBounds(100, 1)
		require.Equal(t, 1, len(chunks))
		require.Equal(t, chunkBounds{start: 0, end: 100}, chunks[0])
	})

	t.Run("no items produces no chunks", func(t *testing.T) {
		chunks := computeChunkBounds(0, 4)
		require.Equal(t, 0, len(chunks)) // No chunks when no items
	})
}
