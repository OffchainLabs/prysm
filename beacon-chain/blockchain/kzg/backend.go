package kzg

// backend is the set of KZG operations that Prysm delegates to an external
// cryptography library. Every call into such a library goes through this
// interface, so that the implementation can be swapped without touching the
// callers, and so that adding a new one means adding a single file.
//
// Scope: the operations Prysm takes from github.com/ethereum/c-kzg-4844/v2
type backend interface {
	// LoadTrustedSetup initializes the backend from the embedded trusted setup.
	LoadTrustedSetup(setup *TrustedSetup) error

	// ComputeCells computes the extended cells of a blob.
	ComputeCells(blob *Blob) ([]Cell, error)

	// ComputeCellsAndKZGProofs computes the extended cells of a blob together
	// with a proof for each cell.
	ComputeCellsAndKZGProofs(blob *Blob) ([]Cell, []Proof, error)

	// RecoverCells reconstructs every cell from a partial set. cellIndices must
	// be sorted in ascending order and have the same length as partialCells.
	RecoverCells(cellIndices []uint64, partialCells []Cell) ([]Cell, error)

	// RecoverCellsAndKZGProofs reconstructs every cell and its proof from a
	// partial set. cellIndices must be sorted in ascending order and have the
	// same length as partialCells.
	RecoverCellsAndKZGProofs(cellIndices []uint64, partialCells []Cell) ([]Cell, []Proof, error)

	// VerifyCellKZGProofBatch verifies a batch of cell proofs. All four slices
	// must have the same length.
	VerifyCellKZGProofBatch(commitments []Bytes48, cellIndices []uint64, cells []Cell, proofs []Bytes48) (bool, error)

	// VerifyBlobKZGProofBatch verifies a batch of blob proofs. All three slices
	// must have the same length.
	VerifyBlobKZGProofBatch(blobs []Blob, commitments []Bytes48, proofs []Bytes48) (bool, error)
}

// activeBackend is the backend that the exported functions of this package route
// through.
var activeBackend backend = ckzgBackend{}
