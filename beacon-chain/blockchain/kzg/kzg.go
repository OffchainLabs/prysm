package kzg

import (
	"github.com/pkg/errors"

	ckzg4844 "github.com/ethereum/c-kzg-4844/v2/bindings/go"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
)

// BytesPerBlob is the number of bytes in a single blob.
const BytesPerBlob = ckzg4844.BytesPerBlob

// Blob represents a serialized chunk of data.
type Blob [BytesPerBlob]byte

// BytesPerCell is the number of bytes in a single cell.
const (
	BytesPerCell  = ckzg4844.BytesPerCell
	BytesPerProof = ckzg4844.BytesPerProof
)

// Cell represents a chunk of an encoded Blob.
type Cell [BytesPerCell]byte

// Commitment represent a KZG commitment to a Blob.
type Commitment [48]byte

// Proof represents a KZG proof that attests to the validity of a Blob or parts of it.
type Proof [BytesPerProof]byte

// Bytes48 is a 48-byte array.
type Bytes48 = ckzg4844.Bytes48

// Bytes32 is a 32-byte array.
type Bytes32 = ckzg4844.Bytes32

// CellsAndProofs represents the Cells and Proofs corresponding to a single blob.
type CellsAndProofs struct {
	Cells  []Cell
	Proofs []Proof
}

// BlobToKZGCommitment computes a KZG commitment from a given blob.
func BlobToKZGCommitment(blob *Blob) (Commitment, error) {
	var kzgBlob kzg4844.Blob
	copy(kzgBlob[:], blob[:])

	commitment, err := kzg4844.BlobToCommitment(&kzgBlob)
	if err != nil {
		return Commitment{}, err
	}

	return Commitment(commitment), nil
}

// ComputeCells computes the (extended) cells from a given blob.
func ComputeCells(blob *Blob) ([]Cell, error) {
	var ckzgBlob ckzg4844.Blob
	copy(ckzgBlob[:], blob[:])

	ckzgCells, err := ckzg4844.ComputeCells(&ckzgBlob)
	if err != nil {
		return nil, errors.Wrap(err, "compute cells")
	}

	cells := make([]Cell, len(ckzgCells))
	for i := range ckzgCells {
		cells[i] = Cell(ckzgCells[i])
	}

	return cells, nil
}

// ComputeBlobKZGProof computes the blob KZG proof from a given blob and its commitment.
func ComputeBlobKZGProof(blob *Blob, commitment Commitment) (Proof, error) {
	var kzgBlob kzg4844.Blob
	copy(kzgBlob[:], blob[:])

	proof, err := kzg4844.ComputeBlobProof(&kzgBlob, kzg4844.Commitment(commitment))
	if err != nil {
		return [48]byte{}, err
	}
	return Proof(proof), nil
}

// ComputeCellsAndKZGProofs computes the cells and cells KZG proofs from a given blob.
func ComputeCellsAndKZGProofs(blob *Blob) (CellsAndProofs, error) {
	var ckzgBlob ckzg4844.Blob
	copy(ckzgBlob[:], blob[:])

	ckzgCells, ckzgProofs, err := ckzg4844.ComputeCellsAndKZGProofs(&ckzgBlob)
	if err != nil {
		return CellsAndProofs{}, err
	}

	return makeCellsAndProofs(ckzgCells[:], ckzgProofs[:])
}

// VerifyCellKZGProofBatch verifies the KZG proofs for a given slice of commitments, cells indices, cells and proofs.
// Note: It is way more efficient to call once this function with big slices than calling it multiple times with small slices.
func VerifyCellKZGProofBatch(commitmentsBytes []Bytes48, cellIndices []uint64, cells []Cell, proofsBytes []Bytes48) (bool, error) {
	// Convert `Cell` type to `ckzg4844.Cell`
	ckzgCells := make([]ckzg4844.Cell, len(cells))

	for i := range cells {
		ckzgCells[i] = ckzg4844.Cell(cells[i])
	}
	return ckzg4844.VerifyCellKZGProofBatch(commitmentsBytes, cellIndices, ckzgCells, proofsBytes)
}

// RecoverCellsAndKZGProofs recovers the complete cells and KZG proofs from a given set of cell indices and partial cells.
// Note: `len(cellIndices)` must be equal to `len(partialCells)` and `cellIndices` must be sorted in ascending order.
func RecoverCellsAndKZGProofs(cellIndices []uint64, partialCells []Cell) (CellsAndProofs, error) {
	// Convert `Cell` type to `ckzg4844.Cell`
	ckzgPartialCells := make([]ckzg4844.Cell, len(partialCells))
	for i := range partialCells {
		ckzgPartialCells[i] = ckzg4844.Cell(partialCells[i])
	}

	ckzgCells, ckzgProofs, err := ckzg4844.RecoverCellsAndKZGProofs(cellIndices, ckzgPartialCells)
	if err != nil {
		return CellsAndProofs{}, errors.Wrap(err, "recover cells and KZG proofs")
	}

	return makeCellsAndProofs(ckzgCells[:], ckzgProofs[:])
}

// makeCellsAndProofs converts cells/proofs to the CellsAndProofs type defined in this package.
func makeCellsAndProofs(ckzgCells []ckzg4844.Cell, ckzgProofs []ckzg4844.KZGProof) (CellsAndProofs, error) {
	if len(ckzgCells) != len(ckzgProofs) {
		return CellsAndProofs{}, errors.New("different number of cells/proofs")
	}

	cells := make([]Cell, 0, len(ckzgCells))
	proofs := make([]Proof, 0, len(ckzgProofs))

	for i := range ckzgCells {
		cells = append(cells, Cell(ckzgCells[i]))
		proofs = append(proofs, Proof(ckzgProofs[i]))
	}

	return CellsAndProofs{
		Cells:  cells,
		Proofs: proofs,
	}, nil
}
