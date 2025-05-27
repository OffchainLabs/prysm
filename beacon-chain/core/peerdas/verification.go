package peerdas

import (
	"bytes"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
)

var (
	ErrBlockColumnSizeMismatch = errors.New("size mismatch between data column and block")
	ErrTooManyCommitments      = errors.New("too many commitments")
	ErrRootMismatch            = errors.New("root mismatch between data column and block")
	ErrCommitmentMismatch      = errors.New("commitment mismatch between data column and block")
)

// DataColumnsAlignWithBlock checks if the data columns align with the block.
func DataColumnsAlignWithBlock(block blocks.ROBlock, dataColumns []blocks.RODataColumn) error {
	// No data columns before Fulu.
	if block.Version() < version.Fulu {
		return nil
	}

	// Compute the maximum number of blobs per block.
	blockSlot := block.Block().Slot()
	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(blockSlot)

	// Check if the block has not too many commitments.
	blockCommitments, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		return errors.Wrap(err, "blob KZG commitments")
	}

	blockCommitmentCount := len(blockCommitments)
	if blockCommitmentCount > maxBlobsPerBlock {
		return ErrTooManyCommitments
	}

	blockRoot := block.Root()

	for _, dataColumn := range dataColumns {
		// Check if the root of the data column sidecar matches the block root.
		if dataColumn.BlockRoot() != blockRoot {
			return ErrRootMismatch
		}

		// Check if the content length of the data column sidecar matches the block.
		if len(dataColumn.Column) != blockCommitmentCount ||
			len(dataColumn.KzgCommitments) != blockCommitmentCount ||
			len(dataColumn.KzgProofs) != blockCommitmentCount {
			return ErrBlockColumnSizeMismatch
		}

		// Check if the commitments of the data column sidecar match the block.
		for i := range blockCommitments {
			if !bytes.Equal(blockCommitments[i], dataColumn.KzgCommitments[i]) {
				return ErrCommitmentMismatch
			}
		}
	}

	return nil
}
