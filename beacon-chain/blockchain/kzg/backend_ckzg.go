package kzg

import (
	"fmt"

	ckzg4844 "github.com/ethereum/c-kzg-4844/v2/bindings/go"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Assert that the sizes this package defines agree with the ones the c-kzg
// bindings use. Both arrays of a pair have to be zero-length, so a mismatch in
// either direction is a compile error rather than a silent misreading of bytes.
var (
	_ [BytesPerBlob - ckzg4844.BytesPerBlob]struct{}
	_ [ckzg4844.BytesPerBlob - BytesPerBlob]struct{}
	_ [BytesPerCell - ckzg4844.BytesPerCell]struct{}
	_ [ckzg4844.BytesPerCell - BytesPerCell]struct{}
	_ [BytesPerProof - ckzg4844.BytesPerProof]struct{}
	_ [ckzg4844.BytesPerProof - BytesPerProof]struct{}
)

var ckzgLoaded bool

// ckzgBackend implements backend on top of the c-kzg-4844 bindings.
type ckzgBackend struct{}

var _ backend = ckzgBackend{}

// decodeHexPoints concatenates a list of 0x-prefixed, equal-length hex encoded
// curve points into the flat byte slice c-kzg expects.
func decodeHexPoints[T ~string](points []T) []byte {
	if len(points) == 0 {
		return nil
	}

	// Every point has the same encoded length: two hex characters per byte, less
	// the "0x" prefix.
	bytesPerPoint := (len(points[0]) - 2) / 2

	out := make([]byte, 0, len(points)*bytesPerPoint)
	for _, point := range points {
		out = append(out, hexutil.MustDecode(string(point))...)
	}

	return out
}

func (ckzgBackend) LoadTrustedSetup(setup *TrustedSetup) error {
	if ckzgLoaded {
		return nil
	}

	// Trades memory for faster cell proof computation.
	const precompute uint = 8

	g1Monomial := decodeHexPoints(setup.G1Monomial[:])
	g1Lagrange := decodeHexPoints(setup.G1Lagrange[:])
	g2Monomial := decodeHexPoints(setup.G2Monomial[:])

	if err := ckzg4844.LoadTrustedSetup(g1Monomial, g1Lagrange, g2Monomial, precompute); err != nil {
		return fmt.Errorf("load trusted setup: %w", err)
	}

	ckzgLoaded = true

	return nil
}

func (ckzgBackend) ComputeCells(blob *Blob) ([]Cell, error) {
	ckzgBlob := ckzg4844.Blob(*blob)

	ckzgCells, err := ckzg4844.ComputeCells(&ckzgBlob)
	if err != nil {
		return nil, fmt.Errorf("compute cells: %w", err)
	}

	return cellsFromCKZG(ckzgCells[:]), nil
}

func (ckzgBackend) ComputeCellsAndKZGProofs(blob *Blob) ([]Cell, []Proof, error) {
	ckzgBlob := ckzg4844.Blob(*blob)

	ckzgCells, ckzgProofs, err := ckzg4844.ComputeCellsAndKZGProofs(&ckzgBlob)
	if err != nil {
		return nil, nil, fmt.Errorf("compute cells and KZG proofs: %w", err)
	}

	return cellsFromCKZG(ckzgCells[:]), proofsFromCKZG(ckzgProofs[:]), nil
}

func (ckzgBackend) RecoverCells(cellIndices []uint64, partialCells []Cell) ([]Cell, error) {
	ckzgCells, err := ckzg4844.RecoverCells(cellIndices, cellsToCKZG(partialCells))
	if err != nil {
		return nil, fmt.Errorf("recover cells: %w", err)
	}

	return cellsFromCKZG(ckzgCells[:]), nil
}

func (ckzgBackend) RecoverCellsAndKZGProofs(cellIndices []uint64, partialCells []Cell) ([]Cell, []Proof, error) {
	ckzgCells, ckzgProofs, err := ckzg4844.RecoverCellsAndKZGProofs(cellIndices, cellsToCKZG(partialCells))
	if err != nil {
		return nil, nil, fmt.Errorf("recover cells and KZG proofs: %w", err)
	}

	return cellsFromCKZG(ckzgCells[:]), proofsFromCKZG(ckzgProofs[:]), nil
}

func (ckzgBackend) VerifyCellKZGProofBatch(commitments []Bytes48, cellIndices []uint64, cells []Cell, proofs []Bytes48) (bool, error) {
	verified, err := ckzg4844.VerifyCellKZGProofBatch(bytes48ToCKZG(commitments), cellIndices, cellsToCKZG(cells), bytes48ToCKZG(proofs))
	if err != nil {
		return false, fmt.Errorf("verify cell KZG proof batch: %w", err)
	}

	return verified, nil
}

func (ckzgBackend) VerifyBlobKZGProofBatch(blobs []Blob, commitments []Bytes48, proofs []Bytes48) (bool, error) {
	ckzgBlobs := make([]ckzg4844.Blob, 0, len(blobs))
	for _, blob := range blobs {
		ckzgBlobs = append(ckzgBlobs, ckzg4844.Blob(blob))
	}

	verified, err := ckzg4844.VerifyBlobKZGProofBatch(ckzgBlobs, bytes48ToCKZG(commitments), bytes48ToCKZG(proofs))
	if err != nil {
		return false, fmt.Errorf("verify blob KZG proof batch: %w", err)
	}

	return verified, nil
}

func cellsToCKZG(cells []Cell) []ckzg4844.Cell {
	ckzgCells := make([]ckzg4844.Cell, 0, len(cells))
	for _, cell := range cells {
		ckzgCells = append(ckzgCells, ckzg4844.Cell(cell))
	}

	return ckzgCells
}

func cellsFromCKZG(ckzgCells []ckzg4844.Cell) []Cell {
	cells := make([]Cell, 0, len(ckzgCells))
	for _, ckzgCell := range ckzgCells {
		cells = append(cells, Cell(ckzgCell))
	}

	return cells
}

func proofsFromCKZG(ckzgProofs []ckzg4844.KZGProof) []Proof {
	proofs := make([]Proof, 0, len(ckzgProofs))
	for _, ckzgProof := range ckzgProofs {
		proofs = append(proofs, Proof(ckzgProof))
	}

	return proofs
}

func bytes48ToCKZG(values []Bytes48) []ckzg4844.Bytes48 {
	ckzgValues := make([]ckzg4844.Bytes48, 0, len(values))
	for _, value := range values {
		ckzgValues = append(ckzgValues, ckzg4844.Bytes48(value))
	}

	return ckzgValues
}
