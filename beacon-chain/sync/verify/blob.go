package verify

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
)

var (
	errBlobVerification          = errors.New("unable to verify blobs")
	ErrIncorrectBlobIndex        = errors.New("incorrect blob index")
	ErrBlobBlockMisaligned       = errors.Wrap(errBlobVerification, "root of block header in blob sidecar does not match block root")
	ErrMismatchedBlobCommitments = errors.Wrap(errBlobVerification, "commitments at given slot, root and index do not match")
)

// BlobAlignsWithBlock verifies if the blob aligns with the block.
func BlobAlignsWithBlock(blob blocks.ROBlob, block blocks.ROBlock) error {
	blockVersion := block.Version()

	if blockVersion < version.Deneb || blockVersion >= version.Fulu {
		return nil
	}

	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(blob.Slot())
	if blob.Index >= uint64(maxBlobsPerBlock) {
		return errors.Wrapf(ErrIncorrectBlobIndex, "index %d exceeds MAX_BLOBS_PER_BLOCK %d", blob.Index, maxBlobsPerBlock)
	}

	if blob.BlockRoot() != block.Root() {
		return ErrBlobBlockMisaligned
	}

	// Verify commitment byte values match
	// TODO: verify commitment inclusion proof - actually replace this with a better rpc blob verification stack altogether.
	commits, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		return err
	}
	blockCommitment := bytesutil.ToBytes48(commits[blob.Index])
	blobCommitment := bytesutil.ToBytes48(blob.KzgCommitment)
	if blobCommitment != blockCommitment {
		return errors.Wrapf(ErrMismatchedBlobCommitments, "commitment %#x != block commitment %#x, at index %d for block root %#x at slot %d ", blobCommitment, blockCommitment, blob.Index, block.Root(), blob.Slot())
	}
	return nil
}
