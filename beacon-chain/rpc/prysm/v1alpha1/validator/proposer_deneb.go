package validator

import (
	"errors"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// BuildBlobSidecars given a block, builds the blob sidecars for the block.
func BuildBlobSidecars(blk interfaces.SignedBeaconBlock, blobs [][]byte, kzgProofs [][]byte) ([]*ethpb.BlobSidecar, error) {
	if blk.Version() < version.Deneb {
		return nil, nil // No blobs before deneb.
	}
	commits, err := blk.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, err
	}
	cLen := len(commits)
	if cLen != len(blobs) || cLen != len(kzgProofs) {
		return nil, errors.New("blob KZG commitments don't match number of blobs or KZG proofs")
	}
	blobSidecars := make([]*ethpb.BlobSidecar, cLen)
	header, err := blk.Header()
	if err != nil {
		return nil, err
	}
	body := blk.Block().Body()
	for i := range blobSidecars {
		proof, err := blocks.MerkleProofKZGCommitment(body, i)
		if err != nil {
			return nil, err
		}
		blobSidecars[i] = &ethpb.BlobSidecar{
			Index:                    uint64(i),
			Blob:                     blobs[i],
			KzgCommitment:            commits[i],
			KzgProof:                 kzgProofs[i],
			SignedBlockHeader:        header,
			CommitmentInclusionProof: proof,
		}
	}
	return blobSidecars, nil
}
