package peerdas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/pkg/errors"
)

func TestDataColumnSidecars(t *testing.T) {
	var expected []*ethpb.DataColumnSidecar = nil
	actual, err := peerdas.DataColumnSidecars(nil, []kzg.CellsAndProofs{})
	require.NoError(t, err)

	require.DeepSSZEqual(t, expected, actual)
}

func TestBlobs(t *testing.T) {
	blobsIndice := map[uint64]bool{}

	almostAllColumns := make([]*ethpb.DataColumnSidecar, 0, fieldparams.NumberOfColumns/2)
	for i := 2; i < fieldparams.NumberOfColumns/2+2; i++ {
		almostAllColumns = append(almostAllColumns, &ethpb.DataColumnSidecar{
			Index: uint64(i),
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
		blob := getRandBlob(int64(blobIndex))
		blobs = append(blobs, blob)

		// Generate a blobKZGCommitment for the blob.
		blobKZGCommitment, proof, err := generateCommitmentAndProof(&blob)
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
	cellsAndProofs := util.GenerateCellsAndProofs(t, blobs)
	dataColumnsSidecar, err := peerdas.DataColumnSidecars(signedBeaconBlock, cellsAndProofs)
	require.NoError(t, err)

	// Compute the blobs from the data columns sidecar.
	roundtripBlobs, err := peerdas.Blobs(blobsIndex, dataColumnsSidecar)
	require.NoError(t, err)

	// Check that the blobs are the same.
	require.DeepSSZEqual(t, verifiedROBlobs, roundtripBlobs)
}

func TestValidatorsCustodyRequirement(t *testing.T) {
	testCases := []struct {
		name     string
		count    uint64
		expected uint64
	}{
		{name: "0 validators", count: 0, expected: 8},
		{name: "1 validator", count: 1, expected: 8},
		{name: "8 validators", count: 8, expected: 8},
		{name: "9 validators", count: 9, expected: 9},
		{name: "100 validators", count: 100, expected: 100},
		{name: "128 validators", count: 128, expected: 128},
		{name: "129 validators", count: 129, expected: 128},
		{name: "1000 validators", count: 1000, expected: 128},
	}

	const balance = uint64(32_000_000_000)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			balances := make([]uint64, 0, tc.count)
			for range tc.count {
				balances = append(balances, balance)
			}

			validatorsIndex := make(map[primitives.ValidatorIndex]bool)
			for i := range tc.count {
				validatorsIndex[primitives.ValidatorIndex(i)] = true
			}

			beaconState, err := state_native.InitializeFromProtoFulu(&ethpb.BeaconStateElectra{Balances: balances})
			require.NoError(t, err)

			actual, err := peerdas.ValidatorsCustodyRequirement(beaconState, validatorsIndex)
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestCustodyGroupSamplingSize(t *testing.T) {
	testCases := []struct {
		name                         string
		custodyType                  peerdas.CustodyType
		validatorsCustodyRequirement uint64
		toAdvertiseCustodyGroupCount uint64
		expected                     uint64
	}{
		{
			name:                         "target, lower than samples per slot",
			custodyType:                  peerdas.Target,
			validatorsCustodyRequirement: 2,
			expected:                     8,
		},
		{
			name:                         "target, higher than samples per slot",
			custodyType:                  peerdas.Target,
			validatorsCustodyRequirement: 100,
			expected:                     100,
		},
		{
			name:                         "actual, lower than samples per slot",
			custodyType:                  peerdas.Actual,
			validatorsCustodyRequirement: 3,
			toAdvertiseCustodyGroupCount: 4,
			expected:                     8,
		},
		{
			name:                         "actual, higher than samples per slot",
			custodyType:                  peerdas.Actual,
			validatorsCustodyRequirement: 100,
			toAdvertiseCustodyGroupCount: 101,
			expected:                     100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a custody info.
			custodyInfo := peerdas.CustodyInfo{}

			// Set the validators custody requirement for target custody group count.
			custodyInfo.TargetGroupCount.SetValidatorsCustodyRequirement(tc.validatorsCustodyRequirement)

			// Set the to advertise custody group count.
			custodyInfo.ToAdvertiseGroupCount.Set(tc.toAdvertiseCustodyGroupCount)

			// Compute the custody group sampling size.
			actual := custodyInfo.CustodyGroupSamplingSize(tc.custodyType)

			// Check the result.
			require.Equal(t, tc.expected, actual)
		})
	}
}
