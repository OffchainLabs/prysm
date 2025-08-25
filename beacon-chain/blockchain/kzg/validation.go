package kzg

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
)

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
