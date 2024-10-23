package peerdas_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/sirupsen/logrus"
)

func deterministicRandomness(seed int64) [32]byte {
	// Converts an int64 to a byte slice
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, seed)
	if err != nil {
		logrus.WithError(err).Error("Failed to write int64 to bytes buffer")
		return [32]byte{}
	}
	bytes := buf.Bytes()

	return sha256.Sum256(bytes)
}

// Returns a serialized random field element in big-endian
func GetRandFieldElement(seed int64) [32]byte {
	bytes := deterministicRandomness(seed)
	var r fr.Element
	r.SetBytes(bytes[:])

	return GoKZG.SerializeScalar(r)
}

// Returns a random blob using the passed seed as entropy
func GetRandBlob(seed int64) kzg.Blob {
	var blob kzg.Blob
	bytesPerBlob := GoKZG.ScalarsPerBlob * GoKZG.SerializedScalarSize
	for i := 0; i < bytesPerBlob; i += GoKZG.SerializedScalarSize {
		fieldElementBytes := GetRandFieldElement(seed + int64(i))
		copy(blob[i:i+GoKZG.SerializedScalarSize], fieldElementBytes[:])
	}
	return blob
}

func GenerateCommitmentAndProof(blob *kzg.Blob) (*kzg.Commitment, *kzg.Proof, error) {
	commitment, err := kzg.BlobToKZGCommitment(blob)
	if err != nil {
		return nil, nil, err
	}
	proof, err := kzg.ComputeBlobKZGProof(blob, commitment)
	if err != nil {
		return nil, nil, err
	}
	return &commitment, &proof, err
}

func TestVerifyDataColumnSidecarKZGProofs(t *testing.T) {
	dbBlock := util.NewBeaconBlockDeneb()
	require.NoError(t, kzg.Start())

	var (
		comms [][]byte
		blobs []kzg.Blob
	)
	for i := int64(0); i < 6; i++ {
		blob := GetRandBlob(i)
		commitment, _, err := GenerateCommitmentAndProof(&blob)
		require.NoError(t, err)
		comms = append(comms, commitment[:])
		blobs = append(blobs, blob)
	}

	dbBlock.Block.Body.BlobKzgCommitments = comms
	sBlock, err := blocks.NewSignedBeaconBlock(dbBlock)
	require.NoError(t, err)
	sCars, err := peerdas.DataColumnSidecars(sBlock, blobs)
	require.NoError(t, err)

	for i, sidecar := range sCars {
		roCol, err := blocks.NewRODataColumn(sidecar)
		require.NoError(t, err)
		verified, err := peerdas.VerifyDataColumnsSidecarKZGProofs([]blocks.RODataColumn{roCol})
		require.NoError(t, err)
		require.Equal(t, true, verified, fmt.Sprintf("sidecar %d failed", i))
	}
}

func TestDataColumnSidecars(t *testing.T) {
	var expected []*ethpb.DataColumnSidecar = nil
	actual, err := peerdas.DataColumnSidecars(nil, []kzg.Blob{})
	require.NoError(t, err)

	require.DeepSSZEqual(t, expected, actual)
}

func TestBlobs(t *testing.T) {
	blobsIndice := map[uint64]bool{}

	almostAllColumns := make([]*ethpb.DataColumnSidecar, 0, fieldparams.NumberOfColumns/2)
	for i := 2; i < fieldparams.NumberOfColumns/2+2; i++ {
		almostAllColumns = append(almostAllColumns, &ethpb.DataColumnSidecar{
			ColumnIndex: uint64(i),
		})
	}

	testCases := []struct {
		name     string
		input    []*ethpb.DataColumnSidecar
		expected []*blocks.VerifiedROBlob
		err      error
	}{
		{
			name:     "empty input",
			input:    []*ethpb.DataColumnSidecar{},
			expected: nil,
			err:      errors.New("some columns are missing: [0 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30 31 32 33 34 35 36 37 38 39 40 41 42 43 44 45 46 47 48 49 50 51 52 53 54 55 56 57 58 59 60 61 62 63]"),
		},
		{
			name:     "missing columns",
			input:    almostAllColumns,
			expected: nil,
			err:      errors.New("some columns are missing: [0 1]"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := peerdas.Blobs(blobsIndice, tc.input)
			if tc.err != nil {
				require.Equal(t, tc.err.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}
			require.DeepSSZEqual(t, tc.expected, actual)
		})
	}
}

func TestDataColumnsSidecarsBlobsRoundtrip(t *testing.T) {
	const blobCount = 5
	blobsIndex := map[uint64]bool{}

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	// Create a protobuf signed beacon block.
	signedBeaconBlockPb := util.NewBeaconBlockDeneb()

	// Generate random blobs and their corresponding commitments and proofs.
	blobs := make([]kzg.Blob, 0, blobCount)
	blobKzgCommitments := make([]*kzg.Commitment, 0, blobCount)
	blobKzgProofs := make([]*kzg.Proof, 0, blobCount)

	for blobIndex := range blobCount {
		// Create a random blob.
		blob := GetRandBlob(int64(blobIndex))
		blobs = append(blobs, blob)

		// Generate a blobKZGCommitment for the blob.
		blobKZGCommitment, proof, err := GenerateCommitmentAndProof(&blob)
		require.NoError(t, err)

		blobKzgCommitments = append(blobKzgCommitments, blobKZGCommitment)
		blobKzgProofs = append(blobKzgProofs, proof)
	}

	// Set the commitments into the block.
	blobZkgCommitmentsBytes := make([][]byte, 0, blobCount)
	for _, blobKZGCommitment := range blobKzgCommitments {
		blobZkgCommitmentsBytes = append(blobZkgCommitmentsBytes, blobKZGCommitment[:])
	}

	signedBeaconBlockPb.Block.Body.BlobKzgCommitments = blobZkgCommitmentsBytes

	// Generate verified RO blobs.
	verifiedROBlobs := make([]*blocks.VerifiedROBlob, 0, blobCount)

	// Create a signed beacon block from the protobuf.
	signedBeaconBlock, err := blocks.NewSignedBeaconBlock(signedBeaconBlockPb)
	require.NoError(t, err)

	commitmentInclusionProof, err := blocks.MerkleProofKZGCommitments(signedBeaconBlock.Block().Body())
	require.NoError(t, err)

	for blobIndex := range blobCount {
		blob := blobs[blobIndex]
		blobKZGCommitment := blobKzgCommitments[blobIndex]
		blobKzgProof := blobKzgProofs[blobIndex]

		// Get the signed beacon block header.
		signedBeaconBlockHeader, err := signedBeaconBlock.Header()
		require.NoError(t, err)

		blobSidecar := &ethpb.BlobSidecar{
			Index:                    uint64(blobIndex),
			Blob:                     blob[:],
			KzgCommitment:            blobKZGCommitment[:],
			KzgProof:                 blobKzgProof[:],
			SignedBlockHeader:        signedBeaconBlockHeader,
			CommitmentInclusionProof: commitmentInclusionProof,
		}

		roBlob, err := blocks.NewROBlob(blobSidecar)
		require.NoError(t, err)

		verifiedROBlob := blocks.NewVerifiedROBlob(roBlob)
		verifiedROBlobs = append(verifiedROBlobs, &verifiedROBlob)
	}

	// Compute data columns sidecars from the signed beacon block and from the blobs.
	dataColumnsSidecar, err := peerdas.DataColumnSidecars(signedBeaconBlock, blobs)
	require.NoError(t, err)

	// Compute the blobs from the data columns sidecar.
	roundtripBlobs, err := peerdas.Blobs(blobsIndex, dataColumnsSidecar)
	require.NoError(t, err)

	// Check that the blobs are the same.
	require.DeepSSZEqual(t, verifiedROBlobs, roundtripBlobs)
}

func TestCustodySubnetCount(t *testing.T) {
	testCases := []struct {
		name                  string
		subscribeToAllSubnets bool
		expected              uint64
	}{
		{
			name:                  "subscribeToAllSubnets=false",
			subscribeToAllSubnets: false,
			expected:              params.BeaconConfig().CustodyRequirement,
		},
		{
			name:                  "subscribeToAllSubnets=true",
			subscribeToAllSubnets: true,
			expected:              params.BeaconConfig().DataColumnSidecarSubnetCount,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set flags.
			resetFlags := flags.Get()
			defer func() {
				flags.Init(resetFlags)
			}()

			params.SetupTestConfigCleanup(t)
			gFlags := new(flags.GlobalFlags)
			gFlags.SubscribeToAllSubnets = tc.subscribeToAllSubnets
			flags.Init(gFlags)

			// Get the custody subnet count.
			actual := peerdas.CustodySubnetCount()
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestCustodyColumnCount(t *testing.T) {
	const expected uint64 = 8

	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.DataColumnSidecarSubnetCount = 32
	config.CustodyRequirement = 2
	params.OverrideBeaconConfig(config)

	actual := peerdas.CustodyColumnCount()
	require.Equal(t, expected, actual)
}

func TestHypergeomCDF(t *testing.T) {
	// Test case from https://en.wikipedia.org/wiki/Hypergeometric_distribution
	// Population size: 1000, number of successes in population: 500, sample size: 10, number of successes in sample: 5
	// Expected result: 0.072
	const (
		expected = 0.0796665913283742
		margin   = 0.000001
	)

	actual := peerdas.HypergeomCDF(5, 128, 65, 16)
	require.Equal(t, true, expected-margin <= actual && actual <= expected+margin)
}

func TestExtendedSampleCount(t *testing.T) {
	const samplesPerSlot = 16

	testCases := []struct {
		name                string
		allowedMissings     uint64
		extendedSampleCount uint64
	}{
		{name: "allowedMissings=0", allowedMissings: 0, extendedSampleCount: 16},
		{name: "allowedMissings=1", allowedMissings: 1, extendedSampleCount: 20},
		{name: "allowedMissings=2", allowedMissings: 2, extendedSampleCount: 24},
		{name: "allowedMissings=3", allowedMissings: 3, extendedSampleCount: 27},
		{name: "allowedMissings=4", allowedMissings: 4, extendedSampleCount: 29},
		{name: "allowedMissings=5", allowedMissings: 5, extendedSampleCount: 32},
		{name: "allowedMissings=6", allowedMissings: 6, extendedSampleCount: 35},
		{name: "allowedMissings=7", allowedMissings: 7, extendedSampleCount: 37},
		{name: "allowedMissings=8", allowedMissings: 8, extendedSampleCount: 40},
		{name: "allowedMissings=9", allowedMissings: 9, extendedSampleCount: 42},
		{name: "allowedMissings=10", allowedMissings: 10, extendedSampleCount: 44},
		{name: "allowedMissings=11", allowedMissings: 11, extendedSampleCount: 47},
		{name: "allowedMissings=12", allowedMissings: 12, extendedSampleCount: 49},
		{name: "allowedMissings=13", allowedMissings: 13, extendedSampleCount: 51},
		{name: "allowedMissings=14", allowedMissings: 14, extendedSampleCount: 53},
		{name: "allowedMissings=15", allowedMissings: 15, extendedSampleCount: 55},
		{name: "allowedMissings=16", allowedMissings: 16, extendedSampleCount: 57},
		{name: "allowedMissings=17", allowedMissings: 17, extendedSampleCount: 59},
		{name: "allowedMissings=18", allowedMissings: 18, extendedSampleCount: 61},
		{name: "allowedMissings=19", allowedMissings: 19, extendedSampleCount: 63},
		{name: "allowedMissings=20", allowedMissings: 20, extendedSampleCount: 65},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := peerdas.ExtendedSampleCount(samplesPerSlot, tc.allowedMissings)
			require.Equal(t, tc.extendedSampleCount, result)
		})
	}
}

func TestCustodyCountFromRecord(t *testing.T) {
	const expected uint64 = 7

	// Create an Ethereum record.
	record := &enr.Record{}
	record.Set(peerdas.Csc(expected))

	actual, err := peerdas.CustodyCountFromRecord(record)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestCanSelfReconstruct(t *testing.T) {
	testCases := []struct {
		name                   string
		totalNumberOfColumns   uint64
		custodyNumberOfColumns uint64
		expected               bool
	}{
		{
			name:                   "totalNumberOfColumns=64, custodyNumberOfColumns=31",
			totalNumberOfColumns:   64,
			custodyNumberOfColumns: 31,
			expected:               false,
		},
		{
			name:                   "totalNumberOfColumns=64, custodyNumberOfColumns=32",
			totalNumberOfColumns:   64,
			custodyNumberOfColumns: 32,
			expected:               true,
		},
		{
			name:                   "totalNumberOfColumns=65, custodyNumberOfColumns=32",
			totalNumberOfColumns:   65,
			custodyNumberOfColumns: 32,
			expected:               false,
		},
		{
			name:                   "totalNumberOfColumns=63, custodyNumberOfColumns=33",
			totalNumberOfColumns:   65,
			custodyNumberOfColumns: 33,
			expected:               true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set the total number of columns.
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.NumberOfColumns = tc.totalNumberOfColumns
			params.OverrideBeaconConfig(cfg)

			// Check if reconstuction is possible.
			actual := peerdas.CanSelfReconstruct(tc.custodyNumberOfColumns)
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
		blob := GetRandBlob(int64(i))
		commitment, _, err := GenerateCommitmentAndProof(&blob)
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
