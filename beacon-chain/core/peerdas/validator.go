package peerdas

import (
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	beaconState "github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

var (
	ErrNilSignedBlockOrEmptyCellsAndProofs = errors.New("nil signed block or empty cells and proofs")
	ErrSizeMismatch                        = errors.New("mismatch in the number of blob KZG commitments and cellsAndProofs")
	ErrNotEnoughDataColumnSidecars         = errors.New("not enough columns")
	ErrDataColumnSidecarsNotSortedByIndex  = errors.New("data column sidecars are not sorted by index")
)

var (
	_ ConstructionPopulator = (*BlockReconstructionSource)(nil)
	_ ConstructionPopulator = (*SidecarReconstructionSource)(nil)
	_ ConstructionPopulator = (*PartialDataColumnHeaderReconstructionSource)(nil)
)

const (
	BlockType                   = "BeaconBlock"
	SidecarType                 = "DataColumnSidecar"
	PartialDataColumnHeaderType = "PartialDataColumnHeader"
)

type (
	// ConstructionPopulator is an interface that can be satisfied by a type that can use data from a struct
	// like a DataColumnSidecar or a BeaconBlock to set the fields in a data column sidecar that cannot
	// be obtained from the engine api.
	ConstructionPopulator interface {
		Slot() primitives.Slot
		Root() [fieldparams.RootLength]byte
		ProposerIndex() primitives.ValidatorIndex
		Commitments() ([][]byte, error)
		Type() string

		extract() (*blockInfo, error)
	}

	// BlockReconstructionSource is a ConstructionPopulator that uses a beacon block as the source of data
	BlockReconstructionSource struct {
		blocks.ROBlock
	}

	// DataColumnSidecar is a ConstructionPopulator that uses a data column sidecar as the source of data
	SidecarReconstructionSource struct {
		blocks.VerifiedRODataColumn
	}

	PartialDataColumnHeaderReconstructionSource struct {
		*ethpb.PartialDataColumnHeader
		root [fieldparams.RootLength]byte
	}

	blockInfo struct {
		signedBlockHeader *ethpb.SignedBeaconBlockHeader
		kzgCommitments    [][]byte
		kzgInclusionProof [][]byte
	}
)

// PopulateFromBlock creates a BlockReconstructionSource from a beacon block
func PopulateFromBlock(block blocks.ROBlock) *BlockReconstructionSource {
	return &BlockReconstructionSource{ROBlock: block}
}

// PopulateFromSidecar creates a SidecarReconstructionSource from a data column sidecar
func PopulateFromSidecar(sidecar blocks.VerifiedRODataColumn) *SidecarReconstructionSource {
	return &SidecarReconstructionSource{VerifiedRODataColumn: sidecar}
}

// PopulateFromPartialHeader creates a PartialDataColumnHeaderReconstructionSource from a partial header.
// It eagerly computes and validates the hash tree root of the block header.
func PopulateFromPartialHeader(header *ethpb.PartialDataColumnHeader) (*PartialDataColumnHeaderReconstructionSource, error) {
	if header.SignedBlockHeader == nil || header.SignedBlockHeader.Header == nil {
		return nil, errors.New("nil signed block header or header")
	}
	root, err := header.SignedBlockHeader.Header.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "hash tree root")
	}
	return &PartialDataColumnHeaderReconstructionSource{PartialDataColumnHeader: header, root: root}, nil
}

// ValidatorsCustodyRequirement returns the number of custody groups regarding the validator indices attached to the beacon node.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#validator-custody
func ValidatorsCustodyRequirement(st beaconState.ReadOnlyBalances, validatorsIndex map[primitives.ValidatorIndex]bool) (uint64, error) {
	cfg := params.BeaconConfig()
	idxs := make([]primitives.ValidatorIndex, 0, len(validatorsIndex))
	for index := range validatorsIndex {
		idxs = append(idxs, index)
	}
	totalBalance, err := st.EffectiveBalanceSum(idxs)
	if err != nil {
		return 0, errors.Wrap(err, "effective balances")
	}

	numberOfCustodyGroups := cfg.NumberOfCustodyGroups
	validatorCustodyRequirement := cfg.ValidatorCustodyRequirement
	balancePerAdditionalCustodyGroup := cfg.BalancePerAdditionalCustodyGroup

	count := totalBalance / balancePerAdditionalCustodyGroup
	return min(max(count, validatorCustodyRequirement), numberOfCustodyGroups), nil
}

// DataColumnSidecars given ConstructionPopulator and the cells/proofs associated with each blob in the
// block, assembles sidecars which can be distributed to peers.
// cellsPerBlob and proofsPerBlob are parallel slices where each index represents a blob sidecar.
// This is an adapted version of
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#get_data_column_sidecars,
// which is designed to be used both when constructing sidecars from a block and from a sidecar, replacing
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#get_data_column_sidecars_from_block and
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#get_data_column_sidecars_from_column_sidecar
func DataColumnSidecars(cellsPerBlob [][]kzg.Cell, proofsPerBlob [][]kzg.Proof, src ConstructionPopulator) ([]blocks.RODataColumn, error) {
	const numberOfColumns = uint64(fieldparams.NumberOfColumns)

	if len(cellsPerBlob) == 0 {
		return nil, nil
	}
	start := time.Now()
	cells, proofs, err := rotateRowsToCols(cellsPerBlob, proofsPerBlob, numberOfColumns)
	if err != nil {
		return nil, errors.Wrap(err, "rotate cells and proofs")
	}
	info, err := src.extract()
	if err != nil {
		return nil, errors.Wrap(err, "extract block info")
	}

	roSidecars := make([]blocks.RODataColumn, 0, numberOfColumns)
	for idx := range numberOfColumns {
		sidecar := &ethpb.DataColumnSidecar{
			Index:                        idx,
			Column:                       cells[idx],
			KzgCommitments:               info.kzgCommitments,
			KzgProofs:                    proofs[idx],
			SignedBlockHeader:            info.signedBlockHeader,
			KzgCommitmentsInclusionProof: info.kzgInclusionProof,
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

func PartialColumns(included bitfield.Bitlist, cellsPerBlob [][]kzg.Cell, proofsPerBlob [][]kzg.Proof, src ConstructionPopulator) ([]blocks.PartialDataColumn, error) {
	start := time.Now()
	const numberOfColumns = uint64(fieldparams.NumberOfColumns)
	cells, proofs, err := rotateRowsToCols(cellsPerBlob, proofsPerBlob, numberOfColumns)
	if err != nil {
		return nil, errors.Wrap(err, "rotate cells and proofs")
	}
	info, err := src.extract()
	if err != nil {
		return nil, errors.Wrap(err, "extract block info")
	}

	dataColumns := make([]blocks.PartialDataColumn, 0, numberOfColumns)
	for idx := range numberOfColumns {
		dc, err := blocks.NewPartialDataColumn(src.Root(), info.signedBlockHeader, idx, info.kzgCommitments, info.kzgInclusionProof)
		if err != nil {
			return nil, errors.Wrap(err, "new ro data column")
		}

		for i := range len(info.kzgCommitments) {
			if !included.BitAt(uint64(i)) {
				continue
			}
			dc.ExtendFromVerifiedCell(uint64(i), cells[idx][0], proofs[idx][0])
			cells[idx] = cells[idx][1:]
			proofs[idx] = proofs[idx][1:]
		}
		dataColumns = append(dataColumns, dc)
	}

	dataColumnComputationTime.Observe(float64(time.Since(start).Milliseconds()))
	return dataColumns, nil
}

// Slot returns the slot of the source
func (s *BlockReconstructionSource) Slot() primitives.Slot {
	return s.Block().Slot()
}

// ProposerIndex returns the proposer index of the source
func (s *BlockReconstructionSource) ProposerIndex() primitives.ValidatorIndex {
	return s.Block().ProposerIndex()
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
	return BlockType
}

// extract extracts the block information from the source
func (b *BlockReconstructionSource) extract() (*blockInfo, error) {
	block := b.Block()

	header, err := b.Header()
	if err != nil {
		return nil, errors.Wrap(err, "header")
	}

	commitments, err := block.Body().BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "commitments")
	}

	inclusionProof, err := blocks.MerkleProofKZGCommitments(block.Body())
	if err != nil {
		return nil, errors.Wrap(err, "merkle proof kzg commitments")
	}

	info := &blockInfo{
		signedBlockHeader: header,
		kzgCommitments:    commitments,
		kzgInclusionProof: inclusionProof,
	}

	return info, nil
}

// rotateRowsToCols takes a 2D slice of cells and proofs, where the x is rows (blobs) and y is columns,
// and returns a 2D slice where x is columns and y is rows.
func rotateRowsToCols(cellsPerBlob [][]kzg.Cell, proofsPerBlob [][]kzg.Proof, numCols uint64) ([][][]byte, [][][]byte, error) {
	if len(cellsPerBlob) == 0 {
		return nil, nil, nil
	}
	if len(cellsPerBlob) != len(proofsPerBlob) {
		return nil, nil, errors.New("cells and proofs length mismatch")
	}
	cellCols := make([][][]byte, numCols)
	proofCols := make([][][]byte, numCols)
	for i := range cellsPerBlob {
		cells := cellsPerBlob[i]
		proofs := proofsPerBlob[i]
		if uint64(len(cells)) != numCols {
			return nil, nil, errors.Wrap(ErrNotEnoughDataColumnSidecars, "not enough cells")
		}
		if len(cells) != len(proofs) {
			return nil, nil, errors.Wrap(ErrNotEnoughDataColumnSidecars, "not enough proofs")
		}
		for j := range numCols {
			if i == 0 {
				cellCols[j] = make([][]byte, len(cellsPerBlob))
				proofCols[j] = make([][]byte, len(cellsPerBlob))
			}
			cellCols[j][i] = cells[j][:]
			proofCols[j][i] = proofs[j][:]
		}
	}
	return cellCols, proofCols, nil
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
	return SidecarType
}

// extract extracts the block information from the source
func (s *SidecarReconstructionSource) extract() (*blockInfo, error) {
	info := &blockInfo{
		signedBlockHeader: s.SignedBlockHeader,
		kzgCommitments:    s.KzgCommitments,
		kzgInclusionProof: s.KzgCommitmentsInclusionProof,
	}

	return info, nil
}

// Slot returns the slot from the partial data column header
func (p *PartialDataColumnHeaderReconstructionSource) Slot() primitives.Slot {
	return p.SignedBlockHeader.Header.Slot
}

// Root returns the block root computed from the header
func (p *PartialDataColumnHeaderReconstructionSource) Root() [fieldparams.RootLength]byte {
	return p.root
}

// ProposerIndex returns the proposer index from the header
func (p *PartialDataColumnHeaderReconstructionSource) ProposerIndex() primitives.ValidatorIndex {
	return p.SignedBlockHeader.Header.ProposerIndex
}

// Commitments returns the KZG commitments from the header
func (p *PartialDataColumnHeaderReconstructionSource) Commitments() ([][]byte, error) {
	return p.KzgCommitments, nil
}

// Type returns the type of the source
func (p *PartialDataColumnHeaderReconstructionSource) Type() string {
	return PartialDataColumnHeaderType
}

// extract extracts the block information from the partial header
func (p *PartialDataColumnHeaderReconstructionSource) extract() (*blockInfo, error) {
	info := &blockInfo{
		signedBlockHeader: p.SignedBlockHeader,
		kzgCommitments:    p.KzgCommitments,
		kzgInclusionProof: p.KzgCommitmentsInclusionProof,
	}

	return info, nil
}
