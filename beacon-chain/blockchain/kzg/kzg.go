package kzg

import (
	"runtime"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

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

// errInvalidProof is returned when KZG proof verification fails.
var errInvalidProof = errors.New("invalid KZG proof")

// Proof represents a KZG proof that attests to the validity of a Blob or parts of it.
type Proof [BytesPerProof]byte

// Bytes48 is a 48-byte array.
type Bytes48 = ckzg4844.Bytes48

// Bytes32 is a 32-byte array.
type Bytes32 = ckzg4844.Bytes32

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
		copy(cells[i][:], ckzgCells[i][:])
	}

	return cells, nil
}

// ComputeBlobKZGProof computes the blob KZG proof from a given blob and its commitment.
func ComputeBlobKZGProof(blob *Blob, commitment Commitment) (Proof, error) {
	var kzgBlob kzg4844.Blob
	copy(kzgBlob[:], blob[:])

	proof, err := kzg4844.ComputeBlobProof(&kzgBlob, kzg4844.Commitment(commitment))
	if err != nil {
		return Proof{}, err
	}
	var result Proof
	copy(result[:], proof[:])
	return result, nil
}

// ComputeCellsAndKZGProofs computes the cells and cells KZG proofs from a given blob.
func ComputeCellsAndKZGProofs(blob *Blob) ([]Cell, []Proof, error) {
	var ckzgBlob ckzg4844.Blob
	copy(ckzgBlob[:], blob[:])

	ckzgCells, ckzgProofs, err := ckzg4844.ComputeCellsAndKZGProofs(&ckzgBlob)
	if err != nil {
		return nil, nil, err
	}

	if len(ckzgCells) != len(ckzgProofs) {
		return nil, nil, errors.New("mismatched cells and proofs length")
	}

	cells := make([]Cell, len(ckzgCells))
	proofs := make([]Proof, len(ckzgProofs))
	for i := range ckzgCells {
		copy(cells[i][:], ckzgCells[i][:])
		copy(proofs[i][:], ckzgProofs[i][:])
	}

	return cells, proofs, nil
}

// chunkBounds represents the start and end indices of a chunk.
type chunkBounds struct {
	start, end int
}

// VerifyCellKZGProofBatch verifies the KZG proofs for a given slice of commitments, cells indices, cells and proofs.
// The verification is parallelized across CPU cores by splitting the input into chunks.
func VerifyCellKZGProofBatch(commitmentsBytes []Bytes48, cellIndices []uint64, cells []Cell, proofsBytes []Bytes48) (bool, error) {
	count := len(cells)

	// Validate all input slices have the same length
	if len(commitmentsBytes) != count || len(cellIndices) != count || len(proofsBytes) != count {
		return false, errors.New("input slices must have equal length")
	}

	// Convert `Cell` type to `ckzg4844.Cell`
	ckzgCells := make([]ckzg4844.Cell, count)
	for i := range cells {
		copy(ckzgCells[i][:], cells[i][:])
	}

	if count == 0 {
		return true, nil
	}

	workerCount := min(count, runtime.GOMAXPROCS(0))
	chunks := computeChunkBounds(count, workerCount)

	var wg errgroup.Group
	for workerIdx := range workerCount {
		bounds := chunks[workerIdx]

		wg.Go(func() error {
			// Verify this chunk
			valid, err := ckzg4844.VerifyCellKZGProofBatch(
				commitmentsBytes[bounds.start:bounds.end],
				cellIndices[bounds.start:bounds.end],
				ckzgCells[bounds.start:bounds.end],
				proofsBytes[bounds.start:bounds.end],
			)

			if err != nil {
				return err
			}

			if !valid {
				return errInvalidProof
			}

			return nil
		})
	}

	// Wait for all workers to complete
	if err := wg.Wait(); err != nil {
		if errors.Is(err, errInvalidProof) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// RecoverCells recovers the complete cells from a given set of cell indices and partial cells.
// Note: `len(cellIndices)` must be equal to `len(partialCells)` and `cellIndices` must be sorted in ascending order.
func RecoverCells(cellIndices []uint64, partialCells []Cell) ([]Cell, error) {
	// Convert `Cell` type to `ckzg4844.Cell`
	ckzgPartialCells := make([]ckzg4844.Cell, len(partialCells))
	for i := range partialCells {
		copy(ckzgPartialCells[i][:], partialCells[i][:])
	}

	ckzgCells, err := ckzg4844.RecoverCells(cellIndices, ckzgPartialCells)
	if err != nil {
		return nil, errors.Wrap(err, "recover cells")
	}

	cells := make([]Cell, len(ckzgCells))
	for i := range ckzgCells {
		copy(cells[i][:], ckzgCells[i][:])
	}

	return cells, nil
}

// RecoverCellsAndKZGProofs recovers the complete cells and KZG proofs from a given set of cell indices and partial cells.
// Note: `len(cellIndices)` must be equal to `len(partialCells)` and `cellIndices` must be sorted in ascending order.
func RecoverCellsAndKZGProofs(cellIndices []uint64, partialCells []Cell) ([]Cell, []Proof, error) {
	// Convert `Cell` type to `ckzg4844.Cell`
	ckzgPartialCells := make([]ckzg4844.Cell, len(partialCells))
	for i := range partialCells {
		copy(ckzgPartialCells[i][:], partialCells[i][:])
	}

	ckzgCells, ckzgProofs, err := ckzg4844.RecoverCellsAndKZGProofs(cellIndices, ckzgPartialCells)
	if err != nil {
		return nil, nil, errors.Wrap(err, "recover cells and KZG proofs")
	}

	if len(ckzgCells) != len(ckzgProofs) {
		return nil, nil, errors.New("mismatched cells and proofs length")
	}

	cells := make([]Cell, len(ckzgCells))
	proofs := make([]Proof, len(ckzgProofs))
	for i := range ckzgCells {
		copy(cells[i][:], ckzgCells[i][:])
		copy(proofs[i][:], ckzgProofs[i][:])
	}

	return cells, proofs, nil
}

// computeChunkBounds calculates evenly distributed chunk boundaries for parallel processing.
// It splits itemsCount into chunks, distributing any remainder across the first chunks.
func computeChunkBounds(itemsCount, workerCount int) []chunkBounds {
	actualWorkers := min(itemsCount, workerCount)

	if actualWorkers == 0 {
		return []chunkBounds{}
	}

	chunkSize := itemsCount / actualWorkers
	remainder := itemsCount % actualWorkers

	chunks := make([]chunkBounds, 0, actualWorkers)
	offset := 0
	for i := range actualWorkers {
		size := chunkSize
		if i < remainder {
			size++
		}

		chunks = append(chunks, chunkBounds{start: offset, end: offset + size})
		offset += size
	}

	return chunks
}
