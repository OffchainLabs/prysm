package kzg

import (
	"fmt"

	"github.com/ethereum/go-ethereum/crypto/kzg4844"
)

// BlobToKZGCommitment computes a KZG commitment from a given blob.
func BlobToKZGCommitment(blob *Blob) (Commitment, error) {
	var kzgBlob kzg4844.Blob
	copy(kzgBlob[:], blob[:])

	commitment, err := kzg4844.BlobToCommitment(&kzgBlob)
	if err != nil {
		return Commitment{}, fmt.Errorf("blob to commitmnent: %w", err)
	}

	return Commitment(commitment), nil
}

// ComputeBlobKZGProof computes the blob KZG proof from a given blob and its commitment.
func ComputeBlobKZGProof(blob *Blob, commitment Commitment) (Proof, error) {
	var kzgBlob kzg4844.Blob
	copy(kzgBlob[:], blob[:])

	proof, err := kzg4844.ComputeBlobProof(&kzgBlob, kzg4844.Commitment(commitment))
	if err != nil {
		return Proof{}, fmt.Errorf("compute blob KZG proof: %w", err)
	}

	var result Proof
	copy(result[:], proof[:])

	return result, nil
}

// ComputeCells computes the (extended) cells from a given blob.
func ComputeCells(blob *Blob) ([]Cell, error) {
	return activeBackend.ComputeCells(blob)
}

// ComputeCellsAndKZGProofs computes the cells and cells KZG proofs from a given blob.
func ComputeCellsAndKZGProofs(blob *Blob) ([]Cell, []Proof, error) {
	return activeBackend.ComputeCellsAndKZGProofs(blob)
}

// VerifyCellKZGProofBatch verifies the KZG proofs for a given slice of commitments, cells indices, cells and proofs.
// Note: It is way more efficient to call once this function with big slices than calling it multiple times with small slices.
func VerifyCellKZGProofBatch(commitmentsBytes []Bytes48, cellIndices []uint64, cells []Cell, proofsBytes []Bytes48) (bool, error) {
	return activeBackend.VerifyCellKZGProofBatch(commitmentsBytes, cellIndices, cells, proofsBytes)
}

// RecoverCells recovers the complete cells from a given set of cell indices and partial cells.
// Note: `len(cellIndices)` must be equal to `len(partialCells)` and `cellIndices` must be sorted in ascending order.
func RecoverCells(cellIndices []uint64, partialCells []Cell) ([]Cell, error) {
	return activeBackend.RecoverCells(cellIndices, partialCells)
}

// RecoverCellsAndKZGProofs recovers the complete cells and KZG proofs from a given set of cell indices and partial cells.
// Note: `len(cellIndices)` must be equal to `len(partialCells)` and `cellIndices` must be sorted in ascending order.
func RecoverCellsAndKZGProofs(cellIndices []uint64, partialCells []Cell) ([]Cell, []Proof, error) {
	return activeBackend.RecoverCellsAndKZGProofs(cellIndices, partialCells)
}
