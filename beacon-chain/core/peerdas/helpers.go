package peerdas

import (
	"encoding/binary"
	"math"
	"math/big"

	cKzg4844 "github.com/ethereum/c-kzg-4844/bindings/go"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/holiman/uint256"
	errors "github.com/pkg/errors"

	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// Bytes per cell
const (
	CustodySubnetCountEnrKey = "csc"

	bytesPerCell = cKzg4844.FieldElementsPerCell * cKzg4844.BytesPerFieldElement
)

// https://github.com/ethereum/consensus-specs/blob/dev/specs/_features/eip7594/p2p-interface.md#the-discovery-domain-discv5
type Csc uint64

func (Csc) ENRKey() string { return CustodySubnetCountEnrKey }

var (
	// Custom errors
	errCustodySubnetCountTooLarge   = errors.New("custody subnet count larger than data column sidecar subnet count")
	errIndexTooLarge                = errors.New("column index is larger than the specified columns count")
	errMismatchLength               = errors.New("mismatch in the length of the commitments and proofs")
	errRecordNil                    = errors.New("record is nil")
	errCannotLoadCustodySubnetCount = errors.New("cannot load the custody subnet count from peer")

	// maxUint256 is the maximum value of a uint256.
	maxUint256 = &uint256.Int{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64}
)

// CustodyColumnSubnets computes the subnets the node should participate in for custody.
func CustodyColumnSubnets(nodeId enode.ID, custodySubnetCount uint64) (map[uint64]bool, error) {
	dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount

	// Check if the custody subnet count is larger than the data column sidecar subnet count.
	if custodySubnetCount > dataColumnSidecarSubnetCount {
		return nil, errCustodySubnetCountTooLarge
	}

	// First, compute the subnet IDs that the node should participate in.
	subnetIds := make(map[uint64]bool, custodySubnetCount)

	one := uint256.NewInt(1)

	for currentId := new(uint256.Int).SetBytes(nodeId.Bytes()); uint64(len(subnetIds)) < custodySubnetCount; currentId.Add(currentId, one) {
		// Convert to big endian bytes.
		currentIdBytesBigEndian := currentId.Bytes32()

		// Convert to little endian.
		currentIdBytesLittleEndian := bytesutil.ReverseByteOrder(currentIdBytesBigEndian[:])

		// Hash the result.
		hashedCurrentId := hash.Hash(currentIdBytesLittleEndian)

		// Get the subnet ID.
		subnetId := binary.LittleEndian.Uint64(hashedCurrentId[:8]) % dataColumnSidecarSubnetCount

		// Add the subnet to the map.
		subnetIds[subnetId] = true

		// Overflow prevention.
		if currentId.Cmp(maxUint256) == 0 {
			currentId = uint256.NewInt(0)
		}
	}

	return subnetIds, nil
}

// CustodyColumns computes the columns the node should custody.
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

// DataColumnSidecars computes the data column sidecars from the signed block and blobs.
// https://github.com/ethereum/consensus-specs/blob/dev/specs/_features/eip7594/das-core.md#recover_matrix
func DataColumnSidecars(signedBlock interfaces.ReadOnlySignedBeaconBlock, blobs []cKzg4844.Blob) ([]*ethpb.DataColumnSidecar, error) {
	blobsCount := len(blobs)
	if blobsCount == 0 {
		return nil, nil
	}

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
		blobCells, blobProofs, err := cKzg4844.ComputeCellsAndKZGProofs(blob)
		if err != nil {
			return nil, errors.Wrap(err, "compute cells and KZG proofs")
		}

		cells = append(cells, blobCells)
		proofs = append(proofs, blobProofs)
	}

	// Get the column sidecars.
	sidecars := make([]*ethpb.DataColumnSidecar, 0, cKzg4844.CellsPerExtBlob)
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
				copiedElem := fieldElement
				cellBytes = append(cellBytes, copiedElem[:]...)
			}

			columnBytes = append(columnBytes, cellBytes)
		}

		kzgProofOfColumnBytes := make([][]byte, 0, blobsCount)
		for _, kzgProof := range kzgProofOfColumn {
			copiedProof := kzgProof
			kzgProofOfColumnBytes = append(kzgProofOfColumnBytes, copiedProof[:])
		}

		sidecar := &ethpb.DataColumnSidecar{
			ColumnIndex:                  columnIndex,
			DataColumn:                   columnBytes,
			KzgCommitments:               blobKzgCommitments,
			KzgProof:                     kzgProofOfColumnBytes,
			SignedBlockHeader:            signedBlockHeader,
			KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
		}

		sidecars = append(sidecars, sidecar)
	}

	return sidecars, nil
}

// DataColumnSidecarsForReconstruct is a TEMPORARY function until there is an official specification for it.
// It is scheduled for deletion.
func DataColumnSidecarsForReconstruct(
	blobKzgCommitments [][]byte,
	signedBlockHeader *ethpb.SignedBeaconBlockHeader,
	kzgCommitmentsInclusionProof [][]byte,
	blobs []cKzg4844.Blob,
) ([]*ethpb.DataColumnSidecar, error) {
	blobsCount := len(blobs)
	if blobsCount == 0 {
		return nil, nil
	}

	// Compute cells and proofs.
	cells := make([][cKzg4844.CellsPerExtBlob]cKzg4844.Cell, 0, blobsCount)
	proofs := make([][cKzg4844.CellsPerExtBlob]cKzg4844.KZGProof, 0, blobsCount)

	for i := range blobs {
		blob := &blobs[i]
		blobCells, blobProofs, err := cKzg4844.ComputeCellsAndKZGProofs(blob)
		if err != nil {
			return nil, errors.Wrap(err, "compute cells and KZG proofs")
		}

		cells = append(cells, blobCells)
		proofs = append(proofs, blobProofs)
	}

	// Get the column sidecars.
	sidecars := make([]*ethpb.DataColumnSidecar, 0, cKzg4844.CellsPerExtBlob)
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
				copiedElem := fieldElement
				cellBytes = append(cellBytes, copiedElem[:]...)
			}

			columnBytes = append(columnBytes, cellBytes)
		}

		kzgProofOfColumnBytes := make([][]byte, 0, blobsCount)
		for _, kzgProof := range kzgProofOfColumn {
			copiedProof := kzgProof
			kzgProofOfColumnBytes = append(kzgProofOfColumnBytes, copiedProof[:])
		}

		sidecar := &ethpb.DataColumnSidecar{
			ColumnIndex:                  columnIndex,
			DataColumn:                   columnBytes,
			KzgCommitments:               blobKzgCommitments,
			KzgProof:                     kzgProofOfColumnBytes,
			SignedBlockHeader:            signedBlockHeader,
			KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
		}

		sidecars = append(sidecars, sidecar)
	}

	return sidecars, nil
}

// VerifyDataColumnSidecarKZGProofs verifies the provided KZG Proofs for the particular
// data column.
func VerifyDataColumnSidecarKZGProofs(sc *ethpb.DataColumnSidecar) (bool, error) {
	if sc.ColumnIndex >= params.BeaconConfig().NumberOfColumns {
		return false, errIndexTooLarge
	}
	if len(sc.DataColumn) != len(sc.KzgCommitments) || len(sc.KzgCommitments) != len(sc.KzgProof) {
		return false, errMismatchLength
	}
	blobsCount := len(sc.DataColumn)

	rowIdx := make([]uint64, 0, blobsCount)
	colIdx := make([]uint64, 0, blobsCount)
	for i := 0; i < len(sc.DataColumn); i++ {
		copiedI := uint64(i)
		rowIdx = append(rowIdx, copiedI)
		colI := sc.ColumnIndex
		colIdx = append(colIdx, colI)
	}
	ckzgComms := make([]cKzg4844.Bytes48, 0, len(sc.KzgCommitments))
	for _, com := range sc.KzgCommitments {
		ckzgComms = append(ckzgComms, cKzg4844.Bytes48(com))
	}
	var cells []cKzg4844.Cell
	for _, ce := range sc.DataColumn {
		var newCell []cKzg4844.Bytes32
		for i := 0; i < len(ce); i += 32 {
			newCell = append(newCell, cKzg4844.Bytes32(ce[i:i+32]))
		}
		cells = append(cells, cKzg4844.Cell(newCell))
	}
	var proofs []cKzg4844.Bytes48
	for _, p := range sc.KzgProof {
		proofs = append(proofs, cKzg4844.Bytes48(p))
	}
	return cKzg4844.VerifyCellKZGProofBatch(ckzgComms, rowIdx, colIdx, cells, proofs)
}

// CustodySubnetCount returns the number of subnets the node should participate in for custody.
func CustodySubnetCount() uint64 {
	count := params.BeaconConfig().CustodyRequirement
	if flags.Get().SubscribeToAllSubnets {
		count = params.BeaconConfig().DataColumnSidecarSubnetCount
	}
	return count
}

// HypergeomCDF computes the hypergeometric cumulative distribution function.
// https://en.wikipedia.org/wiki/Hypergeometric_distribution
func HypergeomCDF(k, M, n, N uint64) float64 {
	denominatorInt := new(big.Int).Binomial(int64(M), int64(N)) // lint:ignore uintcast
	denominator := new(big.Float).SetInt(denominatorInt)

	rBig := big.NewFloat(0)

	for i := uint64(0); i < k+1; i++ {
		a := new(big.Int).Binomial(int64(n), int64(i)) // lint:ignore uintcast
		b := new(big.Int).Binomial(int64(M-n), int64(N-i))
		numeratorInt := new(big.Int).Mul(a, b)
		numerator := new(big.Float).SetInt(numeratorInt)
		item := new(big.Float).Quo(numerator, denominator)
		rBig.Add(rBig, item)
	}

	r, _ := rBig.Float64()

	return r
}

// ExtendedSampleCount computes, for a given number of samples per slot and allowed failures the
// number of samples we should actually query from peers.
// TODO: Add link to the specification once it is available.
func ExtendedSampleCount(samplesPerSlot, allowedFailures uint64) uint64 {
	// Retrieve the columns count
	columnsCount := params.BeaconConfig().NumberOfColumns

	// If half of the columns are missing, we are able to reconstruct the data.
	// If half of the columns + 1 are missing, we are not able to reconstruct the data.
	// This is the smallest worst case.
	worstCaseMissing := columnsCount/2 + 1

	// Compute the false positive threshold.
	falsePositiveThreshold := HypergeomCDF(0, columnsCount, worstCaseMissing, samplesPerSlot)

	var sampleCount uint64

	// Finally, compute the extended sample count.
	for sampleCount = samplesPerSlot; sampleCount < columnsCount+1; sampleCount++ {
		if HypergeomCDF(allowedFailures, columnsCount, worstCaseMissing, sampleCount) <= falsePositiveThreshold {
			break
		}
	}

	return sampleCount
}

func CustodyCountFromRecord(record *enr.Record) (uint64, error) {
	// By default, we assume the peer custodies the minimum number of subnets.
	if record == nil {
		return 0, errRecordNil
	}

	// Load the `custody_subnet_count`
	var csc Csc
	if err := record.Load(&csc); err != nil {
		return 0, errCannotLoadCustodySubnetCount
	}

	return uint64(csc), nil
}
