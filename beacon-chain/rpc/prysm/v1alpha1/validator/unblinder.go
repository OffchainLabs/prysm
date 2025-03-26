package validator

import (
	"bytes"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	consensusblocks "github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

func unblindBlobsSidecars(block interfaces.SignedBeaconBlock, bundle blocks.BlobsBundle) ([]*ethpb.BlobSidecar, error) {
	if block.Version() < version.Deneb {
		return nil, nil
	}
	body := block.Block().Body()
	blockCommitments, err := body.BlobKzgCommitments()
	if err != nil {
		return nil, err
	}
	if len(blockCommitments) == 0 {
		return nil, nil
	}
	// Do not allow builders to provide no blob bundles for blocks which carry commitments.
	if bundle == nil {
		return nil, errors.New("no valid bundle provided")
	}
	header, err := block.Header()
	if err != nil {
		return nil, err
	}

	// Ensure there are equal counts of blobs/commitments/proofs.
	if len(bundle.GetKzgCommitments()) != len(bundle.GetBlobs()) {
		return nil, errors.New("mismatch commitments count")
	}
	if len(bundle.GetProofs()) != len(bundle.GetBlobs()) {
		return nil, errors.New("mismatch proofs count")
	}

	// Verify that commitments in the bundle match the block.
	if len(bundle.GetKzgCommitments()) != len(blockCommitments) {
		return nil, errors.New("commitment count doesn't match block")
	}
	for i, commitment := range blockCommitments {
		if !bytes.Equal(bundle.GetKzgCommitments()[i], commitment) {
			return nil, errors.New("commitment value doesn't match block")
		}
	}

	sidecars := make([]*ethpb.BlobSidecar, len(bundle.GetBlobs()))
	for i, b := range bundle.GetBlobs() {
		proof, err := consensusblocks.MerkleProofKZGCommitment(body, i)
		if err != nil {
			return nil, err
		}
		sidecars[i] = &ethpb.BlobSidecar{
			Index:                    uint64(i),
			Blob:                     bytesutil.SafeCopyBytes(b),
			KzgCommitment:            bytesutil.SafeCopyBytes(bundle.GetKzgCommitments()[i]),
			KzgProof:                 bytesutil.SafeCopyBytes(bundle.GetProofs()[i]),
			SignedBlockHeader:        header,
			CommitmentInclusionProof: proof,
		}
	}
	return sidecars, nil
}
