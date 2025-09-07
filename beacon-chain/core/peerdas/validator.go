package peerdas

import (
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	beaconState "github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
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

// DataColumnSidecarsFromBlock, given a signed block and the cells/proofs associated with each blob in the
// block, assemble the sidecars which can be distributed to peers.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#get_data_column_sidecars_from_block
func DataColumnSidecarsFromBlock(signedBlock interfaces.ReadOnlySignedBeaconBlock, cellsAndKzgProofs []kzg.CellsAndProofs) ([]blocks.RODataColumn, error) {
	if signedBlock == nil || signedBlock.IsNil() {
		return nil, ErrNilSignedBlockOrEmptyCellsAndProofs
	}

	block := signedBlock.Block()
	blockBody := block.Body()
	blobKzgCommitments, err := blockBody.BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "blob KZG commitments")
	}

	signedBlockHeader, err := signedBlock.Header()
	if err != nil {
		return nil, errors.Wrap(err, "signed block header")
	}

	kzgCommitmentsInclusionProof, err := blocks.MerkleProofKZGCommitments(blockBody)
	if err != nil {
		return nil, errors.Wrap(err, "merkle proof KZG commitments")
	}

	dataColumnSidecars, err := dataColumnSidecars(signedBlockHeader, blobKzgCommitments, kzgCommitmentsInclusionProof, cellsAndKzgProofs)
	if err != nil {
		return nil, errors.Wrap(err, "data column sidecars")
	}

	return dataColumnSidecars, nil
}

// DataColumnSidecarsFromColumnSidecar, given a DataColumnSidecar and the cells/proofs associated with each blob corresponding
// to the commitments it contains, assemble all sidecars for distribution to peers.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#get_data_column_sidecars_from_column_sidecar
func DataColumnSidecarsFromColumnSidecar(sidecar blocks.VerifiedRODataColumn, cellsAndKzgProofs []kzg.CellsAndProofs) ([]blocks.RODataColumn, error) {
	dataColumnSidecars, err := dataColumnSidecars(sidecar.SignedBlockHeader, sidecar.KzgCommitments, sidecar.KzgCommitmentsInclusionProof, cellsAndKzgProofs)
	if err != nil {
		return nil, errors.Wrap(err, "data column sidecars")
	}

	return dataColumnSidecars, nil
}

// dataColumnSidecars, given a signed block header and the commitments, inclusion proof, cells/proofs associated with
// each blob in the block, assemble the sidecars which can be distributed to peers.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#get_data_column_sidecars
func dataColumnSidecars(
	signedBlockHeader *ethpb.SignedBeaconBlockHeader,
	blobKzgCommitments [][]byte,
	kzgCommitmentsInclusionProof [][]byte,
	cellsAndProofs []kzg.CellsAndProofs,
) ([]blocks.RODataColumn, error) {
	start := time.Now()
	if len(cellsAndProofs) != len(blobKzgCommitments) {
		return nil, ErrSizeMismatch
	}

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	blobsCount := len(cellsAndProofs)
	roSidecars := make([]blocks.RODataColumn, 0, numberOfColumns)
	for columnIndex := range numberOfColumns {
		column := make([]kzg.Cell, 0, blobsCount)
		kzgProofOfColumn := make([]kzg.Proof, 0, blobsCount)

		for rowIndex := range blobsCount {
			cellsForRow := cellsAndProofs[rowIndex].Cells
			proofsForRow := cellsAndProofs[rowIndex].Proofs

			// Validate that we have enough cells and proofs for this column index
			if columnIndex >= uint64(len(cellsForRow)) {
				return nil, errors.Errorf("column index %d exceeds cells length %d for blob %d", columnIndex, len(cellsForRow), rowIndex)
			}
			if columnIndex >= uint64(len(proofsForRow)) {
				return nil, errors.Errorf("column index %d exceeds proofs length %d for blob %d", columnIndex, len(proofsForRow), rowIndex)
			}

			cell := cellsForRow[columnIndex]
			column = append(column, cell)

			kzgProof := proofsForRow[columnIndex]
			kzgProofOfColumn = append(kzgProofOfColumn, kzgProof)
		}

		columnBytes := make([][]byte, 0, blobsCount)
		for i := range column {
			columnBytes = append(columnBytes, column[i][:])
		}

		kzgProofOfColumnBytes := make([][]byte, 0, blobsCount)
		for _, kzgProof := range kzgProofOfColumn {
			kzgProofOfColumnBytes = append(kzgProofOfColumnBytes, kzgProof[:])
		}

		sidecar := &ethpb.DataColumnSidecar{
			Index:                        columnIndex,
			Column:                       columnBytes,
			KzgCommitments:               blobKzgCommitments,
			KzgProofs:                    kzgProofOfColumnBytes,
			SignedBlockHeader:            signedBlockHeader,
			KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
		}

		roSidecar, err := blocks.NewRODataColumn(sidecar)
		if err != nil {
			return nil, errors.Wrap(err, "new ro data column")
		}

		roSidecars = append(roSidecars, roSidecar)
	}

	dataColumnComputationTime.Observe(float64(time.Since(start).Milliseconds()))
	return roSidecars, nil
}
