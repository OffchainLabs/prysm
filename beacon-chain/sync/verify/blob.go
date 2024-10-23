package verify

import (
	"reflect"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

var (
	errBlobVerification   = errors.New("unable to verify blobs")
	errColumnVerification = errors.New("unable to verify column")

	ErrIncorrectBlobIndex   = errors.New("incorrect blob index")
	ErrIncorrectColumnIndex = errors.New("incorrect column index")

	ErrBlobBlockMisaligned   = errors.Wrap(errBlobVerification, "root of block header in blob sidecar does not match block root")
	ErrColumnBlockMisaligned = errors.Wrap(errColumnVerification, "root of block header in column sidecar does not match block root")

	ErrMismatchedBlobCommitments   = errors.Wrap(errBlobVerification, "commitments at given slot, root and index do not match")
	ErrMismatchedColumnCommitments = errors.Wrap(errColumnVerification, "commitments at given slot, root and index do not match")
)

// BlobAlignsWithBlock verifies if the blob aligns with the block.
func BlobAlignsWithBlock(blob blocks.ROBlob, block blocks.ROBlock) error {
	if block.Version() < version.Deneb {
		return nil
	}
	if blob.Index >= fieldparams.MaxBlobsPerBlock {
		return errors.Wrapf(ErrIncorrectBlobIndex, "index %d exceeds MAX_BLOBS_PER_BLOCK %d", blob.Index, fieldparams.MaxBlobsPerBlock)
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

type WrappedBlockDataColumn struct {
	ROBlock      interfaces.ReadOnlyBeaconBlock
	RODataColumn blocks.RODataColumn
}

func DataColumnsAlignWithBlock(
	wrappedBlockDataColumns []WrappedBlockDataColumn,
	dataColumnsVerifier verification.NewDataColumnsVerifier,
) error {
	for _, wrappedBlockDataColumn := range wrappedBlockDataColumns {
		dataColumn := wrappedBlockDataColumn.RODataColumn
		block := wrappedBlockDataColumn.ROBlock

		// Extract the block root from the data column.
		blockRoot := dataColumn.BlockRoot()

		// Retrieve the KZG commitments from the block.
		blockKZGCommitments, err := block.Body().BlobKzgCommitments()
		if err != nil {
			return errors.Wrap(err, "blob KZG commitments")
		}

		// Retrieve the KZG commitments from the data column.
		dataColumnKZGCommitments := dataColumn.KzgCommitments

		// Verify the commitments in the block match the commitments in the data column.
		if !reflect.DeepEqual(blockKZGCommitments, dataColumnKZGCommitments) {
			// Retrieve the data columns slot.
			dataColumSlot := dataColumn.Slot()

			return errors.Wrapf(
				ErrMismatchedColumnCommitments,
				"data column commitments `%#v` != block commitments `%#v` for block root %#x at slot %d",
				dataColumnKZGCommitments,
				blockKZGCommitments,
				blockRoot,
				dataColumSlot,
			)
		}
	}

	dataColumns := make([]blocks.RODataColumn, 0, len(wrappedBlockDataColumns))
	for _, wrappedBlowrappedBlockDataColumn := range wrappedBlockDataColumns {
		dataColumn := wrappedBlowrappedBlockDataColumn.RODataColumn
		dataColumns = append(dataColumns, dataColumn)
	}

	// Verify if data columns index are in bounds.
	verifier := dataColumnsVerifier(dataColumns, verification.InitsyncColumnSidecarRequirements)
	if err := verifier.DataColumnsIndexInBounds(); err != nil {
		return errors.Wrap(err, "data column index in bounds")
	}

	// Verify the KZG inclusion proof verification.
	if err := verifier.SidecarInclusionProven(); err != nil {
		return errors.Wrap(err, "inclusion proof verification")
	}

	// Verify the KZG proof verification.
	if err := verifier.SidecarKzgProofVerified(); err != nil {
		return errors.Wrap(err, "KZG proof verification")
	}

	return nil
}
