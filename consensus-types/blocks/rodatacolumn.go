package blocks

import (
	"iter"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

var (
	errNilGloasDataColumn = errors.New("received nil gloas data column sidecar")
	errNotFuluDataColumn  = errors.New("data column sidecar is not a fulu type")
	errNotGloasDataColumn = errors.New("data column sidecar is not a gloas type")
)

// RODataColumn represents a read-only data column sidecar with its block root.
// It supports both Fulu and Gloas fork variants. Only one of fulu/gloas is non-nil.
type RODataColumn struct {
	fulu                *ethpb.DataColumnSidecar
	gloas               *ethpb.DataColumnSidecarGloas
	root                [fieldparams.RootLength]byte
	bidCommitmentsGloas [][]byte // KZG commitments from the block's execution payload bid.
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

// ProposerIndex returns the proposer index. Returns an error for Gloas sidecars.
func (dc *RODataColumn) ProposerIndex() (primitives.ValidatorIndex, error) {
	if dc.gloas != nil {
		return 0, errNotFuluDataColumn
	}
	return dc.fulu.SignedBlockHeader.Header.ProposerIndex, nil
}

// ParentRoot returns the parent root. Returns an error for Gloas sidecars.
func (dc *RODataColumn) ParentRoot() ([fieldparams.RootLength]byte, error) {
	if dc.gloas != nil {
		return [fieldparams.RootLength]byte{}, errNotFuluDataColumn
	}
	return bytesutil.ToBytes32(dc.fulu.SignedBlockHeader.Header.ParentRoot), nil
}

// SignedBlockHeader returns the signed block header. Returns an error for Gloas sidecars.
func (dc *RODataColumn) SignedBlockHeader() (*ethpb.SignedBeaconBlockHeader, error) {
	if dc.gloas != nil {
		return nil, errNotFuluDataColumn
	}
	return dc.fulu.SignedBlockHeader, nil
}

// KzgCommitments returns the KZG commitments.
// For Fulu these are in the sidecar. For Gloas these come from the block's bid
// and must be set via SetBidCommitments.
func (dc *RODataColumn) KzgCommitments() ([][]byte, error) {
	if dc.gloas != nil {
		if dc.bidCommitmentsGloas == nil {
			return nil, errNotFuluDataColumn
		}
		return dc.bidCommitmentsGloas, nil
	}
	return dc.fulu.KzgCommitments, nil
}

// SetBidCommitments sets the KZG commitments from the block's bid to be used for gloas.
func (dc *RODataColumn) SetBidCommitments(c [][]byte) {
	if dc.fulu != nil {
		dc.fulu.KzgCommitments = c
		return
	}
	dc.bidCommitmentsGloas = c
}

// KzgCommitmentsInclusionProof returns the inclusion proof. Returns an error for Gloas sidecars.
func (dc *RODataColumn) KzgCommitmentsInclusionProof() ([][]byte, error) {
	if dc.gloas != nil {
		return nil, errNotFuluDataColumn
	}
	return dc.fulu.KzgCommitmentsInclusionProof, nil
}

// MarshalSSZ marshals the underlying proto to SSZ bytes.
// Works for both Fulu and Gloas sidecars.
func (dc *RODataColumn) MarshalSSZ() ([]byte, error) {
	if dc.gloas != nil {
		return dc.gloas.MarshalSSZ()
	}
	return dc.fulu.MarshalSSZ()
}

// MarshalSSZTo marshals the underlying proto to the provided byte slice.
func (dc *RODataColumn) MarshalSSZTo(buf []byte) ([]byte, error) {
	if dc.gloas != nil {
		return dc.gloas.MarshalSSZTo(buf)
	}
	return dc.fulu.MarshalSSZTo(buf)
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

type CellProofBundleWithSize struct {
	Size     int
	Iterator iter.Seq[CellProofBundle]
}

func RODataColumnsToCellProofBundlesWithSize(sidecars ...RODataColumn) CellProofBundleWithSize {
	if len(sidecars) == 0 {
		return CellProofBundleWithSize{Size: 0, Iterator: func(_ func(CellProofBundle) bool) {}}
	}
	return CellProofBundleWithSize{
		Size:     len(sidecars) * len(sidecars[0].Column()), // assuming all sidecars have the same number of cells
		Iterator: RODataColumnsToCellProofBundles(sidecars),
	}
}

func RODataColumnsToCellProofBundles(sidecars []RODataColumn) iter.Seq[CellProofBundle] {
	return func(yield func(CellProofBundle) bool) {
		for _, sidecar := range sidecars {
			cells := sidecar.Column()
			kcs, err := sidecar.KzgCommitments()
			if err != nil {
				log.WithError(err).Error("kzg commitments not present on sidecar")
				return
			}
			kps := sidecar.KzgProofs()
			for i := range cells {
				if !yield(CellProofBundle{
					ColumnIndex: sidecar.Index(),
					Commitment:  kcs[i],
					Cell:        cells[i],
					Proof:       kps[i],
				}) {
					return
				}
			}
		}
	}
}
