package peerdas

import (
	"encoding/binary"
	"math"
	"math/big"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/holiman/uint256"
	errors "github.com/pkg/errors"

	kzg "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"

	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

const (
	CustodySubnetCountEnrKey = "csc"
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

	columnsPerSubnet := fieldparams.NumberOfColumns / dataColumnSidecarSubnetCount

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
func DataColumnSidecars(signedBlock interfaces.ReadOnlySignedBeaconBlock, blobs []kzg.Blob) ([]*ethpb.DataColumnSidecar, error) {
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
	cellsAndProofs := make([]kzg.CellsAndProofs, 0, blobsCount)

	for i := range blobs {
		blob := &blobs[i]
		blobCellsAndProofs, err := kzg.ComputeCellsAndKZGProofs(blob)
		if err != nil {
			return nil, errors.Wrap(err, "compute cells and KZG proofs")
		}

		cellsAndProofs = append(cellsAndProofs, blobCellsAndProofs)
	}

	// Get the column sidecars.
	sidecars := make([]*ethpb.DataColumnSidecar, 0, fieldparams.NumberOfColumns)
	for columnIndex := uint64(0); columnIndex < fieldparams.NumberOfColumns; columnIndex++ {
		column := make([]kzg.Cell, 0, blobsCount)
		kzgProofOfColumn := make([]kzg.Proof, 0, blobsCount)

		for rowIndex := 0; rowIndex < blobsCount; rowIndex++ {
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
	cellsAndProofs []kzg.CellsAndProofs,
) ([]*ethpb.DataColumnSidecar, error) {
	// Each CellsAndProofs corresponds to a Blob
	// So we can get the BlobCount by checking the length of CellsAndProofs
	blobsCount := len(cellsAndProofs)
	if blobsCount == 0 {
		return nil, nil
	}

	// Get the column sidecars.
	sidecars := make([]*ethpb.DataColumnSidecar, 0, fieldparams.NumberOfColumns)
	for columnIndex := uint64(0); columnIndex < fieldparams.NumberOfColumns; columnIndex++ {
		column := make([]kzg.Cell, 0, blobsCount)
		kzgProofOfColumn := make([]kzg.Proof, 0, blobsCount)

		for rowIndex := 0; rowIndex < blobsCount; rowIndex++ {
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
func VerifyDataColumnSidecarKZGProofs(sc blocks.RODataColumn) (bool, error) {
	if sc.ColumnIndex >= params.BeaconConfig().NumberOfColumns {
		return false, errIndexTooLarge
	}
	if len(sc.DataColumn) != len(sc.KzgCommitments) || len(sc.KzgCommitments) != len(sc.KzgProof) {
		return false, errMismatchLength
	}

	var commitments []kzg.Bytes48
	var indices []uint64
	var cells []kzg.Cell
	var proofs []kzg.Bytes48
	for i := range sc.DataColumn {
		commitments = append(commitments, kzg.Bytes48(sc.KzgCommitments[i]))
		indices = append(indices, sc.ColumnIndex)
		cells = append(cells, kzg.Cell(sc.DataColumn[i]))
		proofs = append(proofs, kzg.Bytes48(sc.KzgProof[i]))
	}

	return kzg.VerifyCellKZGProofBatch(commitments, indices, cells, proofs)
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

func CanSelfReconstruct(numCol uint64) bool {
	total := params.BeaconConfig().NumberOfColumns
	// if total is odd, then we need total / 2 + 1 columns to reconstruct
	// if total is even, then we need total / 2 columns to reconstruct
	columnsNeeded := total/2 + total%2
	return numCol >= columnsNeeded
}
