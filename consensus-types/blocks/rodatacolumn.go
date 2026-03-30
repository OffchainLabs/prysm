package blocks

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
)

var (
	errNilGloasDataColumn = errors.New("received nil gloas data column sidecar")
	errNotFuluDataColumn  = errors.New("data column sidecar is not a fulu type")
	errNotGloasDataColumn = errors.New("data column sidecar is not a gloas type")
)

// RODataColumn represents a read-only data column sidecar with its block root.
// It supports both Fulu and Gloas fork variants. Only one of fulu/gloas is non-nil.
type RODataColumn struct {
	fulu  *ethpb.DataColumnSidecar
	gloas *ethpb.DataColumnSidecarGloas
	root  [fieldparams.RootLength]byte
}

// NewRODataColumn creates a new RODataColumn from a Fulu DataColumnSidecar.
func NewRODataColumn(dc *ethpb.DataColumnSidecar) (RODataColumn, error) {
	if err := roDataColumnNilCheck(dc); err != nil {
		return RODataColumn{}, err
	}
	root, err := dc.SignedBlockHeader.Header.HashTreeRoot()
	if err != nil {
		return RODataColumn{}, err
	}
	return RODataColumn{fulu: dc, root: root}, nil
}

// NewRODataColumnWithRoot creates a new RODataColumn from a Fulu DataColumnSidecar with a given root.
func NewRODataColumnWithRoot(dc *ethpb.DataColumnSidecar, root [fieldparams.RootLength]byte) (RODataColumn, error) {
	if err := roDataColumnNilCheck(dc); err != nil {
		return RODataColumn{}, err
	}
	return RODataColumn{fulu: dc, root: root}, nil
}

// NewRODataColumnGloas creates a new RODataColumn from a Gloas DataColumnSidecarGloas.
func NewRODataColumnGloas(dc *ethpb.DataColumnSidecarGloas) (RODataColumn, error) {
	if dc == nil {
		return RODataColumn{}, errNilGloasDataColumn
	}
	root := bytesutil.ToBytes32(dc.BeaconBlockRoot)
	return RODataColumn{gloas: dc, root: root}, nil
}

// NewRODataColumnGloasWithRoot creates a new RODataColumn from a Gloas DataColumnSidecarGloas with a given root.
func NewRODataColumnGloasWithRoot(dc *ethpb.DataColumnSidecarGloas, root [fieldparams.RootLength]byte) (RODataColumn, error) {
	if dc == nil {
		return RODataColumn{}, errNilGloasDataColumn
	}
	return RODataColumn{gloas: dc, root: root}, nil
}

func roDataColumnNilCheck(dc *ethpb.DataColumnSidecar) error {
	if dc == nil {
		return errNilDataColumn
	}
	if dc.SignedBlockHeader == nil || dc.SignedBlockHeader.Header == nil {
		return errNilBlockHeader
	}
	if len(dc.SignedBlockHeader.Signature) == 0 {
		return errMissingBlockSignature
	}
	return nil
}

// IsGloas returns true if this data column is a Gloas fork variant.
func (dc *RODataColumn) IsGloas() bool {
	return dc.gloas != nil
}

// --- Common accessors (both forks) ---

// BlockRoot returns the root of the block.
func (dc *RODataColumn) BlockRoot() [fieldparams.RootLength]byte {
	return dc.root
}

// Slot returns the slot of the data column sidecar.
func (dc *RODataColumn) Slot() primitives.Slot {
	if dc.gloas != nil {
		return dc.gloas.Slot
	}
	return dc.fulu.SignedBlockHeader.Header.Slot
}

// Index returns the column index.
func (dc *RODataColumn) Index() uint64 {
	if dc.gloas != nil {
		return dc.gloas.Index
	}
	return dc.fulu.Index
}

// Column returns the column cell data.
func (dc *RODataColumn) Column() [][]byte {
	if dc.gloas != nil {
		return dc.gloas.Column
	}
	return dc.fulu.Column
}

// KzgProofs returns the KZG proofs.
func (dc *RODataColumn) KzgProofs() [][]byte {
	if dc.gloas != nil {
		return dc.gloas.KzgProofs
	}
	return dc.fulu.KzgProofs
}

// --- Fulu-only accessors ---

// ProposerIndex returns the proposer index. Only valid for Fulu sidecars.
func (dc *RODataColumn) ProposerIndex() primitives.ValidatorIndex {
	if dc.gloas != nil {
		return 0
	}
	return dc.fulu.SignedBlockHeader.Header.ProposerIndex
}

// ParentRoot returns the parent root. Only valid for Fulu sidecars.
func (dc *RODataColumn) ParentRoot() [fieldparams.RootLength]byte {
	if dc.gloas != nil {
		return [fieldparams.RootLength]byte{}
	}
	return bytesutil.ToBytes32(dc.fulu.SignedBlockHeader.Header.ParentRoot)
}

// SignedBlockHeader returns the signed block header. Only valid for Fulu sidecars.
// Returns nil for Gloas sidecars.
func (dc *RODataColumn) SignedBlockHeader() *ethpb.SignedBeaconBlockHeader {
	if dc.gloas != nil {
		return nil
	}
	return dc.fulu.SignedBlockHeader
}

// KzgCommitments returns the KZG commitments. Only valid for Fulu sidecars.
// Returns nil for Gloas sidecars (commitments come from the block's bid).
func (dc *RODataColumn) KzgCommitments() [][]byte {
	if dc.gloas != nil {
		return nil
	}
	return dc.fulu.KzgCommitments
}

// KzgCommitmentsInclusionProof returns the inclusion proof. Only valid for Fulu sidecars.
// Returns nil for Gloas sidecars.
func (dc *RODataColumn) KzgCommitmentsInclusionProof() [][]byte {
	if dc.gloas != nil {
		return nil
	}
	return dc.fulu.KzgCommitmentsInclusionProof
}

// SszMarshaler returns the underlying proto as an ssz.Marshaler.
// Works for both Fulu and Gloas sidecars.
func (dc *RODataColumn) SszMarshaler() ssz.Marshaler {
	if dc.gloas != nil {
		return dc.gloas
	}
	return dc.fulu
}

// SizeSSZ returns the SSZ encoded size of the underlying proto.
func (dc *RODataColumn) SizeSSZ() int {
	if dc.gloas != nil {
		return dc.gloas.SizeSSZ()
	}
	return dc.fulu.SizeSSZ()
}

// --- Proto access ---

// DataColumnSidecar returns the underlying Fulu proto, or nil if this is a Gloas sidecar.
func (dc *RODataColumn) DataColumnSidecar() *ethpb.DataColumnSidecar {
	return dc.fulu
}

// DataColumnSidecarGloas returns the underlying Gloas proto, or nil if this is a Fulu sidecar.
func (dc *RODataColumn) DataColumnSidecarGloas() *ethpb.DataColumnSidecarGloas {
	return dc.gloas
}

// VerifiedRODataColumn represents an RODataColumn that has undergone full verification (eg block sig, inclusion proof, commitment check).
type VerifiedRODataColumn struct {
	RODataColumn
}

// NewRODataColumnNoVerify creates an RODataColumn without validation. This should only be used in tests
// where intentionally malformed sidecars are needed to test error handling.
func NewRODataColumnNoVerify(dc *ethpb.DataColumnSidecar) RODataColumn {
	return RODataColumn{fulu: dc}
}

// NewVerifiedRODataColumn "upgrades" an RODataColumn to a VerifiedRODataColumn. This method should only be used by the verification package.
func NewVerifiedRODataColumn(roDataColumn RODataColumn) VerifiedRODataColumn {
	return VerifiedRODataColumn{RODataColumn: roDataColumn}
}
