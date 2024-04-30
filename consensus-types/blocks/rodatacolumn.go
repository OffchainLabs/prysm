package blocks

import (
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
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

// NewRODataColumnWithRoot creates a new RODataColumn with a given root.
// TODO: Add test
func NewRODataColumnWithRoot(dc *ethpb.DataColumnSidecar, root [fieldparams.RootLength]byte) (RODataColumn, error) {
	// Check if the data column is nil.
	if err := roDataColumnNilCheck(dc); err != nil {
		return RODataColumn{}, err
	}

	return RODataColumn{DataColumnSidecar: dc, root: root}, nil
}

// NewRODataColumn creates a new RODataColumn by computing the HashTreeRoot of the header.
// TODO: Add test
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

// BlockRoot returns the root of the block.
// TODO: Add test
func (dc *RODataColumn) BlockRoot() [fieldparams.RootLength]byte {
	return dc.root
}

// Slot returns the slot of the data column sidecar.
// TODO: Add test
func (dc *RODataColumn) Slot() primitives.Slot {
	return dc.SignedBlockHeader.Header.Slot
}

// ParentRoot returns the parent root of the data column sidecar.
// TODO: Add test
func (dc *RODataColumn) ParentRoot() [fieldparams.RootLength]byte {
	return bytesutil.ToBytes32(dc.SignedBlockHeader.Header.ParentRoot)
}

// ParentRootSlice returns the parent root of the data column sidecar as a byte slice.
// TODO: Add test
func (dc *RODataColumn) ParentRootSlice() []byte {
	return dc.SignedBlockHeader.Header.ParentRoot
}

// BodyRoot returns the body root of the data column sidecar.
// TODO: Add test
func (dc *RODataColumn) BodyRoot() [fieldparams.RootLength]byte {
	return bytesutil.ToBytes32(dc.SignedBlockHeader.Header.BodyRoot)
}

// ProposerIndex returns the proposer index of the data column sidecar.
// TODO: Add test
func (dc *RODataColumn) ProposerIndex() primitives.ValidatorIndex {
	return dc.SignedBlockHeader.Header.ProposerIndex
}

// BlockRootSlice returns the block root as a byte slice. This is often more convenient/concise
// than setting a tmp var to BlockRoot(), just so that it can be sliced.
// TODO: Add test
func (dc *RODataColumn) BlockRootSlice() []byte {
	return dc.root[:]
}

// RODataColumn is a custom type for a []RODataColumn, allowing methods to be defined that act on a slice of RODataColumn.
type RODataColumnSlice []RODataColumn

// Protos is a helper to make a more concise conversion from []RODataColumn->[]*ethpb.DataColumnSidecar.
func (s RODataColumnSlice) Protos() []*ethpb.DataColumnSidecar {
	pb := make([]*ethpb.DataColumnSidecar, len(s))
	for i := range s {
		pb[i] = s[i].DataColumnSidecar
	}
	return pb
}

// VerifiedRODataColumn represents an RODataColumn that has undergone full verification (eg block sig, inclusion proof, commitment check).
type VerifiedRODataColumn struct {
	RODataColumn
}

// NewVerifiedRODataColumn "upgrades" an RODataColumn to a VerifiedRODataColumn. This method should only be used by the verification package.
func NewVerifiedRODataColumn(rodc RODataColumn) VerifiedRODataColumn {
	return VerifiedRODataColumn{RODataColumn: rodc}
}
