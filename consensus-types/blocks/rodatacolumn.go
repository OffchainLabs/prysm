package blocks

import (
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

// RODataColumn represents a read-only data column sidecar with its block root.
type RODataColumn struct {
	*ethpb.DataColumnSidecar
	root [fieldparams.RootLength]byte
}

func roDataColumnNilCheck(dc *ethpb.DataColumnSidecar) error {
	// Check if the data column is nil.
	if dc == nil {
		return errNilDataColumn
	}

	// Check if the data column header is nil.
	if dc.SignedBlockHeader == nil || dc.SignedBlockHeader.Header == nil {
		return errNilBlockHeader
	}

	// Check if the data column signature is nil.
	if len(dc.SignedBlockHeader.Signature) == 0 {
		return errMissingBlockSignature
	}

	return nil
}

// NewRODataColumn creates a new RODataColumn by computing the HashTreeRoot of the header.
func NewRODataColumn(dc *ethpb.DataColumnSidecar) (RODataColumn, error) {
	if err := roDataColumnNilCheck(dc); err != nil {
		return RODataColumn{}, err
	}

	root, err := dc.SignedBlockHeader.Header.HashTreeRoot()
	if err != nil {
		return RODataColumn{}, err
	}

	return RODataColumn{DataColumnSidecar: dc, root: root}, nil
}

// NewRODataColumnWithRoot creates a new RODataColumn with a given root.
func NewRODataColumnWithRoot(dc *ethpb.DataColumnSidecar, root [fieldparams.RootLength]byte) (RODataColumn, error) {
	// Check if the data column is nil.
	if err := roDataColumnNilCheck(dc); err != nil {
		return RODataColumn{}, err
	}

	return RODataColumn{DataColumnSidecar: dc, root: root}, nil
}

// BlockRoot returns the root of the block.
func (dc *RODataColumn) BlockRoot() [fieldparams.RootLength]byte {
	return dc.root
}

// Slot returns the slot of the data column sidecar.
func (dc *RODataColumn) Slot() primitives.Slot {
	return dc.SignedBlockHeader.Header.Slot
}

// ProposerIndex returns the proposer index of the data column sidecar.
func (dc *RODataColumn) ProposerIndex() primitives.ValidatorIndex {
	return dc.SignedBlockHeader.Header.ProposerIndex
}

// ParentRoot returns the parent root of the data column sidecar.
func (dc *RODataColumn) ParentRoot() [fieldparams.RootLength]byte {
	return bytesutil.ToBytes32(dc.SignedBlockHeader.Header.ParentRoot)
}

// VerifiedRODataColumn represents an RODataColumn that has undergone full verification (eg block sig, inclusion proof, commitment check).
type VerifiedRODataColumn struct {
	RODataColumn
}

// NewVerifiedRODataColumn "upgrades" an RODataColumn to a VerifiedRODataColumn. This method should only be used by the verification package.
func NewVerifiedRODataColumn(roDataColumn RODataColumn) VerifiedRODataColumn {
	return VerifiedRODataColumn{RODataColumn: roDataColumn}
}
