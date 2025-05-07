package peerdas_test

import (
	"crypto/rand"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/p2p/enr"
)

func TestVerifyDataColumnSidecar(t *testing.T) {
	t.Run("index too large", func(t *testing.T) {
		roSidecar := createTestSidecar(t, 1_000_000, nil, nil, nil)
		err := peerdas.VerifyDataColumnSidecar(roSidecar)
		require.ErrorIs(t, err, peerdas.ErrIndexTooLarge)
	})

	t.Run("no commitments", func(t *testing.T) {
		roSidecar := createTestSidecar(t, 0, nil, nil, nil)
		err := peerdas.VerifyDataColumnSidecar(roSidecar)
		require.ErrorIs(t, err, peerdas.ErrNoKzgCommitments)
	})

	t.Run("KZG commitments size mismatch", func(t *testing.T) {
		kzgCommitments := make([][]byte, 1)
		roSidecar := createTestSidecar(t, 0, nil, kzgCommitments, nil)
		err := peerdas.VerifyDataColumnSidecar(roSidecar)
		require.ErrorIs(t, err, peerdas.ErrMismatchLength)
	})

	t.Run("KZG proofs size mismatch", func(t *testing.T) {
		column, kzgCommitments := make([][]byte, 1), make([][]byte, 1)
		roSidecar := createTestSidecar(t, 0, column, kzgCommitments, nil)
		err := peerdas.VerifyDataColumnSidecar(roSidecar)
		require.ErrorIs(t, err, peerdas.ErrMismatchLength)
	})

	t.Run("nominal", func(t *testing.T) {
		column, kzgCommitments, kzgProofs := make([][]byte, 1), make([][]byte, 1), make([][]byte, 1)
		roSidecar := createTestSidecar(t, 0, column, kzgCommitments, kzgProofs)
		err := peerdas.VerifyDataColumnSidecar(roSidecar)
		require.NoError(t, err)
	})
}

func TestVerifyDataColumnSidecarKZGProofs(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	generateSidecars := func(t *testing.T) []*ethpb.DataColumnSidecar {
		const blobCount = int64(6)

		dbBlock := util.NewBeaconBlockDeneb()

		commitments := make([][]byte, 0, blobCount)
		blobs := make([]kzg.Blob, 0, blobCount)

		for i := range blobCount {
			blob := getRandBlob(i)
			commitment, _, err := generateCommitmentAndProof(&blob)
			require.NoError(t, err)

			commitments = append(commitments, commitment[:])
			blobs = append(blobs, blob)
		}

		dbBlock.Block.Body.BlobKzgCommitments = commitments
		sBlock, err := blocks.NewSignedBeaconBlock(dbBlock)
		require.NoError(t, err)

		cellsAndProofs := util.GenerateCellsAndProofs(t, blobs)
		sidecars, err := peerdas.DataColumnSidecars(sBlock, cellsAndProofs)
		require.NoError(t, err)

		return sidecars
	}

	generateRODataColumnSidecars := func(t *testing.T, sidecars []*ethpb.DataColumnSidecar) []blocks.RODataColumn {
		roDataColumnSidecars := make([]blocks.RODataColumn, 0, len(sidecars))
		for _, sidecar := range sidecars {
			roCol, err := blocks.NewRODataColumn(sidecar)
			require.NoError(t, err)

			roDataColumnSidecars = append(roDataColumnSidecars, roCol)
		}

		return roDataColumnSidecars
	}

	t.Run("invalid proof", func(t *testing.T) {
		sidecars := generateSidecars(t)
		sidecars[0].Column[0][0]++ // It is OK to overflow
		roDataColumnSidecars := generateRODataColumnSidecars(t, sidecars)

		err := peerdas.VerifyDataColumnsSidecarKZGProofs(roDataColumnSidecars)
		require.ErrorIs(t, err, peerdas.ErrInvalidKZGProof)
	})

	t.Run("nominal", func(t *testing.T) {
		sidecars := generateSidecars(t)
		roDataColumnSidecars := generateRODataColumnSidecars(t, sidecars)

		err := peerdas.VerifyDataColumnsSidecarKZGProofs(roDataColumnSidecars)
		require.NoError(t, err)
	})
}

func Test_VerifyKZGInclusionProofColumn(t *testing.T) {
	const (
		blobCount   = 3
		columnIndex = 0
	)

	// Generate random KZG commitments `blobCount` blobs.
	kzgCommitments := make([][]byte, blobCount)

	for i := 0; i < blobCount; i++ {
		kzgCommitments[i] = make([]byte, 48)
		_, err := rand.Read(kzgCommitments[i])
		require.NoError(t, err)
	}

	pbBody := &ethpb.BeaconBlockBodyDeneb{
		RandaoReveal: make([]byte, 96),
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
		Graffiti: make([]byte, 32),
		SyncAggregate: &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		},
		ExecutionPayload: &enginev1.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			ExtraData:     make([]byte, 0),
		},
		BlobKzgCommitments: kzgCommitments,
	}

	root, err := pbBody.HashTreeRoot()
	require.NoError(t, err)

	body, err := blocks.NewBeaconBlockBody(pbBody)
	require.NoError(t, err)

	kzgCommitmentsInclusionProof, err := blocks.MerkleProofKZGCommitments(body)
	require.NoError(t, err)

	testCases := []struct {
		name              string
		expectedError     error
		dataColumnSidecar *ethpb.DataColumnSidecar
	}{
		{
			name:              "nilSignedBlockHeader",
			expectedError:     peerdas.ErrNilBlockHeader,
			dataColumnSidecar: &ethpb.DataColumnSidecar{},
		},
		{
			name:          "nilHeader",
			expectedError: peerdas.ErrNilBlockHeader,
			dataColumnSidecar: &ethpb.DataColumnSidecar{
				SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{},
			},
		},
		{
			name:          "invalidBodyRoot",
			expectedError: peerdas.ErrBadRootLength,
			dataColumnSidecar: &ethpb.DataColumnSidecar{
				SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
					Header: &ethpb.BeaconBlockHeader{},
				},
			},
		},
		{
			name:          "unverifiedMerkleProof",
			expectedError: peerdas.ErrInvalidInclusionProof,
			dataColumnSidecar: &ethpb.DataColumnSidecar{
				SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
					Header: &ethpb.BeaconBlockHeader{
						BodyRoot: make([]byte, 32),
					},
				},
				KzgCommitments: kzgCommitments,
			},
		},
		{
			name:          "nominal",
			expectedError: nil,
			dataColumnSidecar: &ethpb.DataColumnSidecar{
				KzgCommitments: kzgCommitments,
				SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
					Header: &ethpb.BeaconBlockHeader{
						BodyRoot: root[:],
					},
				},
				KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			roDataColumn := blocks.RODataColumn{DataColumnSidecar: tc.dataColumnSidecar}
			err = peerdas.VerifyDataColumnSidecarInclusionProof(roDataColumn)
			if tc.expectedError == nil {
				require.NoError(t, err)
				return
			}

			require.ErrorIs(t, tc.expectedError, err)
		})
	}
}

func TestComputeSubnetForDataColumnSidecar(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.DataColumnSidecarSubnetCount = 128
	params.OverrideBeaconConfig(config)

	require.Equal(t, uint64(0), peerdas.ComputeSubnetForDataColumnSidecar(0))
	require.Equal(t, uint64(1), peerdas.ComputeSubnetForDataColumnSidecar(1))
	require.Equal(t, uint64(0), peerdas.ComputeSubnetForDataColumnSidecar(128))
	require.Equal(t, uint64(1), peerdas.ComputeSubnetForDataColumnSidecar(129))
}

func TestDataColumnSubnets(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.DataColumnSidecarSubnetCount = 128
	params.OverrideBeaconConfig(config)

	input := map[uint64]bool{0: true, 1: true, 128: true, 129: true, 131: true}
	expected := map[uint64]bool{0: true, 1: true, 3: true}
	actual := peerdas.DataColumnSubnets(input)

	require.Equal(t, len(expected), len(actual))
	for k, v := range expected {
		require.Equal(t, v, actual[k])
	}
}

func TestCustodyGroupCountFromRecord(t *testing.T) {
	t.Run("nil record", func(t *testing.T) {
		_, err := peerdas.CustodyGroupCountFromRecord(nil)
		require.ErrorIs(t, err, peerdas.ErrRecordNil)
	})

	t.Run("no cgc", func(t *testing.T) {
		_, err := peerdas.CustodyGroupCountFromRecord(&enr.Record{})
		require.ErrorIs(t, err, peerdas.ErrCannotLoadCustodyGroupCount)
	})

	t.Run("nominal", func(t *testing.T) {
		const expected uint64 = 7

		record := &enr.Record{}
		record.Set(peerdas.Cgc(expected))

		actual, err := peerdas.CustodyGroupCountFromRecord(record)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})
}

func createTestSidecar(t *testing.T, index uint64, column, kzgCommitments, kzgProofs [][]byte) blocks.RODataColumn {
	pbSignedBeaconBlock := util.NewBeaconBlockDeneb()
	signedBeaconBlock, err := blocks.NewSignedBeaconBlock(pbSignedBeaconBlock)
	require.NoError(t, err)

	signedBlockHeader, err := signedBeaconBlock.Header()
	require.NoError(t, err)

	sidecar := &ethpb.DataColumnSidecar{
		Index:             index,
		Column:            column,
		KzgCommitments:    kzgCommitments,
		KzgProofs:         kzgProofs,
		SignedBlockHeader: signedBlockHeader,
	}

	roSidecar, err := blocks.NewRODataColumn(sidecar)
	require.NoError(t, err)

	return roSidecar
}
