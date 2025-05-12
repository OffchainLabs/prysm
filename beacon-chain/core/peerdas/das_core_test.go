package peerdas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/pkg/errors"
)

func TestCustodyGroups(t *testing.T) {
	// The happy path is unit tested in spec tests.
	numberOfCustodyGroup := params.BeaconConfig().NumberOfCustodyGroups
	_, err := peerdas.CustodyGroups(enode.ID{}, numberOfCustodyGroup+1)
	require.ErrorIs(t, err, peerdas.ErrCustodyGroupCountTooLarge)
}

func TestComputeColumnsForCustodyGroup(t *testing.T) {
	// The happy path is unit tested in spec tests.
	numberOfCustodyGroup := params.BeaconConfig().NumberOfCustodyGroups
	_, err := peerdas.ComputeColumnsForCustodyGroup(numberOfCustodyGroup)
	require.ErrorIs(t, err, peerdas.ErrCustodyGroupTooLarge)
}

func TestDataColumnSidecars(t *testing.T) {
	t.Run("nil signed block", func(t *testing.T) {
		var expected []*ethpb.DataColumnSidecar = nil
		actual, err := peerdas.DataColumnSidecars(nil, []kzg.CellsAndProofs{})
		require.NoError(t, err)

		require.DeepSSZEqual(t, expected, actual)
	})

	t.Run("empty cells and proofs", func(t *testing.T) {
		// Create a protobuf signed beacon block.
		signedBeaconBlockPb := util.NewBeaconBlockDeneb()

		// Create a signed beacon block from the protobuf.
		signedBeaconBlock, err := blocks.NewSignedBeaconBlock(signedBeaconBlockPb)
		require.NoError(t, err)

		actual, err := peerdas.DataColumnSidecars(signedBeaconBlock, []kzg.CellsAndProofs{})
		require.NoError(t, err)
		require.IsNil(t, actual)
	})

	t.Run("sizes mismatch", func(t *testing.T) {
		// Create a protobuf signed beacon block.
		signedBeaconBlockPb := util.NewBeaconBlockDeneb()

		// Create a signed beacon block from the protobuf.
		signedBeaconBlock, err := blocks.NewSignedBeaconBlock(signedBeaconBlockPb)
		require.NoError(t, err)

		// Create cells and proofs.
		cellsAndProofs := make([]kzg.CellsAndProofs, 1)

		_, err = peerdas.DataColumnSidecars(signedBeaconBlock, cellsAndProofs)
		require.ErrorIs(t, err, peerdas.ErrMismatchSize)
	})
}

// --------------------------------------------------------------------------------------------------------------------------------------
// DataColumnsSidecarsFromItems is tested as part of the DataColumnSidecars tests, in the TestDataColumnsSidecarsBlobsRoundtrip function.
// --------------------------------------------------------------------------------------------------------------------------------------

func TestComputeCustodyGroupForColumn(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.NumberOfColumns = 128
	config.NumberOfCustodyGroups = 64
	params.OverrideBeaconConfig(config)

	t.Run("index too large", func(t *testing.T) {
		_, err := peerdas.ComputeCustodyGroupForColumn(1_000_000)
		require.ErrorIs(t, err, peerdas.ErrIndexTooLarge)
	})

	t.Run("nominal", func(t *testing.T) {
		expected := uint64(2)
		actual, err := peerdas.ComputeCustodyGroupForColumn(2)
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		expected = uint64(3)
		actual, err = peerdas.ComputeCustodyGroupForColumn(3)
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		expected = uint64(2)
		actual, err = peerdas.ComputeCustodyGroupForColumn(66)
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		expected = uint64(3)
		actual, err = peerdas.ComputeCustodyGroupForColumn(67)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})
}

func TestBlobs(t *testing.T) {
	blobsIndice := map[uint64]bool{}

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	almostAllColumns := make([]*ethpb.DataColumnSidecar, 0, numberOfColumns/2)
	for i := uint64(2); i < numberOfColumns/2+2; i++ {
		almostAllColumns = append(almostAllColumns, &ethpb.DataColumnSidecar{
			Index: i,
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

func TestCustodyColumns(t *testing.T) {
	t.Run("group too large", func(t *testing.T) {
		_, err := peerdas.CustodyColumns([]uint64{1_000_000})
		require.ErrorIs(t, err, peerdas.ErrCustodyGroupTooLarge)
	})

	t.Run("nominal", func(t *testing.T) {
		input := []uint64{1, 2}
		expected := map[uint64]bool{1: true, 2: true}

		actual, err := peerdas.CustodyColumns(input)
		require.NoError(t, err)
		require.Equal(t, len(expected), len(actual))
		for i := range actual {
			require.Equal(t, expected[i], actual[i])
		}
	})
}
