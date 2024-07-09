package kzg

import (
	"errors"

	ckzg4844 "github.com/ethereum/c-kzg-4844/bindings/go"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
)

// Blob represents a serialized chunk of data.
type Blob [BytesPerBlob]byte

// Commitment represent a KZG commitment to a Blob.
type Commitment [48]byte

// Proof represents a KZG proof that attests to the validity of a Blob or parts of it.
type Proof [48]byte

// Bytes48 is a 48-byte array.
type Bytes48 = ckzg4844.Bytes48

// Bytes32 is a 32-byte array.
type Bytes32 = ckzg4844.Bytes32

// BytesPerCell is the number of bytes in a single cell.
const BytesPerCell = ckzg4844.FieldElementsPerCell * ckzg4844.BytesPerFieldElement

// BytesPerBlob is the number of bytes in a single blob.
const BytesPerBlob = ckzg4844.BytesPerBlob

// fieldElementsPerCell is the number of field elements in a single cell.
const fieldElementsPerCell = ckzg4844.FieldElementsPerCell

// CellsPerExtBlob is the number of cells that we generate for a single blob.
// This is equivalent to the number of columns in the data matrix.
const CellsPerExtBlob = ckzg4844.CellsPerExtBlob

// Cell represents a chunk of an encoded Blob.
type Cell [BytesPerCell]byte

// CellsAndProofs represents the Cells and Proofs corresponding to
// a single blob.
type CellsAndProofs struct {
	Cells  [ckzg4844.CellsPerExtBlob]Cell
	Proofs [ckzg4844.CellsPerExtBlob]Proof
}

func BlobToKZGCommitment(blob *Blob) (Commitment, error) {
	comm, err := kzg4844.BlobToCommitment(kzg4844.Blob(*blob))
	if err != nil {
		return Commitment{}, err
	}
	return Commitment(comm), nil
}

func ComputeBlobKZGProof(blob *Blob, commitment Commitment) (Proof, error) {
	proof, err := kzg4844.ComputeBlobProof(kzg4844.Blob(*blob), kzg4844.Commitment(commitment))
	if err != nil {
		return [48]byte{}, err
	}
	return Proof(proof), nil
}

func ComputeCellsAndKZGProofs(blob *Blob) (CellsAndProofs, error) {
	ckzgBlob := ckzg4844.Blob(*blob)
	_cells, _proofs, err := ckzg4844.ComputeCellsAndKZGProofs(&ckzgBlob)
	if err != nil {
		return CellsAndProofs{}, err
	}

	// Convert Cells and Proofs to types defined in this package
	var cells [ckzg4844.CellsPerExtBlob]Cell
	for i := range _cells {
		cells[i] = ckzgCellToCell(&_cells[i])
	}

	var proofs [ckzg4844.CellsPerExtBlob]Proof
	for i := range _proofs {
		proofs[i] = Proof(_proofs[i])
	}

	return CellsAndProofs{
		Cells:  cells,
		Proofs: proofs,
	}, nil
}

// VerifyCellKZGProof is unused. TODO: We can check when the batch size for `VerifyCellKZGProofBatch` is 1
// and call this, though I think its better if the cryptography library handles this.
func VerifyCellKZGProof(commitmentBytes Bytes48, cellId uint64, cell *Cell, proofBytes Bytes48) (bool, error) {
	return ckzg4844.VerifyCellKZGProof(commitmentBytes, cellId, cellToCKZGCell(cell), proofBytes)
}

func VerifyCellKZGProofBatch(commitmentsBytes []Bytes48, rowIndices, columnIndices []uint64, _cells []Cell, proofsBytes []Bytes48) (bool, error) {
	// Convert `Cell` type to `ckzg4844.Cell`
	ckzgCells := make([]ckzg4844.Cell, len(_cells))
	for i := range _cells {
		ckzgCells[i] = cellToCKZGCell(&_cells[i])
	}

	return ckzg4844.VerifyCellKZGProofBatch(commitmentsBytes, rowIndices, columnIndices, ckzgCells, proofsBytes)
}

func recoverAllCells(cellIds []uint64, _cells []Cell) ([ckzg4844.CellsPerExtBlob]Cell, error) {
	// Convert `Cell` type to `ckzg4844.Cell`
	ckzgCells := make([]ckzg4844.Cell, len(_cells))
	for i := range _cells {
		ckzgCells[i] = cellToCKZGCell(&_cells[i])
	}

	recoveredCells, err := ckzg4844.RecoverAllCells(cellIds, ckzgCells)
	if err != nil {
		return [ckzg4844.CellsPerExtBlob]Cell{}, err
	}

	// This should never happen, we return an error instead of panicking.
	if len(recoveredCells) != ckzg4844.CellsPerExtBlob {
		return [ckzg4844.CellsPerExtBlob]Cell{}, errors.New("recovered cells length is not equal to CellsPerExtBlob")
	}

	// Convert `ckzg4844.Cell` type to `Cell`
	var ret [ckzg4844.CellsPerExtBlob]Cell
	for i := range recoveredCells {
		ret[i] = ckzgCellToCell(&recoveredCells[i])
	}
	return ret, nil
}

// RecoverCellsAndKZGProofs recovers the cells and compute the KZG Proofs associated with the cells.
func RecoverCellsAndKZGProofs(cellIds []uint64, _cells []Cell) (CellsAndProofs, error) {
	// First recover all of the cells
	recoveredCells, err := recoverAllCells(cellIds, _cells)
	if err != nil {
		return CellsAndProofs{}, err
	}

	// Extract the Blob from all of the Cells
	blob, err := cellsToBlob(&recoveredCells)
	if err != nil {
		return CellsAndProofs{}, err
	}

	// Compute all of the cells and KZG proofs
	return ComputeCellsAndKZGProofs(&blob)
}

func cellsToBlob(_cells *[ckzg4844.CellsPerExtBlob]Cell) (Blob, error) {
	// Convert `Cell` type to `ckzg4844.Cell`
	var ckzgCells [ckzg4844.CellsPerExtBlob]ckzg4844.Cell
	for i := range _cells {
		ckzgCells[i] = cellToCKZGCell(&_cells[i])
	}

	blob, err := ckzg4844.CellsToBlob(ckzgCells)
	if err != nil {
		return Blob{}, err
	}

	return Blob(blob), nil
}

// The correct type for Cell is [BytesPerCell]byte
// c-kzg currently uses [BytesPerFieldElement]Bytes32
// so we have these helper methods to convert between the two.
func cellToCKZGCell(flattened *Cell) ckzg4844.Cell {
	var cell ckzg4844.Cell
	for i := 0; i < fieldElementsPerCell; i++ {
		copy(cell[i][:], flattened[i*32:(i+1)*32])
	}
	return cell
}
func ckzgCellToCell(cell *ckzg4844.Cell) Cell {
	var flattened Cell
	for i, fieldElement := range cell {
		copy(flattened[i*32:(i+1)*32], fieldElement[:])
	}
	return flattened
}
