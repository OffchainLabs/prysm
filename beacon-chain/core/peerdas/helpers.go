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
	CustodyGroupCountEnrKey = "cgc"
)

// https://github.com/ethereum/consensus-specs/blob/v1.5.0-alpha.10/specs/fulu/p2p-interface.md#the-discovery-domain-discv5
type Cgc uint64

func (Cgc) ENRKey() string { return CustodyGroupCountEnrKey }

var (
	// Custom errors
	errCustodyGroupCountTooLarge      = errors.New("custody group count too large")
	errWrongComputedCustodyGroupCount = errors.New("wrong computed custody group count, should never happen")
	errIndexTooLarge                  = errors.New("column index is larger than the specified columns count")
	errMismatchLength                 = errors.New("mismatch in the length of the commitments and proofs")
	errRecordNil                      = errors.New("record is nil")
	errCannotLoadCustodyGroupCount    = errors.New("cannot load the custody group count from peer")

	// maxUint256 is the maximum value of a uint256.
	maxUint256 = &uint256.Int{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64}
)

// CustodyGroups computes the custody groups the node should participate in for custody.
// https://github.com/ethereum/consensus-specs/blob/v1.5.0-alpha.10/specs/fulu/das-core.md#get_custody_groups
func CustodyGroups(nodeId enode.ID, custodyGroupCount uint64) (map[uint64]bool, error) {
	numberOfCustodyGroup := params.BeaconConfig().NumberOfCustodyGroups

	// Check if the custody group count is larger than the number of custody groups.
	if custodyGroupCount > numberOfCustodyGroup {
		return nil, errCustodyGroupCountTooLarge
	}

	custodyGroups := make(map[uint64]bool, custodyGroupCount)
	one := uint256.NewInt(1)

	for currentId := new(uint256.Int).SetBytes(nodeId.Bytes()); uint64(len(custodyGroups)) < custodyGroupCount; currentId.Add(currentId, one) {
		// Convert to big endian bytes.
		currentIdBytesBigEndian := currentId.Bytes32()

		// Convert to little endian.
		currentIdBytesLittleEndian := bytesutil.ReverseByteOrder(currentIdBytesBigEndian[:])

		// Hash the result.
		hashedCurrentId := hash.Hash(currentIdBytesLittleEndian)

		// Get the custody group ID.
		custodyGroupId := binary.LittleEndian.Uint64(hashedCurrentId[:8]) % numberOfCustodyGroup

		// Add the custody group to the map.
		custodyGroups[custodyGroupId] = true

		// Overflow prevention.
		if currentId.Cmp(maxUint256) == 0 {
			currentId = uint256.NewInt(0)
		}
	}

	// Final check.
	if uint64(len(custodyGroups)) != custodyGroupCount {
		return nil, errWrongComputedCustodyGroupCount
	}

	return custodyGroups, nil
}

// ComputeColumnsForCustodyGroup computes the columns for a given custody group.
// https://github.com/ethereum/consensus-specs/blob/v1.5.0-alpha.10/specs/fulu/das-core.md#compute_columns_for_custody_group
func ComputeColumnsForCustodyGroup(custodyGroup uint64) ([]uint64, error) {
	beaconConfig := params.BeaconConfig()
	numberOfCustodyGroup := beaconConfig.NumberOfCustodyGroups

	if custodyGroup > numberOfCustodyGroup {
		return nil, errCustodyGroupCountTooLarge
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

// ComputeCustodyGroupForColumn computes the custody group for a given column.
// It is the reciprocal function of ComputeColumnsForCustodyGroup.
func ComputeCustodyGroupForColumn(columnIndex uint64) (uint64, error) {
	beaconConfig := params.BeaconConfig()
	numberOfColumns := beaconConfig.NumberOfColumns

	if columnIndex >= numberOfColumns {
		return 0, errIndexTooLarge
	}

	numberOfCustodyGroups := beaconConfig.NumberOfCustodyGroups
	columnsPerGroup := numberOfColumns / numberOfCustodyGroups

	return columnIndex / columnsPerGroup, nil
}

// ComputeSubnetForDataColumnSidecar computes the subnet for a data column sidecar.
// https://github.com/ethereum/consensus-specs/blob/dev/specs/fulu/p2p-interface.md#compute_subnet_for_data_column_sidecar
func ComputeSubnetForDataColumnSidecar(columnIndex uint64) uint64 {
	dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount
	return columnIndex % dataColumnSidecarSubnetCount
}

// CustodyColumns computes the columns the node should custody.
func CustodyColumns(custodyGroups map[uint64]bool) (map[uint64]bool, error) {
	numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups

	custodyGroupCount := len(custodyGroups)

	// Compute the columns for each custody group.
	columns := make(map[uint64]bool, custodyGroupCount)
	for group := range custodyGroups {
		if group >= numberOfCustodyGroups {
			return nil, errCustodyGroupCountTooLarge
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

// DataColumnSubnets computes the subnets for the data columns.
func DataColumnSubnets(dataColumns map[uint64]bool) map[uint64]bool {
	subnets := make(map[uint64]bool, len(dataColumns))

	for column := range dataColumns {
		subnet := ComputeSubnetForDataColumnSidecar(column)
		subnets[subnet] = true
	}

	return subnets
}

// DataColumnSidecars computes the data column sidecars from the signed block and blobs.
// https://github.com/ethereum/consensus-specs/blob/dev/specs/fulu/das-core.md#get_data_column_sidecars
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

// VerifyDataColumnsSidecarKZGProofs verifies the provided KZG Proofs of data columns.
func VerifyDataColumnsSidecarKZGProofs(sidecars []blocks.RODataColumn) (bool, error) {
	// Retrieve the number of columns.
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	// Compute the total count.
	count := 0
	for _, sidecar := range sidecars {
		count += len(sidecar.DataColumn)
	}

	commitments := make([]kzg.Bytes48, 0, count)
	indices := make([]uint64, 0, count)
	cells := make([]kzg.Cell, 0, count)
	proofs := make([]kzg.Bytes48, 0, count)

	for _, sidecar := range sidecars {
		// Check if the columns index is not too large
		if sidecar.ColumnIndex >= numberOfColumns {
			return false, errIndexTooLarge
		}

		// Check if the KZG commitments size and data column size match.
		if len(sidecar.DataColumn) != len(sidecar.KzgCommitments) {
			return false, errMismatchLength
		}

		// Check if the KZG proofs size and data column size match.
		if len(sidecar.DataColumn) != len(sidecar.KzgProof) {
			return false, errMismatchLength
		}

		for i := range sidecar.DataColumn {
			commitments = append(commitments, kzg.Bytes48(sidecar.KzgCommitments[i]))
			indices = append(indices, sidecar.ColumnIndex)
			cells = append(cells, kzg.Cell(sidecar.DataColumn[i]))
			proofs = append(proofs, kzg.Bytes48(sidecar.KzgProof[i]))
		}
	}

	// Verify all the batch at once.
	verified, err := kzg.VerifyCellKZGProofBatch(commitments, indices, cells, proofs)
	if err != nil {
		return false, errors.Wrap(err, "verify cell KZG proof batch")
	}

	return verified, nil
}

// CustodyGroupCount returns the number of groups the node should participate in for custody.
func CustodyGroupCount() uint64 {
	if flags.Get().SubscribeToAllSubnets {
		return params.BeaconConfig().NumberOfCustodyGroups
	}

	return params.BeaconConfig().CustodyRequirement
}

// CustodyGroupSamplingSize returns the number of custody groups the node should sample from.
// https://github.com/ethereum/consensus-specs/blob/v1.5.0-alpha.10/specs/fulu/das-core.md#custody-sampling
func CustodyGroupSamplingSize() uint64 {
	samplesPerSlot := params.BeaconConfig().SamplesPerSlot
	custodyGroupCount := CustodyGroupCount()

	return max(samplesPerSlot, custodyGroupCount)
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

// CustodyGroupCountFromRecord extracts the custody group count from an ENR record.
func CustodyGroupCountFromRecord(record *enr.Record) (uint64, error) {
	if record == nil {
		return 0, errRecordNil
	}

	// Load the `cgc`
	var cgc Cgc
	if cgc := record.Load(&cgc); cgc != nil {
		return 0, errCannotLoadCustodyGroupCount
	}

	return uint64(cgc), nil
}

func CanSelfReconstruct(custodyGroupCount uint64) bool {
	total := params.BeaconConfig().NumberOfCustodyGroups
	// If total is odd, then we need total / 2 + 1 columns to reconstruct.
	// If total is even, then we need total / 2 columns to reconstruct.
	custodyGroupsNeeded := total/2 + total%2
	return custodyGroupCount >= custodyGroupsNeeded
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
