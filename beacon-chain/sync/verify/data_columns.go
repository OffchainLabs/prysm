package verify

import (
	"reflect"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
)

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
