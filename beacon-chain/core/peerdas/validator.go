package peerdas

import (
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	beaconState "github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

var (
	ErrNilSignedBlockOrEmptyCellsAndProofs = errors.New("nil signed block or empty cells and proofs")
	ErrSizeMismatch                        = errors.New("mismatch in the number of blob KZG commitments and cellsAndProofs")
	ErrNotEnoughDataColumnSidecars         = errors.New("not enough columns")
	ErrDataColumnSidecarsNotSortedByIndex  = errors.New("data column sidecars are not sorted by index")
)

var (
	_ ConstructionPopulator = (*SidecarReconstructionSource)(nil)
	_ ConstructionPopulator = (*BlockReconstructionSource)(nil)
)

type (
	// ConstructionPopulator is an interface that can be satisfied by a type that can use data from a struct
	// like a DataColumnSidecar or a BeaconBlock to set the fields in a data column sidecar that cannot
	// be obtained from the engine api.
	ConstructionPopulator interface {
		Populate(*ethpb.DataColumnSidecar) error
		Slot() primitives.Slot
		Root() [fieldparams.RootLength]byte
		Commitments() ([][]byte, error)
		Type() string
	}

	// BlockReconstructionSource is a ConstructionPopulator that uses a beacon block as the source of data
	BlockReconstructionSource struct {
		blocks.ROBlock
	}

	// DataColumnSidecar is a ConstructionPopulator that uses a data column sidecar as the source of data
	SidecarReconstructionSource struct {
		blocks.RODataColumn
	}
)

// PopulateFromBlock creates a BlockReconstructionSource from a beacon block
func PopulateFromBlock(block blocks.ROBlock) *BlockReconstructionSource {
	return &BlockReconstructionSource{ROBlock: block}
}

// PopulateFromSidecar creates a SidecarReconstructionSource from a data column sidecar
func PopulateFromSidecar(sidecar blocks.RODataColumn) *SidecarReconstructionSource {
	return &SidecarReconstructionSource{RODataColumn: sidecar}
}

// ValidatorsCustodyRequirement returns the number of custody groups regarding the validator indices attached to the beacon node.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#validator-custody
func ValidatorsCustodyRequirement(state beaconState.ReadOnlyBeaconState, validatorsIndex map[primitives.ValidatorIndex]bool) (uint64, error) {
	totalNodeBalance := uint64(0)
	for index := range validatorsIndex {
		validator, err := state.ValidatorAtIndexReadOnly(index)
		if err != nil {
			return 0, errors.Wrapf(err, "validator at index %v", index)
		}

		totalNodeBalance += validator.EffectiveBalance()
	}

	beaconConfig := params.BeaconConfig()
	numberOfCustodyGroups := beaconConfig.NumberOfCustodyGroups
	validatorCustodyRequirement := beaconConfig.ValidatorCustodyRequirement
	balancePerAdditionalCustodyGroup := beaconConfig.BalancePerAdditionalCustodyGroup

	count := totalNodeBalance / balancePerAdditionalCustodyGroup
	return min(max(count, validatorCustodyRequirement), numberOfCustodyGroups), nil
}

// DataColumnSidecars, given ConstructionPopulator and the cells/proofs associated with each blob in the
// block, assembles sidecars which can be distributed to peers.
// This is an adapted version of
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#get_data_column_sidecars,
// which is designed to be used both when constructing sidecars from a block and from a sidecar, replacing
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#get_data_column_sidecars_from_block and
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#get_data_column_sidecars_from_column_sidecar
func DataColumnSidecars(rows []kzg.CellsAndProofs, src ConstructionPopulator) ([]blocks.RODataColumn, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	start := time.Now()
	cells, proofs, err := rotateRowsToCols(rows, params.BeaconConfig().NumberOfColumns)
	if err != nil {
		return nil, errors.Wrap(err, "rotate cells and proofs")
	}

	maxIdx := params.BeaconConfig().NumberOfColumns
	roSidecars := make([]blocks.RODataColumn, 0, maxIdx)
	for idx := range maxIdx {
		sidecar := &ethpb.DataColumnSidecar{
			Index:     idx,
			Column:    cells[idx],
			KzgProofs: proofs[idx],
		}
		if err := src.Populate(sidecar); err != nil {
			return nil, errors.Wrap(err, "column field setter set")
		}
		if len(sidecar.KzgCommitments) != len(sidecar.Column) || len(sidecar.KzgCommitments) != len(sidecar.KzgProofs) {
			return nil, ErrSizeMismatch
		}

		roSidecar, err := blocks.NewRODataColumnWithRoot(sidecar, src.Root())
		if err != nil {
			return nil, errors.Wrap(err, "new ro data column")
		}
		roSidecars = append(roSidecars, roSidecar)
	}

	dataColumnComputationTime.Observe(float64(time.Since(start).Milliseconds()))
	return roSidecars, nil
}

func (b *BlockReconstructionSource) Populate(dc *ethpb.DataColumnSidecar) error {
	block := b.Block()
	blockBody := block.Body()
	blobKzgCommitments, err := blockBody.BlobKzgCommitments()
	if err != nil {
		return errors.Wrap(err, "blob KZG commitments")
	}
	dc.KzgCommitments = blobKzgCommitments
	dc.SignedBlockHeader, err = b.Header()
	if err != nil {
		return errors.Wrap(err, "signed block header")
	}
	dc.KzgCommitmentsInclusionProof, err = blocks.MerkleProofKZGCommitments(blockBody)
	if err != nil {
		return errors.Wrap(err, "merkle proof KZG commitments")
	}
	return nil
}

// Slot returns the slot of the source
func (s *BlockReconstructionSource) Slot() primitives.Slot {
	return s.Block().Slot()
}

// Commitments returns the blob KZG commitments of the source
func (s *BlockReconstructionSource) Commitments() ([][]byte, error) {
	c, err := s.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "blob KZG commitments")
	}

	return c, nil
}

// Type returns the type of the source
func (s *BlockReconstructionSource) Type() string {
	return "BeaconBlock"
}

// rotateRowsToCols takes a 2D slice of cells and proofs, where the x is rows (blobs) and y is columns,
// and returns a 2D slice where x is columns and y is rows.
func rotateRowsToCols(rows []kzg.CellsAndProofs, numCols uint64) ([][][]byte, [][][]byte, error) {
	if len(rows) == 0 {
		return nil, nil, nil
	}
	cellCols := make([][][]byte, numCols)
	proofCols := make([][][]byte, numCols)
	for i, cp := range rows {
		if uint64(len(cp.Cells)) != numCols {
			return nil, nil, errors.Wrap(ErrNotEnoughDataColumnSidecars, "not enough cells")
		}
		if len(cp.Cells) != len(cp.Proofs) {
			return nil, nil, errors.Wrap(ErrNotEnoughDataColumnSidecars, "not enough proofs")
		}
		for j := uint64(0); j < numCols; j++ {
			if i == 0 {
				cellCols[j] = make([][]byte, len(rows))
				proofCols[j] = make([][]byte, len(rows))
			}
			cellCols[j][i] = cp.Cells[j][:]
			proofCols[j][i] = cp.Proofs[j][:]
		}
	}
	return cellCols, proofCols, nil
}

func (s *SidecarReconstructionSource) Populate(dc *ethpb.DataColumnSidecar) error {
	dc.SignedBlockHeader = s.SignedBlockHeader
	dc.KzgCommitments = s.KzgCommitments
	dc.KzgCommitmentsInclusionProof = s.KzgCommitmentsInclusionProof
	return nil
}

// Root returns the block root of the source
func (s *SidecarReconstructionSource) Root() [fieldparams.RootLength]byte {
	return s.BlockRoot()
}

// Commmitments returns the blob KZG commitments of the source
func (s *SidecarReconstructionSource) Commitments() ([][]byte, error) {
	return s.KzgCommitments, nil
}

// Type returns the type of the source
func (s *SidecarReconstructionSource) Type() string {
	return "DataColumnSidecar"
}
