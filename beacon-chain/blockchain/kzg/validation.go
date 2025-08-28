package kzg

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
	ckzg4844 "github.com/ethereum/c-kzg-4844/v2/bindings/go"
	"github.com/pkg/errors"
)

func bytesToBlob(blob []byte) *GoKZG.Blob {
	var ret GoKZG.Blob
	copy(ret[:], blob)
	return &ret
}

func bytesToCommitment(commitment []byte) (ret GoKZG.KZGCommitment) {
	copy(ret[:], commitment)
	return
}

func bytesToKZGProof(proof []byte) (ret GoKZG.KZGProof) {
	copy(ret[:], proof)
	return
}

// Verify performs single or batch verification of commitments depending on the number of given BlobSidecars.
func Verify(blobSidecars ...blocks.ROBlob) error {
	if len(blobSidecars) == 0 {
		return nil
	}
	if len(blobSidecars) == 1 {
		return kzgContext.VerifyBlobKZGProof(
			bytesToBlob(blobSidecars[0].Blob),
			bytesToCommitment(blobSidecars[0].KzgCommitment),
			bytesToKZGProof(blobSidecars[0].KzgProof))
	}
	blobs := make([]GoKZG.Blob, len(blobSidecars))
	cmts := make([]GoKZG.KZGCommitment, len(blobSidecars))
	proofs := make([]GoKZG.KZGProof, len(blobSidecars))
	for i, sidecar := range blobSidecars {
		blobs[i] = *bytesToBlob(sidecar.Blob)
		cmts[i] = bytesToCommitment(sidecar.KzgCommitment)
		proofs[i] = bytesToKZGProof(sidecar.KzgProof)
	}
	return kzgContext.VerifyBlobKZGProofBatch(blobs, cmts, proofs)
}

// VerifyBlobKZGProofBatch verifies KZG proofs for multiple blobs using batch verification.
// This is more efficient than verifying each blob individually when len(blobs) > 1.
// For single blob verification, it uses the optimized single verification path.
func VerifyBlobKZGProofBatch(blobs [][]byte, commitments [][]byte, proofs [][]byte) error {
	if len(blobs) != len(commitments) || len(blobs) != len(proofs) {
		return errors.Errorf("number of blobs (%d), commitments (%d), and proofs (%d) must match", len(blobs), len(commitments), len(proofs))
	}

	if len(blobs) == 0 {
		return nil
	}

	// Optimize for single blob case - use single verification to avoid batch overhead
	if len(blobs) == 1 {
		return kzgContext.VerifyBlobKZGProof(
			bytesToBlob(blobs[0]),
			bytesToCommitment(commitments[0]),
			bytesToKZGProof(proofs[0]))
	}

	// Use batch verification for multiple blobs
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
		return errors.Wrap(err, "batch verification")
	}
	if !valid {
		return errors.New("batch KZG proof verification failed")
	}

	return nil
}

// VerifyCellKZGProofBatchFromBlobData verifies cell KZG proofs in batch format directly from blob data.
// This is more efficient than reconstructing data column sidecars when you have the raw blob data and cell proofs.
// For PeerDAS/Fulu, the execution client provides cell proofs in flattened format via BlobsBundleV2.
// For single blob verification, it optimizes by computing cells once and verifying efficiently.
func VerifyCellKZGProofBatchFromBlobData(blobs [][]byte, commitments [][]byte, cellProofs [][]byte, numberOfColumns uint64) error {
	blobCount := uint64(len(blobs))
	expectedCellProofs := blobCount * numberOfColumns

	if uint64(len(cellProofs)) != expectedCellProofs {
		return errors.Errorf("expected %d cell proofs, got %d", expectedCellProofs, len(cellProofs))
	}

	if len(commitments) != len(blobs) {
		return errors.Errorf("number of commitments (%d) must match number of blobs (%d)", len(commitments), len(blobs))
	}

	if blobCount == 0 {
		return nil
	}

	// Handle multiple blobs - compute cells for all blobs
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
		for columnIndex := range numberOfColumns {
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
		return errors.Wrap(err, "cell batch verification")
	}
	if !valid {
		return errors.New("cell KZG proof batch verification failed")
	}

	return nil
}
