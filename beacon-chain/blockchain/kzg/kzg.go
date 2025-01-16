package kzg

import (
	"errors"

	goethkzg "github.com/crate-crypto/go-eth-kzg"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
)

// BytesPerBlob is the number of bytes in a single (non extended) blob.
const BytesPerBlob = goethkzg.BytesPerCell * goethkzg.CellsPerExtBlob / 2

// Blob represents a serialized chunk of data.
type Blob [BytesPerBlob]byte

// BytesPerCell is the number of bytes in a single cell.
const BytesPerCell = goethkzg.BytesPerCell

// Cell represents a chunk of an encoded Blob.
type Cell [BytesPerCell]byte

// Commitment represent a KZG commitment to a Blob.
type Commitment [48]byte

// Proof represents a KZG proof that attests to the validity of a Blob or parts of it.
type Proof [48]byte

// Bytes48 is a 48-byte array.
type Bytes48 = [48]byte

// Bytes32 is a 32-byte array.
type Bytes32 = [32]byte

// CellsAndProofs represents the Cells and Proofs corresponding to
// a single blob.
type CellsAndProofs struct {
	Cells  []Cell
	Proofs []Proof
}

func BlobToKZGCommitment(blob *Blob) (Commitment, error) {
	kzgBlob := kzg4844.Blob(*blob)
	comm, err := kzg4844.BlobToCommitment(&kzgBlob)
	if err != nil {
		return Commitment{}, err
	}
	return Commitment(comm), nil
}

func ComputeBlobKZGProof(blob *Blob, commitment Commitment) (Proof, error) {
	kzgBlob := kzg4844.Blob(*blob)
	proof, err := kzg4844.ComputeBlobProof(&kzgBlob, kzg4844.Commitment(commitment))
	if err != nil {
		return [48]byte{}, err
	}
	return Proof(proof), nil
}

func ComputeCellsAndKZGProofs(blob *Blob) (CellsAndProofs, error) {
	goEthKZGBlob := (*goethkzg.Blob)(blob)
	cells, proofs, err := goEthKZGContext.ComputeCellsAndKZGProofs(goEthKZGBlob, 0)
	if err != nil {
		return CellsAndProofs{}, err
	}
	return makeCellsAndProofsGoEthKZG(cells[:], proofs[:])
}

// Convert c-kzg cells/proofs to the CellsAndProofs type defined in this package.
func makeCellsAndProofsGoEthKZG(goethkzgCells []*goethkzg.Cell, goethkzgProofs []goethkzg.KZGProof) (CellsAndProofs, error) {
	if len(goethkzgCells) != len(goethkzgProofs) {
		return CellsAndProofs{}, errors.New("different number of cells/proofs")
	}

	var cells []Cell
	var proofs []Proof
	for i := range goethkzgCells {
		cells = append(cells, Cell(*goethkzgCells[i]))
		proofs = append(proofs, Proof(goethkzgProofs[i]))
	}

	return CellsAndProofs{
		Cells:  cells,
		Proofs: proofs,
	}, nil
}

func convertBytes48SliceToKZGCommitmentSlice(bytes48Slice []Bytes48) []goethkzg.KZGCommitment {
	commitments := make([]goethkzg.KZGCommitment, len(bytes48Slice))
	for i, b48 := range bytes48Slice {
		copy(commitments[i][:], b48[:])
	}
	return commitments
}

func convertCellSliceToPointers(cells []Cell) []*goethkzg.Cell {
	cellPointers := make([]*goethkzg.Cell, len(cells))
	for i := range cells {
		kzgCell := goethkzg.Cell(cells[i])
		cellPointers[i] = &kzgCell
	}
	return cellPointers
}

func convertBytes48SliceToKZGProofSlice(bytes48Slice []Bytes48) []goethkzg.KZGProof {
	commitments := make([]goethkzg.KZGProof, len(bytes48Slice))
	for i, b48 := range bytes48Slice {
		copy(commitments[i][:], b48[:])
	}
	return commitments
}

func VerifyCellKZGProofBatch(commitmentsBytes []Bytes48, cellIndices []uint64, cells []Cell, proofsBytes []Bytes48) (bool, error) {
	kzgCommitments := convertBytes48SliceToKZGCommitmentSlice(commitmentsBytes)
	kzgCells := convertCellSliceToPointers(cells)
	kzgProofs := convertBytes48SliceToKZGProofSlice(proofsBytes)

	err := goEthKZGContext.VerifyCellKZGProofBatch(kzgCommitments, cellIndices, kzgCells, kzgProofs)
	if err != nil {
		return false, err
	}
	// TODO: This conforms to the c-kzg API, I think we should change this to only return an error
	return true, nil
}

func RecoverCellsAndKZGProofs(cellIndices []uint64, partialCells []Cell) (CellsAndProofs, error) {
	kzgCells := convertCellSliceToPointers(partialCells)
	cells, proofs, err := goEthKZGContext.RecoverCellsAndComputeKZGProofs(cellIndices, kzgCells, 0)
	if err != nil {
		return CellsAndProofs{}, err
	}

	return makeCellsAndProofsGoEthKZG(cells[:], proofs[:])
}
