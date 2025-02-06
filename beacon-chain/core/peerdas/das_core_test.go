package peerdas_test

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

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
	dataColumnsSidecar, err := peerdas.DataColumnSidecars(signedBeaconBlock, blobs)
	require.NoError(t, err)

	// Compute the blobs from the data columns sidecar.
	roundtripBlobs, err := peerdas.Blobs(blobsIndex, dataColumnsSidecar)
	require.NoError(t, err)

	// Check that the blobs are the same.
	require.DeepSSZEqual(t, verifiedROBlobs, roundtripBlobs)
}
