package peerdas_test

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestCanSelfReconstruct(t *testing.T) {
	testCases := []struct {
		name                       string
		totalNumberOfCustodyGroups uint64
		custodyNumberOfGroups      uint64
		expected                   bool
	}{
		{
			name:                       "totalNumberOfCustodyGroups=64, custodyNumberOfGroups=31",
			totalNumberOfCustodyGroups: 64,
			custodyNumberOfGroups:      31,
			expected:                   false,
		},
		{
			name:                       "totalNumberOfCustodyGroups=64, custodyNumberOfGroups=32",
			totalNumberOfCustodyGroups: 64,
			custodyNumberOfGroups:      32,
			expected:                   true,
		},
		{
			name:                       "totalNumberOfCustodyGroups=65, custodyNumberOfGroups=32",
			totalNumberOfCustodyGroups: 65,
			custodyNumberOfGroups:      32,
			expected:                   false,
		},
		{
			name:                       "totalNumberOfCustodyGroups=63, custodyNumberOfGroups=33",
			totalNumberOfCustodyGroups: 65,
			custodyNumberOfGroups:      33,
			expected:                   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set the total number of columns.
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.NumberOfCustodyGroups = tc.totalNumberOfCustodyGroups
			params.OverrideBeaconConfig(cfg)

			// Check if reconstuction is possible.
			actual := peerdas.CanSelfReconstruct(tc.custodyNumberOfGroups)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestReconstructionRoundTrip(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	const blobCount = 5

	var blockRoot [fieldparams.RootLength]byte

	signedBeaconBlockPb := util.NewBeaconBlockDeneb()
	require.NoError(t, kzg.Start())

	// Generate random blobs and their corresponding commitments.
	var (
		blobsKzgCommitments [][]byte
		blobs               []kzg.Blob
	)
	for i := range blobCount {
		blob := getRandBlob(int64(i))
		commitment, _, err := generateCommitmentAndProof(&blob)
		require.NoError(t, err)

		blobsKzgCommitments = append(blobsKzgCommitments, commitment[:])
		blobs = append(blobs, blob)
	}

	// Generate a signed beacon block.
	signedBeaconBlockPb.Block.Body.BlobKzgCommitments = blobsKzgCommitments
	signedBeaconBlock, err := blocks.NewSignedBeaconBlock(signedBeaconBlockPb)
	require.NoError(t, err)

	// Get the signed beacon block header.
	signedBeaconBlockHeader, err := signedBeaconBlock.Header()
	require.NoError(t, err)

	// Convert data columns sidecars from signed block and blobs.
	dataColumnSidecars, err := peerdas.DataColumnSidecars(signedBeaconBlock, blobs)
	require.NoError(t, err)

	// Create verified RO data columns.
	verifiedRoDataColumns := make([]*blocks.VerifiedRODataColumn, 0, blobCount)
	for _, dataColumnSidecar := range dataColumnSidecars {
		roDataColumn, err := blocks.NewRODataColumn(dataColumnSidecar)
		require.NoError(t, err)

		verifiedRoDataColumn := blocks.NewVerifiedRODataColumn(roDataColumn)
		verifiedRoDataColumns = append(verifiedRoDataColumns, &verifiedRoDataColumn)
	}

	verifiedRoDataColumn := verifiedRoDataColumns[0]

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	var noDataColumns []*ethpb.DataColumnSidecar
	dataColumnsWithDifferentLengths := []*ethpb.DataColumnSidecar{
		{DataColumn: [][]byte{{}, {}}},
		{DataColumn: [][]byte{{}}},
	}
	notEnoughDataColumns := dataColumnSidecars[:numberOfColumns/2-1]
	originalDataColumns := dataColumnSidecars[:numberOfColumns/2]
	extendedDataColumns := dataColumnSidecars[numberOfColumns/2:]
	evenDataColumns := make([]*ethpb.DataColumnSidecar, 0, numberOfColumns/2)
	oddDataColumns := make([]*ethpb.DataColumnSidecar, 0, numberOfColumns/2)
	allDataColumns := dataColumnSidecars

	for i, dataColumn := range dataColumnSidecars {
		if i%2 == 0 {
			evenDataColumns = append(evenDataColumns, dataColumn)
		} else {
			oddDataColumns = append(oddDataColumns, dataColumn)
		}
	}

	testCases := []struct {
		name               string
		dataColumnsSidecar []*ethpb.DataColumnSidecar
		isError            bool
	}{
		{
			name:               "No data columns sidecars",
			dataColumnsSidecar: noDataColumns,
			isError:            true,
		},
		{
			name:               "Data columns sidecar with different lengths",
			dataColumnsSidecar: dataColumnsWithDifferentLengths,
			isError:            true,
		},
		{
			name:               "All columns are present (no actual need to reconstruct)",
			dataColumnsSidecar: allDataColumns,
			isError:            false,
		},
		{
			name:               "Only original columns are present",
			dataColumnsSidecar: originalDataColumns,
			isError:            false,
		},
		{
			name:               "Only extended columns are present",
			dataColumnsSidecar: extendedDataColumns,
			isError:            false,
		},
		{
			name:               "Only even columns are present",
			dataColumnsSidecar: evenDataColumns,
			isError:            false,
		},
		{
			name:               "Only odd columns are present",
			dataColumnsSidecar: oddDataColumns,
			isError:            false,
		},
		{
			name:               "Not enough columns to reconstruct",
			dataColumnsSidecar: notEnoughDataColumns,
			isError:            true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Recover cells and proofs from available data columns sidecars.
			cellsAndProofs, err := peerdas.RecoverCellsAndProofs(tc.dataColumnsSidecar, blockRoot)
			isError := (err != nil)
			require.Equal(t, tc.isError, isError)

			if isError {
				return
			}

			// Recover all data columns sidecars from cells and proofs.
			reconstructedDataColumnsSideCars, err := peerdas.DataColumnSidecarsForReconstruct(
				blobsKzgCommitments,
				signedBeaconBlockHeader,
				verifiedRoDataColumn.KzgCommitmentsInclusionProof,
				cellsAndProofs,
			)

			require.NoError(t, err)

			expected := dataColumnSidecars
			actual := reconstructedDataColumnsSideCars
			require.DeepSSZEqual(t, expected, actual)
		})
	}
}
