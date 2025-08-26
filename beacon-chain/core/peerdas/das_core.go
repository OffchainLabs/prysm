package peerdas

import (
	"encoding/binary"
	"math"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/crypto/hash"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
)

var (
	// Custom errors
	ErrCustodyGroupTooLarge               = errors.New("custody group too large")
	ErrCustodyGroupCountTooLarge          = errors.New("custody group count too large")
	ErrSizeMismatch                       = errors.New("mismatch in the number of blob KZG commitments and cellsAndProofs")
	ErrNotEnoughDataColumnSidecars        = errors.New("not enough columns")
	ErrDataColumnSidecarsNotSortedByIndex = errors.New("data column sidecars are not sorted by index")
	errWrongComputedCustodyGroupCount     = errors.New("wrong computed custody group count, should never happen")

	// maxUint256 is the maximum value of an uint256.
	maxUint256 = &uint256.Int{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64}
)

// CustodyGroups computes the custody groups the node should participate in for custody.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#get_custody_groups
func CustodyGroups(nodeId enode.ID, custodyGroupCount uint64) ([]uint64, error) {
	numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups

	// Check if the custody group count is larger than the number of custody groups.
	if custodyGroupCount > numberOfCustodyGroups {
		return nil, ErrCustodyGroupCountTooLarge
	}

	// Shortcut if all custody groups are needed.
	if custodyGroupCount == numberOfCustodyGroups {
		custodyGroups := make([]uint64, 0, numberOfCustodyGroups)
		for i := range numberOfCustodyGroups {
			custodyGroups = append(custodyGroups, i)
		}

		return custodyGroups, nil
	}

	one := uint256.NewInt(1)

	custodyGroupsMap := make(map[uint64]bool, custodyGroupCount)
	custodyGroups := make([]uint64, 0, custodyGroupCount)
	for currentId := new(uint256.Int).SetBytes(nodeId.Bytes()); uint64(len(custodyGroups)) < custodyGroupCount; {
		// Convert to big endian bytes.
		currentIdBytesBigEndian := currentId.Bytes32()

		// Convert to little endian.
		currentIdBytesLittleEndian := bytesutil.ReverseByteOrder(currentIdBytesBigEndian[:])

		// Hash the result.
		hashedCurrentId := hash.Hash(currentIdBytesLittleEndian)

		// Get the custody group ID.
		custodyGroup := binary.LittleEndian.Uint64(hashedCurrentId[:8]) % numberOfCustodyGroups

		// Add the custody group to the map.
		if !custodyGroupsMap[custodyGroup] {
			custodyGroupsMap[custodyGroup] = true
			custodyGroups = append(custodyGroups, custodyGroup)
		}

		if currentId.Cmp(maxUint256) == 0 {
			// Overflow prevention.
			currentId = uint256.NewInt(0)
		} else {
			// Increment the current ID.
			currentId.Add(currentId, one)
		}
	}

	// Final check.
	if uint64(len(custodyGroups)) != custodyGroupCount {
		return nil, errWrongComputedCustodyGroupCount
	}

	// Sort the custody groups.
	slices.Sort[[]uint64](custodyGroups)

	return custodyGroups, nil
}

// ComputeColumnsForCustodyGroup computes the columns for a given custody group.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#compute_columns_for_custody_group
func ComputeColumnsForCustodyGroup(custodyGroup uint64) ([]uint64, error) {
	beaconConfig := params.BeaconConfig()
	numberOfCustodyGroups := beaconConfig.NumberOfCustodyGroups

	if custodyGroup >= numberOfCustodyGroups {
		return nil, ErrCustodyGroupTooLarge
	}

	numberOfColumns := beaconConfig.NumberOfColumns

	columnsPerGroup := numberOfColumns / numberOfCustodyGroups

	columns := make([]uint64, 0, columnsPerGroup)
	for i := range columnsPerGroup {
		column := numberOfCustodyGroups*i + custodyGroup
		columns = append(columns, column)
	}

	return columns, nil
}

// DataColumnSidecars computes the data column sidecars from the signed block, cells and cell proofs.
// The returned value contains pointers to function parameters.
// (If the caller alterates `cellsAndProofs` afterwards, the returned value will be modified as well.)
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#get_data_column_sidecars_from_block
func DataColumnSidecars(signedBlock interfaces.ReadOnlySignedBeaconBlock, cellsAndProofs []kzg.CellsAndProofs) ([]*ethpb.DataColumnSidecar, error) {
	if signedBlock == nil || signedBlock.IsNil() || len(cellsAndProofs) == 0 {
		return nil, nil
	}

	block := signedBlock.Block()
	blockBody := block.Body()
	blobKzgCommitments, err := blockBody.BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "blob KZG commitments")
	}

	if len(blobKzgCommitments) != len(cellsAndProofs) {
		return nil, ErrSizeMismatch
	}

	signedBlockHeader, err := signedBlock.Header()
	if err != nil {
		return nil, errors.Wrap(err, "signed block header")
	}

	kzgCommitmentsInclusionProof, err := blocks.MerkleProofKZGCommitments(blockBody)
	if err != nil {
		return nil, errors.Wrap(err, "merkle proof KZG commitments")
	}

	dataColumnSidecars, err := dataColumnsSidecars(signedBlockHeader, blobKzgCommitments, kzgCommitmentsInclusionProof, cellsAndProofs)
	if err != nil {
		return nil, errors.Wrap(err, "data column sidecars")
	}

	return dataColumnSidecars, nil
}

// ComputeCustodyGroupForColumn computes the custody group for a given column.
// It is the reciprocal function of ComputeColumnsForCustodyGroup.
func ComputeCustodyGroupForColumn(columnIndex uint64) (uint64, error) {
	beaconConfig := params.BeaconConfig()
	numberOfColumns := beaconConfig.NumberOfColumns
	numberOfCustodyGroups := beaconConfig.NumberOfCustodyGroups

	if columnIndex >= numberOfColumns {
		return 0, ErrIndexTooLarge
	}

	return columnIndex % numberOfCustodyGroups, nil
}

// CustodyColumns computes the custody columns from the custody groups.
func CustodyColumns(custodyGroups []uint64) (map[uint64]bool, error) {
	numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups

	custodyGroupCount := len(custodyGroups)

	// Compute the columns for each custody group.
	columns := make(map[uint64]bool, custodyGroupCount)
	for _, group := range custodyGroups {
		if group >= numberOfCustodyGroups {
			return nil, ErrCustodyGroupTooLarge
		}

		groupColumns, err := ComputeColumnsForCustodyGroup(group)
		if err != nil {
			return nil, errors.Wrap(err, "compute columns for custody group")
		}

		for _, column := range groupColumns {
			columns[column] = true
		}
	}

	return columns, nil
}

// dataColumnsSidecars computes the data column sidecars from the signed block header, the blob KZG commiments,
// the KZG commitment includion proofs and cells and cell proofs.
// The returned value contains pointers to function parameters.
// (If the caller alterates input parameters afterwards, the returned value will be modified as well.)
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/validator.md#get_data_column_sidecars
func dataColumnsSidecars(
	signedBlockHeader *ethpb.SignedBeaconBlockHeader,
	blobKzgCommitments [][]byte,
	kzgCommitmentsInclusionProof [][]byte,
	cellsAndProofs []kzg.CellsAndProofs,
) ([]*ethpb.DataColumnSidecar, error) {
	start := time.Now()
	if len(blobKzgCommitments) != len(cellsAndProofs) {
		return nil, ErrSizeMismatch
	}

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	blobsCount := len(cellsAndProofs)
	sidecars := make([]*ethpb.DataColumnSidecar, 0, numberOfColumns)
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

		sidecars = append(sidecars, sidecar)
	}

	dataColumnComputationTime.Observe(float64(time.Since(start).Milliseconds()))
	return sidecars, nil
}
