package peerdas_test

import (
	"crypto/rand"
	"fmt"
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
	testCases := []struct {
		name             string
		index            uint64
		blobCount        int
		commitmentCount  int
		proofCount       int
		maxBlobsPerBlock uint64
		expectedError    error
	}{
		{name: "index too large", index: 1_000_000, expectedError: peerdas.ErrIndexTooLarge},
		{name: "no commitments", expectedError: peerdas.ErrNoKzgCommitments},
		{name: "too many commitments", blobCount: 10, commitmentCount: 10, proofCount: 10, maxBlobsPerBlock: 2, expectedError: peerdas.ErrTooManyCommitments},
		{name: "commitments size mismatch", commitmentCount: 1, maxBlobsPerBlock: 1, expectedError: peerdas.ErrMismatchLength},
		{name: "proofs size mismatch", blobCount: 1, commitmentCount: 1, maxBlobsPerBlock: 1, expectedError: peerdas.ErrMismatchLength},
		{name: "nominal", blobCount: 1, commitmentCount: 1, proofCount: 1, maxBlobsPerBlock: 1, expectedError: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig()
			cfg.FuluForkEpoch = 0
			cfg.BlobSchedule = []params.BlobScheduleEntry{{Epoch: 0, MaxBlobsPerBlock: tc.maxBlobsPerBlock}}
			params.OverrideBeaconConfig(cfg)

			column := make([][]byte, tc.blobCount)
			kzgCommitments := make([][]byte, tc.commitmentCount)
			kzgProof := make([][]byte, tc.proofCount)

			roSidecar := createTestSidecar(t, tc.index, column, kzgCommitments, kzgProof)
			err := peerdas.VerifyDataColumnSidecar(roSidecar)

			if tc.expectedError != nil {
				require.ErrorIs(t, err, tc.expectedError)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestVerifyDataColumnSidecarKZGProofs(t *testing.T) {
	const (
		blobCount = 6
		seed      = 0
	)
	err := kzg.Start()
	require.NoError(t, err)

	t.Run("size mismatch", func(t *testing.T) {
		sidecars := generateRandomSidecars(t, seed, blobCount)
		sidecars[0].Column[0] = sidecars[0].Column[0][:len(sidecars[0].Column[0])-1] // Remove one byte to create size mismatch

		err := peerdas.VerifyDataColumnsSidecarKZGProofs(sidecars)
		require.ErrorIs(t, err, peerdas.ErrMismatchLength)
	})

	t.Run("invalid proof", func(t *testing.T) {
		sidecars := generateRandomSidecars(t, seed, blobCount)
		sidecars[0].Column[0][0]++ // It is OK to overflow

		err := peerdas.VerifyDataColumnsSidecarKZGProofs(sidecars)
		require.ErrorIs(t, err, peerdas.ErrInvalidKZGProof)
	})

	t.Run("nominal", func(t *testing.T) {
		sidecars := generateRandomSidecars(t, seed, blobCount)
		err := peerdas.VerifyDataColumnsSidecarKZGProofs(sidecars)
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

func BenchmarkVerifyDataColumnSidecarKZGProofs_SameCommitments_NoBatch(b *testing.B) {
	const blobCount = 12
	err := kzg.Start()
	require.NoError(b, err)

	b.StopTimer()
	b.ResetTimer()
	for i := range int64(b.N) {
		// Generate new random sidecars to ensure the KZG backend does not cache anything.
		sidecars := generateRandomSidecars(b, i, blobCount)

		for _, sidecar := range sidecars {
			sidecars := []blocks.RODataColumn{sidecar}
			b.StartTimer()
			err := peerdas.VerifyDataColumnsSidecarKZGProofs(sidecars)
			b.StopTimer()
			require.NoError(b, err)
		}
	}
}

func BenchmarkVerifyDataColumnSidecarKZGProofs_DiffCommitments_Batch(b *testing.B) {
	const blobCount = 12
	numberOfColumns := int64(params.BeaconConfig().NumberOfColumns)
	err := kzg.Start()
	require.NoError(b, err)

	columnsCounts := []int64{1, 2, 4, 8, 16, 32, 64, 128}

	for i, columnsCount := range columnsCounts {
		b.Run(fmt.Sprintf("columnsCount_%d", columnsCount), func(b *testing.B) {
			b.StopTimer()
			b.ResetTimer()

			for j := range int64(b.N) {
				allSidecars := make([]blocks.RODataColumn, 0, numberOfColumns)
				for k := int64(0); k < numberOfColumns; k += columnsCount {
					// Use different seeds to generate different blobs/commitments
					seed := int64(b.N*i) + numberOfColumns*j + blobCount*k
					sidecars := generateRandomSidecars(b, seed, blobCount)

					// Pick sidecars.
					allSidecars = append(allSidecars, sidecars[k:k+columnsCount]...)
				}

				b.StartTimer()
				err := peerdas.VerifyDataColumnsSidecarKZGProofs(allSidecars)
				b.StopTimer()
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkVerifyDataColumnSidecarKZGProofs_DiffCommitments_Batch4(b *testing.B) {
	const (
		blobCount = 12

		// columnsCount*batchCount = 128
		columnsCount = 4
		batchCount   = 32
	)

	err := kzg.Start()
	require.NoError(b, err)

	b.StopTimer()
	b.ResetTimer()

	for i := range int64(b.N) {
		allSidecars := make([][]blocks.RODataColumn, 0, batchCount)
		for j := range int64(batchCount) {
			// Use different seeds to generate different blobs/commitments
			sidecars := generateRandomSidecars(b, int64(batchCount)*i+j*blobCount, blobCount)
			allSidecars = append(allSidecars, sidecars)
		}

		for _, sidecars := range allSidecars {
			b.StartTimer()
			err := peerdas.VerifyDataColumnsSidecarKZGProofs(sidecars)
			b.StopTimer()
			require.NoError(b, err)
		}
	}
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

func generateRandomSidecars(t testing.TB, seed, blobCount int64) []blocks.RODataColumn {
	dbBlock := util.NewBeaconBlockDeneb()

	commitments := make([][]byte, 0, blobCount)
	blobs := make([]kzg.Blob, 0, blobCount)

	for i := range blobCount {
		subSeed := seed + i
		blob := getRandBlob(subSeed)
		commitment, err := generateCommitment(&blob)
		require.NoError(t, err)

		commitments = append(commitments, commitment[:])
		blobs = append(blobs, blob)
	}

	dbBlock.Block.Body.BlobKzgCommitments = commitments
	sBlock, err := blocks.NewSignedBeaconBlock(dbBlock)
	require.NoError(t, err)

	cellsAndProofs := util.GenerateCellsAndProofs(t, blobs)
	rob, err := blocks.NewROBlock(sBlock)
	require.NoError(t, err)
	sidecars, err := peerdas.DataColumnSidecars(cellsAndProofs, peerdas.PopulateFromBlock(rob))
	require.NoError(t, err)

	return sidecars
}
