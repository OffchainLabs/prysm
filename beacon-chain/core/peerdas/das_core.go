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
	ErrCustodyGroupTooLarge           = errors.New("custody group too large")
	ErrCustodyGroupCountTooLarge      = errors.New("custody group count too large")
	ErrMismatchSize                   = errors.New("mismatch in the number of blob KZG commitments and cellsAndProofs")
	errWrongComputedCustodyGroupCount = errors.New("wrong computed custody group count, should never happen")

	// maxUint256 is the maximum value of an uint256.
	maxUint256 = &uint256.Int{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64}
)

type CustodyType int

const (
	Target CustodyType = iota
	Actual
)

// CustodyGroups computes the custody groups the node should participate in for custody.
// https://github.com/ethereum/consensus-specs/blob/v1.5.0-beta.5/specs/fulu/das-core.md#get_custody_groups
func CustodyGroups(nodeId enode.ID, custodyGroupCount uint64) ([]uint64, error) {
	numberOfCustodyGroup := params.BeaconConfig().NumberOfCustodyGroups

	// Check if the custody group count is larger than the number of custody groups.
	if custodyGroupCount > numberOfCustodyGroup {
		return nil, ErrCustodyGroupCountTooLarge
	}

	// Shortcut if all custody groups are needed.
	if custodyGroupCount == numberOfCustodyGroup {
		custodyGroups := make([]uint64, 0, numberOfCustodyGroup)
		for i := range numberOfCustodyGroup {
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
		custodyGroup := binary.LittleEndian.Uint64(hashedCurrentId[:8]) % numberOfCustodyGroup

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

		// Sort the custody groups.
		slices.Sort[[]uint64](custodyGroups)
	}

	// Final check.
	if uint64(len(custodyGroups)) != custodyGroupCount {
		return nil, errWrongComputedCustodyGroupCount
	}

	return custodyGroups, nil
}

// ComputeColumnsForCustodyGroup computes the columns for a given custody group.
// https://github.com/ethereum/consensus-specs/blob/v1.5.0-beta.5/specs/fulu/das-core.md#compute_columns_for_custody_group
func ComputeColumnsForCustodyGroup(custodyGroup uint64) ([]uint64, error) {
	beaconConfig := params.BeaconConfig()
	numberOfCustodyGroup := beaconConfig.NumberOfCustodyGroups

	if custodyGroup >= numberOfCustodyGroup {
		return nil, ErrCustodyGroupTooLarge
	}

	numberOfColumns := beaconConfig.NumberOfColumns

	columnsPerGroup := numberOfColumns / numberOfCustodyGroup

	columns := make([]uint64, 0, columnsPerGroup)
	for i := range columnsPerGroup {
		column := numberOfCustodyGroup*i + custodyGroup
		columns = append(columns, column)
	}

	return columns, nil
}

// DataColumnSidecars computes the data column sidecars from the signed block, cells and cell proofs.
// The returned value contains pointers to function parameters.
// (If the caller alterates `cellsAndProofs` afterwards, the returned value will be modified as well.)
// https://github.com/ethereum/consensus-specs/blob/v1.5.0-beta.3/specs/fulu/das-core.md#get_data_column_sidecars
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
		return nil, ErrMismatchSize
	}

	signedBlockHeader, err := signedBlock.Header()
	if err != nil {
		return nil, errors.Wrap(err, "signed block header")
	}

	kzgCommitmentsInclusionProof, err := blocks.MerkleProofKZGCommitments(blockBody)
	if err != nil {
		return nil, errors.Wrap(err, "merkle proof ZKG commitments")
	}

	dataColumnSidecars, err := DataColumnsSidecarsFromItems(signedBlockHeader, blobKzgCommitments, kzgCommitmentsInclusionProof, cellsAndProofs)
	if err != nil {
		return nil, errors.Wrap(err, "data column sidecars from items")
	}

	return dataColumnSidecars, nil
}

// DataColumnsSidecarsFromItems computes the data column sidecars from the signed block header, the blob KZG commiments,
// the KZG commitment includion proofs and cells and cell proofs.
// The returned value contains pointers to function parameters.
// (If the caller alterates input parameters afterwards, the returned value will be modified as well.)
func DataColumnsSidecarsFromItems(
	signedBlockHeader *ethpb.SignedBeaconBlockHeader,
	blobKzgCommitments [][]byte,
	kzgCommitmentsInclusionProof [][]byte,
	cellsAndProofs []kzg.CellsAndProofs,
) ([]*ethpb.DataColumnSidecar, error) {
	start := time.Now()
	if len(blobKzgCommitments) != len(cellsAndProofs) {
		return nil, ErrMismatchSize
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

// Blobs extract blobs from `dataColumnsSidecar`.
// This can be seen as the reciprocal function of DataColumnSidecars.
// `dataColumnsSidecar` needs to contain the datacolumns corresponding to the non-extended matrix,
// else an error will be returned.
// (`dataColumnsSidecar` can contain extra columns, but they will be ignored.)
func Blobs(indices map[uint64]bool, dataColumnsSidecar []*ethpb.DataColumnSidecar) ([]*blocks.VerifiedROBlob, error) {
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	// Compute the number of needed columns, including the number of columns is odd case.
	neededColumnCount := (numberOfColumns + 1) / 2

	// Check if all needed columns are present.
	sliceIndexFromColumnIndex := make(map[uint64]int, len(dataColumnsSidecar))
	for i := range dataColumnsSidecar {
		dataColumnSideCar := dataColumnsSidecar[i]
		index := dataColumnSideCar.Index

		if index < neededColumnCount {
			sliceIndexFromColumnIndex[index] = i
		}
	}

	actualColumnCount := uint64(len(sliceIndexFromColumnIndex))

	// Get missing columns.
	if actualColumnCount < neededColumnCount {
		var missingColumnsSlice []uint64

		for i := range neededColumnCount {
			if _, ok := sliceIndexFromColumnIndex[i]; !ok {
				missingColumnsSlice = append(missingColumnsSlice, i)
			}
		}

		slices.Sort[[]uint64](missingColumnsSlice)
		return nil, errors.Errorf("some columns are missing: %v", missingColumnsSlice)
	}

	// It is safe to retrieve the first column since we already checked that `dataColumnsSidecar` is not empty.
	firstDataColumnSidecar := dataColumnsSidecar[0]

	blobCount := uint64(len(firstDataColumnSidecar.Column))

	// Check all colums have te same length.
	for i := range dataColumnsSidecar {
		if uint64(len(dataColumnsSidecar[i].Column)) != blobCount {
			return nil, errors.Errorf("mismatch in the length of the data columns, expected %d, got %d", blobCount, len(dataColumnsSidecar[i].Column))
		}
	}

	// Reconstruct verified RO blobs from columns.
	verifiedROBlobs := make([]*blocks.VerifiedROBlob, 0, blobCount)

	// Populate and filter indices.
	indicesSlice := populateAndFilterIndices(indices, blobCount)

	for _, blobIndex := range indicesSlice {
		var blob kzg.Blob

		// Compute the content of the blob.
		for columnIndex := range neededColumnCount {
			sliceIndex, ok := sliceIndexFromColumnIndex[columnIndex]
			if !ok {
				return nil, errors.Errorf("missing column %d, this should never happen", columnIndex)
			}

			dataColumnSideCar := dataColumnsSidecar[sliceIndex]
			cell := dataColumnSideCar.Column[blobIndex]

			for i := range cell {
				blob[columnIndex*kzg.BytesPerCell+uint64(i)] = cell[i]
			}
		}

		// Retrieve the blob KZG commitment.
		blobKZGCommitment := kzg.Commitment(firstDataColumnSidecar.KzgCommitments[blobIndex])

		// Compute the blob KZG proof.
		blobKzgProof, err := kzg.ComputeBlobKZGProof(&blob, blobKZGCommitment)
		if err != nil {
			return nil, errors.Wrap(err, "compute blob KZG proof")
		}

		blobSidecar := &ethpb.BlobSidecar{
			Index:                    blobIndex,
			Blob:                     blob[:],
			KzgCommitment:            blobKZGCommitment[:],
			KzgProof:                 blobKzgProof[:],
			SignedBlockHeader:        firstDataColumnSidecar.SignedBlockHeader,
			CommitmentInclusionProof: firstDataColumnSidecar.KzgCommitmentsInclusionProof,
		}

		roBlob, err := blocks.NewROBlob(blobSidecar)
		if err != nil {
			return nil, errors.Wrap(err, "new RO blob")
		}

		verifiedROBlob := blocks.NewVerifiedROBlob(roBlob)
		verifiedROBlobs = append(verifiedROBlobs, &verifiedROBlob)
	}

	return verifiedROBlobs, nil
}

// CustodyGroupSamplingSize returns the number of custody groups the node should sample from.
// https://github.com/ethereum/consensus-specs/blob/v1.5.0-beta.5/specs/fulu/das-core.md#custody-sampling
func (custodyInfo *CustodyInfo) CustodyGroupSamplingSize(ct CustodyType) uint64 {
	custodyGroupCount := custodyInfo.TargetGroupCount.Get()

	if ct == Actual {
		custodyGroupCount = custodyInfo.ActualGroupCount()
	}

	samplesPerSlot := params.BeaconConfig().SamplesPerSlot
	return max(samplesPerSlot, custodyGroupCount)
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

// populateAndFilterIndices returns a sorted slices of indices, setting all indices if none are provided,
// and filtering out indices higher than the blob count.
func populateAndFilterIndices(indices map[uint64]bool, blobCount uint64) []uint64 {
	// If no indices are provided, provide all blobs.
	if len(indices) == 0 {
		for i := range blobCount {
			indices[i] = true
		}
	}

	// Filter blobs index higher than the blob count.
	indicesSlice := make([]uint64, 0, len(indices))
	for i := range indices {
		if i < blobCount {
			indicesSlice = append(indicesSlice, i)
		}
	}

	// Sort the indices.
	slices.Sort[[]uint64](indicesSlice)

	return indicesSlice
}
