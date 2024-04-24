package peerdas

import (
	"encoding/binary"

	cKzg4844 "github.com/ethereum/c-kzg-4844/bindings/go"
	"github.com/holiman/uint256"

	"github.com/ethereum/go-ethereum/p2p/enode"
	errors "github.com/pkg/errors"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

const (
	// Number of field elements per extended blob
	fieldElementsPerExtBlob = 2 * cKzg4844.FieldElementsPerBlob

	// Bytes per cell
	bytesPerCell = cKzg4844.FieldElementsPerCell * cKzg4844.BytesPerFieldElement

	// Number of cells in the extended matrix
	extendedMatrixSize = fieldparams.MaxBlobsPerBlock * cKzg4844.CellsPerExtBlob
)

type (
	extendedMatrix []cKzg4844.Cell

	cellCoordinate struct {
		blobIndex uint64
		cellID    uint64
	}
)

var (
	errCustodySubnetCountTooLarge          = errors.New("custody subnet count larger than data column sidecar subnet count")
	errCellNotFound                        = errors.New("cell not found (should never happen)")
	errCurveOrder                          = errors.New("could not set bls curve order as big int")
	errBlsFieldElementNil                  = errors.New("bls field element is nil")
	errBlsFieldElementBiggerThanCurveOrder = errors.New("bls field element higher than curve order")
	errBlsFieldElementDoesNotFit           = errors.New("bls field element does not fit in BytesPerFieldElement")
)

// https://github.com/ethereum/consensus-specs/blob/dev/specs/_features/eip7594/das-core.md#helper-functions
func CustodyColumns(nodeId enode.ID, custodySubnetCount uint64) (map[uint64]bool, error) {
	dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount

	// Compute the custodied subnets.
	subnetIds, err := CustodyColumnSubnets(nodeId, custodySubnetCount)
	if err != nil {
		return nil, errors.Wrap(err, "custody subnets")
	}

	columnsPerSubnet := cKzg4844.CellsPerExtBlob / dataColumnSidecarSubnetCount

	// Knowing the subnet ID and the number of columns per subnet, select all the columns the node should custody.
	// Columns belonging to the same subnet are contiguous.
	columnIndices := make(map[uint64]bool, custodySubnetCount*columnsPerSubnet)
	for i := uint64(0); i < columnsPerSubnet; i++ {
		for subnetId := range subnetIds {
			columnIndex := dataColumnSidecarSubnetCount*i + subnetId
			columnIndices[columnIndex] = true
		}
	}

	return columnIndices, nil
}

func CustodyColumnSubnets(nodeId enode.ID, custodySubnetCount uint64) (map[uint64]bool, error) {
	dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount

	// Check if the custody subnet count is larger than the data column sidecar subnet count.
	if custodySubnetCount > dataColumnSidecarSubnetCount {
		return nil, errCustodySubnetCountTooLarge
	}

	// First, compute the subnet IDs that the node should participate in.
	subnetIds := make(map[uint64]bool, custodySubnetCount)

	for i := uint64(0); uint64(len(subnetIds)) < custodySubnetCount; i++ {
		nodeIdUInt256, nextNodeIdUInt256 := new(uint256.Int), new(uint256.Int)
		nodeIdUInt256.SetBytes(nodeId.Bytes())
		nextNodeIdUInt256.Add(nodeIdUInt256, uint256.NewInt(i))
		nextNodeIdUInt64 := nextNodeIdUInt256.Uint64()
		nextNodeId := bytesutil.Uint64ToBytesLittleEndian(nextNodeIdUInt64)

		hashedNextNodeId := hash.Hash(nextNodeId)
		subnetId := binary.LittleEndian.Uint64(hashedNextNodeId[:8]) % dataColumnSidecarSubnetCount

		if _, exists := subnetIds[subnetId]; !exists {
			subnetIds[subnetId] = true
		}
	}

	return subnetIds, nil
}

// computeExtendedMatrix computes the extended matrix from the blobs.
// https://github.com/ethereum/consensus-specs/blob/dev/specs/_features/eip7594/das-core.md#compute_extended_matrix
func computeExtendedMatrix(blobs []cKzg4844.Blob) (extendedMatrix, error) {
	matrix := make(extendedMatrix, 0, extendedMatrixSize)

	for i := range blobs {
		// Chunk a non-extended blob into cells representing the corresponding extended blob.
		blob := &blobs[i]
		cells, err := cKzg4844.ComputeCells(blob)
		if err != nil {
			return nil, errors.Wrap(err, "compute cells for blob")
		}

		matrix = append(matrix, cells[:]...)
	}

	return matrix, nil
}

// recoverMatrix recovers the extended matrix from some cells.
// https://github.com/ethereum/consensus-specs/blob/dev/specs/_features/eip7594/das-core.md#recover_matrix
func recoverMatrix(cellFromCoordinate map[cellCoordinate]cKzg4844.Cell, blobCount uint64) (extendedMatrix, error) {
	matrix := make(extendedMatrix, 0, extendedMatrixSize)

	for blobIndex := uint64(0); blobIndex < blobCount; blobIndex++ {
		// Filter all cells that belong to the current blob.
		cellIds := make([]uint64, 0, cKzg4844.CellsPerExtBlob)
		for coordinate := range cellFromCoordinate {
			if coordinate.blobIndex == blobIndex {
				cellIds = append(cellIds, coordinate.cellID)
			}
		}

		// Retrieve cells corresponding to all `cellIds`.
		cellIdsCount := len(cellIds)

		cells := make([]cKzg4844.Cell, 0, cellIdsCount)
		for _, cellId := range cellIds {
			coordinate := cellCoordinate{blobIndex: blobIndex, cellID: cellId}
			cell, ok := cellFromCoordinate[coordinate]
			if !ok {
				return matrix, errCellNotFound
			}

			cells = append(cells, cell)
		}

		// Recover all cells.
		allCellsForRow, err := cKzg4844.RecoverAllCells(cellIds, cells)
		if err != nil {
			return matrix, errors.Wrap(err, "recover all cells")
		}

		matrix = append(matrix, allCellsForRow[:]...)
	}

	return matrix, nil
}

// https://github.com/ethereum/consensus-specs/blob/dev/specs/_features/eip7594/das-core.md#recover_matrix
func dataColumnSidecars(signedBlock interfaces.SignedBeaconBlock, blobs []cKzg4844.Blob) ([]ethpb.DataColumnSidecar, error) {
	blobsCount := len(blobs)

	// Get the signed block header.
	signedBlockHeader, err := signedBlock.Header()
	if err != nil {
		return nil, errors.Wrap(err, "signed block header")
	}

	// Get the block body.
	block := signedBlock.Block()
	blockBody := block.Body()

	// Get the blob KZG commitments.
	blobKzgCommitments, err := blockBody.BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "blob KZG commitments")
	}

	// Compute the KZG commitments inclusion proof.
	kzgCommitmentsInclusionProof, err := blocks.MerkleProofKZGCommitments(blockBody)
	if err != nil {
		return nil, errors.Wrap(err, "merkle proof ZKG commitments")
	}

	// Compute cells and proofs.
	cells := make([][cKzg4844.CellsPerExtBlob]cKzg4844.Cell, 0, blobsCount)
	proofs := make([][cKzg4844.CellsPerExtBlob]cKzg4844.KZGProof, 0, blobsCount)

	for i := range blobs {
		blob := &blobs[i]
		blobCells, blobProofs, err := cKzg4844.ComputeCellsAndProofs(blob)
		if err != nil {
			return nil, errors.Wrap(err, "compute cells and proofs")
		}

		cells = append(cells, blobCells)
		proofs = append(proofs, blobProofs)
	}

	// Get the column sidecars.
	sidecars := make([]ethpb.DataColumnSidecar, cKzg4844.CellsPerExtBlob)
	for columnIndex := uint64(0); columnIndex < cKzg4844.CellsPerExtBlob; columnIndex++ {
		column := make([]cKzg4844.Cell, 0, blobsCount)
		kzgProofOfColumn := make([]cKzg4844.KZGProof, 0, blobsCount)

		for rowIndex := 0; rowIndex < blobsCount; rowIndex++ {
			cell := cells[rowIndex][columnIndex]
			column = append(column, cell)

			kzgProof := proofs[rowIndex][columnIndex]
			kzgProofOfColumn = append(kzgProofOfColumn, kzgProof)
		}

		columnBytes := make([][]byte, 0, blobsCount)
		for i := range column {
			cell := column[i]

			cellBytes := make([]byte, 0, bytesPerCell)
			for _, fieldElement := range cell {
				cellBytes = append(cellBytes, fieldElement[:]...)
			}

			columnBytes = append(columnBytes, cellBytes)
		}

		kzgProofOfColumnBytes := make([][]byte, 0, blobsCount)
		for _, kzgProof := range kzgProofOfColumn {
			kzgProofOfColumnBytes = append(kzgProofOfColumnBytes, kzgProof[:])
		}

		sidecars = append(sidecars, ethpb.DataColumnSidecar{
			ColumnIndex:                  columnIndex,
			DataColumn:                   columnBytes,
			KzgCommitments:               blobKzgCommitments,
			KzgProof:                     kzgProofOfColumnBytes,
			SignedBlockHeader:            signedBlockHeader,
			KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
		})
	}

	return sidecars, nil
}
