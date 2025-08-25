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
const BytesPerCell = ckzg4844.BytesPerCell

// Cell represents a chunk of an encoded Blob.
type Cell [BytesPerCell]byte

// Commitment represent a KZG commitment to a Blob.
type Commitment [48]byte

// Proof represents a KZG proof that attests to the validity of a Blob or parts of it.
type Proof [48]byte

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

// VerifyBlobKZGProofBatch verifies KZG proofs for multiple blobs using batch verification.
// This is more efficient than verifying each blob individually.
func VerifyBlobKZGProofBatch(blobs [][]byte, commitments [][]byte, proofs [][]byte) error {
	if len(blobs) != len(commitments) || len(blobs) != len(proofs) {
		return errors.New("number of blobs, commitments, and proofs must match")
	}

	if len(blobs) == 0 {
		return nil
	}

	// Convert to c-kzg-4844 types for batch verification
	ckzgBlobs := make([]ckzg4844.Blob, len(blobs))
	ckzgCommitments := make([]ckzg4844.Bytes48, len(commitments))
	ckzgProofs := make([]ckzg4844.Bytes48, len(proofs))

	for i := range blobs {
		copy(ckzgBlobs[i][:], blobs[i])
		copy(ckzgCommitments[i][:], commitments[i])
		copy(ckzgProofs[i][:], proofs[i])
	}

	valid, err := ckzg4844.VerifyBlobKZGProofBatch(ckzgBlobs, ckzgCommitments, ckzgProofs)
	if err != nil {
		return errors.Wrap(err, "batch verification failed")
	}
	if !valid {
		return errors.New("batch KZG proof verification failed")
	}

	return nil
}

// VerifyCellKZGProofBatchFromBlobData verifies cell KZG proofs in batch format directly from blob data.
// This is more efficient than reconstructing data column sidecars when you have the raw blob data and cell proofs.
// For PeerDAS/Fulu, the execution client provides cell proofs in flattened format via BlobsBundleV2.
func VerifyCellKZGProofBatchFromBlobData(blobs [][]byte, commitments [][]byte, cellProofs [][]byte, numberOfColumns uint64) error {
	blobCount := uint64(len(blobs))
	expectedCellProofs := blobCount * numberOfColumns
	
	if len(cellProofs) != int(expectedCellProofs) {
		return errors.Errorf("expected %d cell proofs, got %d", expectedCellProofs, len(cellProofs))
	}
	
	if len(commitments) != len(blobs) {
		return errors.Errorf("number of commitments (%d) must match number of blobs (%d)", len(commitments), len(blobs))
	}

	if blobCount == 0 {
		return nil
	}

	// Compute cells from blobs
	allCells := make([]Cell, 0, expectedCellProofs)
	allCommitments := make([]Bytes48, 0, expectedCellProofs)
	allIndices := make([]uint64, 0, expectedCellProofs)
	allProofs := make([]Bytes48, 0, expectedCellProofs)

	for blobIndex, blobData := range blobs {
		// Convert blob to kzg.Blob type
		var blob Blob
		copy(blob[:], blobData)

		// Compute cells for this blob
		cells, err := ComputeCells(&blob)
		if err != nil {
			return errors.Wrapf(err, "failed to compute cells for blob %d", blobIndex)
		}

		// Add cells and corresponding data for each column
		for columnIndex := uint64(0); columnIndex < numberOfColumns; columnIndex++ {
			cellProofIndex := uint64(blobIndex)*numberOfColumns + columnIndex
			
			allCells = append(allCells, cells[columnIndex])
			allCommitments = append(allCommitments, Bytes48(commitments[blobIndex]))
			allIndices = append(allIndices, columnIndex)
			
			var proof Bytes48
			copy(proof[:], cellProofs[cellProofIndex])
			allProofs = append(allProofs, proof)
		}
	}

	// Batch verify all cells
	valid, err := VerifyCellKZGProofBatch(allCommitments, allIndices, allCells, allProofs)
	if err != nil {
		return errors.Wrap(err, "cell batch verification failed")
	}
	if !valid {
		return errors.New("cell KZG proof batch verification failed")
	}

	return nil
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
