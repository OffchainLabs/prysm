package validator

import (
	"bytes"

	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
)

func unblindBlobsSidecars(block interfaces.SignedBeaconBlock, bundle enginev1.BlobsBundler) ([]*ethpb.BlobSidecar, error) {
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

	kzgCommitments := bundle.GetKzgCommitments()
	blobs := bundle.GetBlobs()
	proofs := bundle.GetProofs()

	// Ensure there are equal counts of blobs/commitments/proofs.
	if len(kzgCommitments) != len(blobs) {
		return nil, errors.New("mismatch commitments count")
	}
	if len(proofs) != len(blobs) {
		return nil, errors.New("mismatch proofs count")
	}

	// Verify that commitments in the bundle match the block.
	if len(kzgCommitments) != len(blockCommitments) {
		return nil, errors.New("commitment count doesn't match block")
	}
	for i, commitment := range blockCommitments {
		if !bytes.Equal(kzgCommitments[i], commitment) {
			return nil, errors.New("commitment value doesn't match block")
		}
	}

	sidecars := make([]*ethpb.BlobSidecar, len(blobs))
	for i, b := range blobs {
		proof, err := consensusblocks.MerkleProofKZGCommitment(body, i)
		if err != nil {
			return nil, err
		}
		sidecars[i] = &ethpb.BlobSidecar{
			Index:                    uint64(i),
			Blob:                     bytesutil.SafeCopyBytes(b),
			KzgCommitment:            bytesutil.SafeCopyBytes(kzgCommitments[i]),
			KzgProof:                 bytesutil.SafeCopyBytes(proofs[i]),
			SignedBlockHeader:        header,
			CommitmentInclusionProof: proof,
		}
	}
	return sidecars, nil
}
