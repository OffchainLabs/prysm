package kzg

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	constantine "github.com/nalepae/go-constantine"
	"github.com/pkg/errors"
)

// constantineBackend implements backend on top of the Constantine library.
type constantineBackend struct {
	ctx *constantine.Context
}

var _ backend = (*constantineBackend)(nil)

func (b *constantineBackend) LoadTrustedSetup(setup *TrustedSetup) error {
	if b.ctx != nil {
		return nil
	}

	// Constantine can only read a trusted setup from a file, so the embedded one
	// has to be written out before it can be loaded.
	path, err := writeTrustedSetupFile(setup)
	if err != nil {
		return fmt.Errorf("write trusted setup file: %w", err)
	}
	defer func() {
		// Only needed until the setup has been read.
		if err := os.Remove(path); err != nil {
			fmt.Fprintf(os.Stderr, "could not remove the trusted setup file: %v\n", err)
		}
	}()

	ctx, err := constantine.NewContext(path, constantine.RecommendedPrecompute)
	if err != nil {
		return fmt.Errorf("constantine new context: %w", err)
	}

	b.ctx = ctx

	return nil
}

func (b *constantineBackend) ComputeCells(blob *Blob) ([]Cell, error) {
	cells, err := b.ctx.ComputeCells((*constantine.Blob)(blob))
	if err != nil {
		return nil, err
	}

	return cellsFromConstantine(cells), nil
}

func (b *constantineBackend) ComputeCellsAndKZGProofs(blob *Blob) ([]Cell, []Proof, error) {
	cells, proofs, err := b.ctx.ComputeCellsAndKZGProofs((*constantine.Blob)(blob))
	if err != nil {
		return nil, nil, err
	}

	return cellsFromConstantine(cells), proofsFromConstantine(proofs), nil
}

// RecoverCells is served by Constantine's recover-with-proofs entry point, since
// it has no cells-only equivalent, and the proofs are discarded. The FK20 proof
// computation that this wastes is the expensive part of the operation.
func (b *constantineBackend) RecoverCells(cellIndices []uint64, partialCells []Cell) ([]Cell, error) {
	cells, _, err := b.ctx.RecoverCellsAndKZGProofs(cellIndices, cellsToConstantine(partialCells))
	if err != nil {
		return nil, err
	}

	return cellsFromConstantine(cells), nil
}

func (b *constantineBackend) RecoverCellsAndKZGProofs(cellIndices []uint64, partialCells []Cell) ([]Cell, []Proof, error) {
	cells, proofs, err := b.ctx.RecoverCellsAndKZGProofs(cellIndices, cellsToConstantine(partialCells))
	if err != nil {
		return nil, nil, err
	}

	return cellsFromConstantine(cells), proofsFromConstantine(proofs), nil
}

func (b *constantineBackend) VerifyCellKZGProofBatch(commitments []Bytes48, cellIndices []uint64, cells []Cell, proofs []Bytes48) (bool, error) {
	return b.ctx.VerifyCellKZGProofBatch(
		bytes48ToConstantine(commitments),
		cellIndices,
		cellsToConstantine(cells),
		bytes48ToConstantine(proofs),
	)
}

func (b *constantineBackend) VerifyBlobKZGProofBatch(blobs []Blob, commitments []Bytes48, proofs []Bytes48) (bool, error) {
	constantineBlobs := make([]constantine.Blob, len(blobs))
	for i := range blobs {
		constantineBlobs[i] = constantine.Blob(blobs[i])
	}

	return b.ctx.VerifyBlobKZGProofBatch(
		constantineBlobs,
		bytes48ToConstantine(commitments),
		bytes48ToConstantine(proofs),
	)
}

// The types of this package are distinct types while Constantine's are aliases of
// plain arrays, so the slices below have to be rebuilt rather than converted. The
// single-blob entry points above avoid that by passing a pointer, which is where
// it matters: those run once per blob on the hot path, whereas the copying here is
// a rounding error next to the verification it feeds, and is what the c-kzg backend
// does too.

func cellsToConstantine(cells []Cell) []constantine.Cell {
	out := make([]constantine.Cell, len(cells))
	for i := range cells {
		out[i] = constantine.Cell(cells[i])
	}

	return out
}

func cellsFromConstantine(cells *[constantine.CellsPerExtBlob]constantine.Cell) []Cell {
	out := make([]Cell, len(cells))
	for i := range cells {
		out[i] = Cell(cells[i])
	}

	return out
}

func proofsFromConstantine(proofs *[constantine.CellsPerExtBlob]constantine.Bytes48) []Proof {
	out := make([]Proof, len(proofs))
	for i := range proofs {
		out[i] = Proof(proofs[i])
	}

	return out
}

func bytes48ToConstantine(values []Bytes48) []constantine.Bytes48 {
	out := make([]constantine.Bytes48, len(values))
	for i := range values {
		out[i] = constantine.Bytes48(values[i])
	}

	return out
}

// writeTrustedSetupFile writes a trusted setup to a temporary file in the format
// the reference c-kzg implementation uses, which is the only one Constantine
// parses. It returns the path, which the caller is responsible for removing.
//
// The format is the number of G1 points, the number of G2 points, then the points
// as hex without the "0x" prefix, one per line: the G1 Lagrange points, the G2
// monomial points, and finally the G1 monomial points. That last section is not
// optional; EIP-7594 needs it.
func writeTrustedSetupFile(setup *TrustedSetup) (string, error) {
	file, err := os.CreateTemp("", "prysm-trusted-setup-*.txt")
	if err != nil {
		return "", errors.Wrap(err, "could not create a temporary file")
	}

	path := file.Name()

	if err := writeTrustedSetup(file, setup); err != nil {
		_ = file.Close()
		_ = os.Remove(path)

		return "", err
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(path)

		return "", errors.Wrap(err, "could not close the trusted setup file")
	}

	return path, nil
}

func writeTrustedSetup(file *os.File, setup *TrustedSetup) error {
	writer := bufio.NewWriter(file)

	if _, err := writer.WriteString("4096\n65\n"); err != nil {
		return errors.Wrap(err, "could not write the trusted setup header")
	}

	// The order of these sections is part of the format.
	for _, point := range setup.G1Lagrange {
		if err := writeTrustedSetupPoint(writer, string(point)); err != nil {
			return err
		}
	}

	for _, point := range setup.G2Monomial {
		if err := writeTrustedSetupPoint(writer, string(point)); err != nil {
			return err
		}
	}

	for _, point := range setup.G1Monomial {
		if err := writeTrustedSetupPoint(writer, string(point)); err != nil {
			return err
		}
	}

	if err := writer.Flush(); err != nil {
		return errors.Wrap(err, "could not flush the trusted setup file")
	}

	return nil
}

func writeTrustedSetupPoint(writer *bufio.Writer, point string) error {
	if _, err := writer.WriteString(strings.TrimPrefix(point, "0x")); err != nil {
		return errors.Wrap(err, "could not write a trusted setup point")
	}

	if err := writer.WriteByte('\n'); err != nil {
		return errors.Wrap(err, "could not write a trusted setup point")
	}

	return nil
}
