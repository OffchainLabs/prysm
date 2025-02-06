package peerdas

import (
	"context"
	"encoding/binary"
	"math"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"golang.org/x/sync/errgroup"
)

var (
	// Custom errors
	errCustodyGroupCountTooLarge      = errors.New("custody group count too large")
	errWrongComputedCustodyGroupCount = errors.New("wrong computed custody group count, should never happen")

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

// CustodyGroupSamplingSize returns the number of custody groups the node should sample from.
// https://github.com/ethereum/consensus-specs/blob/v1.5.0-alpha.10/specs/fulu/das-core.md#custody-sampling
func CustodyGroupSamplingSize() uint64 {
	samplesPerSlot := params.BeaconConfig().SamplesPerSlot
	custodyGroupCount := CustodyGroupCount()

	return max(samplesPerSlot, custodyGroupCount)
}

// CustodyColumns computes the custody columns from the custody groups.
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
