package peerdas

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"slices"
	"time"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/holiman/uint256"
	errors "github.com/pkg/errors"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"

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
	startTime := time.Now()
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
	cellsAndProofs := make([]kzg.CellsAndProofs, blobsCount)

	eg, _ := errgroup.WithContext(context.Background())
	for i := range blobs {
		blobIndex := i
		eg.Go(func() error {
			blob := &blobs[blobIndex]
			blobCellsAndProofs, err := kzg.ComputeCellsAndKZGProofs(blob)
			if err != nil {
				return errors.Wrap(err, "compute cells and KZG proofs")
			}

			cellsAndProofs[blobIndex] = blobCellsAndProofs
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
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
	dataColumnComputationTime.Observe(float64(time.Since(startTime).Milliseconds()))
	return sidecars, nil
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
	filteredIndices := make(map[uint64]bool, len(indices))
	for i := range indices {
		if i < blobCount {
			filteredIndices[i] = true
		}
	}

	// Transform set to slice.
	indicesSlice := make([]uint64, 0, len(filteredIndices))
	for i := range filteredIndices {
		indicesSlice = append(indicesSlice, i)
	}

	// Sort the indices.
	slices.Sort[[]uint64](indicesSlice)

	return indicesSlice
}

// Blobs extract blobs from `dataColumnsSidecar`.
// This can be seen as the reciprocal function of DataColumnSidecars.
// `dataColumnsSidecar` needs to contain the datacolumns corresponding to the non-extended matrix,
// else an error will be returned.
// (`dataColumnsSidecar` can contain extra columns, but they will be ignored.)
func Blobs(indices map[uint64]bool, dataColumnsSidecar []*ethpb.DataColumnSidecar) ([]*blocks.VerifiedROBlob, error) {
	columnCount := fieldparams.NumberOfColumns

	neededColumnCount := columnCount / 2

	// Check if all needed columns are present.
	sliceIndexFromColumnIndex := make(map[uint64]int, len(dataColumnsSidecar))
	for i := range dataColumnsSidecar {
		dataColumnSideCar := dataColumnsSidecar[i]
		columnIndex := dataColumnSideCar.ColumnIndex

		if columnIndex < uint64(neededColumnCount) {
			sliceIndexFromColumnIndex[columnIndex] = i
		}
	}

	actualColumnCount := len(sliceIndexFromColumnIndex)

	// Get missing columns.
	if actualColumnCount < neededColumnCount {
		missingColumns := make(map[int]bool, neededColumnCount-actualColumnCount)
		for i := range neededColumnCount {
			if _, ok := sliceIndexFromColumnIndex[uint64(i)]; !ok {
				missingColumns[i] = true
			}
		}

		missingColumnsSlice := make([]int, 0, len(missingColumns))
		for i := range missingColumns {
			missingColumnsSlice = append(missingColumnsSlice, i)
		}

		slices.Sort[[]int](missingColumnsSlice)
		return nil, errors.Errorf("some columns are missing: %v", missingColumnsSlice)
	}

	// It is safe to retrieve the first column since we already checked that `dataColumnsSidecar` is not empty.
	firstDataColumnSidecar := dataColumnsSidecar[0]

	blobCount := uint64(len(firstDataColumnSidecar.DataColumn))

	// Check all colums have te same length.
	for i := range dataColumnsSidecar {
		if uint64(len(dataColumnsSidecar[i].DataColumn)) != blobCount {
			return nil, errors.Errorf("mismatch in the length of the data columns, expected %d, got %d", blobCount, len(dataColumnsSidecar[i].DataColumn))
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
			sliceIndex, ok := sliceIndexFromColumnIndex[uint64(columnIndex)]
			if !ok {
				return nil, errors.Errorf("missing column %d, this should never happen", columnIndex)
			}

			dataColumnSideCar := dataColumnsSidecar[sliceIndex]
			cell := dataColumnSideCar.DataColumn[blobIndex]

			for i := 0; i < len(cell); i++ {
				blob[columnIndex*kzg.BytesPerCell+i] = cell[i]
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

// CustodyColumnCount returns the number of columns the node should custody.
func CustodyColumnCount() uint64 {
	// Get the number of subnets.
	dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount

	// Compute the number of columns per subnet.
	columnsPerSubnet := fieldparams.NumberOfColumns / dataColumnSidecarSubnetCount

	// Get the number of subnets we custody
	custodySubnetCount := CustodySubnetCount()

	// Finally, compute the number of columns we should custody.
	custodyColumnCount := custodySubnetCount * columnsPerSubnet

	return custodyColumnCount
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

// RecoverCellsAndProofs recovers the cells and proofs from the data column sidecars.
func RecoverCellsAndProofs(
	dataColumnSideCars []*ethpb.DataColumnSidecar,
	blockRoot [fieldparams.RootLength]byte,
) ([]kzg.CellsAndProofs, error) {
	var wg errgroup.Group

	dataColumnSideCarsCount := len(dataColumnSideCars)

	if dataColumnSideCarsCount == 0 {
		return nil, errors.New("no data column sidecars")
	}

	// Check if all columns have the same length.
	blobCount := len(dataColumnSideCars[0].DataColumn)
	for _, sidecar := range dataColumnSideCars {
		length := len(sidecar.DataColumn)

		if length != blobCount {
			return nil, errors.New("columns do not have the same length")
		}
	}

	// Recover cells and compute proofs in parallel.
	recoveredCellsAndProofs := make([]kzg.CellsAndProofs, blobCount)

	for blobIndex := 0; blobIndex < blobCount; blobIndex++ {
		bIndex := blobIndex
		wg.Go(func() error {
			start := time.Now()

			cellsIndices := make([]uint64, 0, dataColumnSideCarsCount)
			cells := make([]kzg.Cell, 0, dataColumnSideCarsCount)

			for _, sidecar := range dataColumnSideCars {
				// Build the cell indices.
				cellsIndices = append(cellsIndices, sidecar.ColumnIndex)

				// Get the cell.
				column := sidecar.DataColumn
				cell := column[bIndex]

				cells = append(cells, kzg.Cell(cell))
			}

			// Recover the cells and proofs for the corresponding blob
			cellsAndProofs, err := kzg.RecoverCellsAndKZGProofs(cellsIndices, cells)

			if err != nil {
				return errors.Wrapf(err, "recover cells and KZG proofs for blob %d", bIndex)
			}

			recoveredCellsAndProofs[bIndex] = cellsAndProofs
			log.WithFields(logrus.Fields{
				"elapsed": time.Since(start),
				"index":   bIndex,
				"root":    fmt.Sprintf("%x", blockRoot),
			}).Debug("Recovered cells and proofs")
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return nil, err
	}

	return recoveredCellsAndProofs, nil
}
