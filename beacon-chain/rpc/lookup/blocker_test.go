package lookup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	mockChain "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	testDB "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/core"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/testutil"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/sirupsen/logrus"
)

func TestGetBlock(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	ctx := context.Background()

	genBlk, blkContainers := testutil.FillDBWithBlocks(ctx, t, beaconDB)
	canonicalRoots := make(map[[32]byte]bool)

	for _, bContr := range blkContainers {
		canonicalRoots[bytesutil.ToBytes32(bContr.BlockRoot)] = true
	}
	headBlock := blkContainers[len(blkContainers)-1]
	nextSlot := headBlock.GetPhase0Block().Block.Slot + 1

	b2 := util.NewBeaconBlock()
	b2.Block.Slot = 30
	b2.Block.ParentRoot = bytesutil.PadTo([]byte{1}, 32)
	util.SaveBlock(t, ctx, beaconDB, b2)
	b3 := util.NewBeaconBlock()
	b3.Block.Slot = 30
	b3.Block.ParentRoot = bytesutil.PadTo([]byte{4}, 32)
	util.SaveBlock(t, ctx, beaconDB, b3)
	b4 := util.NewBeaconBlock()
	b4.Block.Slot = nextSlot
	b4.Block.ParentRoot = bytesutil.PadTo([]byte{8}, 32)
	util.SaveBlock(t, ctx, beaconDB, b4)

	wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*ethpb.BeaconBlockContainer_Phase0Block).Phase0Block)
	require.NoError(t, err)

	fetcher := &BeaconDbBlocker{
		BeaconDB: beaconDB,
		ChainInfoFetcher: &mockChain.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &ethpb.Checkpoint{Root: blkContainers[64].BlockRoot},
			CanonicalRoots:      canonicalRoots,
		},
	}

	root, err := genBlk.Block.HashTreeRoot()
	require.NoError(t, err)

	tests := []struct {
		name    string
		blockID []byte
		want    *ethpb.SignedBeaconBlock
		wantErr bool
	}{
		{
			name:    "slot",
			blockID: []byte("30"),
			want:    blkContainers[30].Block.(*ethpb.BeaconBlockContainer_Phase0Block).Phase0Block,
		},
		{
			name:    "bad formatting",
			blockID: []byte("3bad0"),
			wantErr: true,
		},
		{
			name:    "canonical",
			blockID: []byte("30"),
			want:    blkContainers[30].Block.(*ethpb.BeaconBlockContainer_Phase0Block).Phase0Block,
		},
		{
			name:    "non canonical",
			blockID: []byte(fmt.Sprintf("%d", nextSlot)),
			want:    nil,
		},
		{
			name:    "head",
			blockID: []byte("head"),
			want:    headBlock.Block.(*ethpb.BeaconBlockContainer_Phase0Block).Phase0Block,
		},
		{
			name:    "finalized",
			blockID: []byte("finalized"),
			want:    blkContainers[64].Block.(*ethpb.BeaconBlockContainer_Phase0Block).Phase0Block,
		},
		{
			name:    "genesis",
			blockID: []byte("genesis"),
			want:    genBlk,
		},
		{
			name:    "genesis root",
			blockID: root[:],
			want:    genBlk,
		},
		{
			name:    "root",
			blockID: blkContainers[20].BlockRoot,
			want:    blkContainers[20].Block.(*ethpb.BeaconBlockContainer_Phase0Block).Phase0Block,
		},
		{
			name:    "non-existent root",
			blockID: bytesutil.PadTo([]byte("hi there"), 32),
			want:    nil,
		},
		{
			name:    "hex",
			blockID: []byte(hexutil.Encode(blkContainers[20].BlockRoot)),
			want:    blkContainers[20].Block.(*ethpb.BeaconBlockContainer_Phase0Block).Phase0Block,
		},
		{
			name:    "no block",
			blockID: []byte("105"),
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := fetcher.Block(ctx, tt.blockID)
			if tt.wantErr {
				assert.NotEqual(t, err, nil, "no error has been returned")
				return
			}
			if tt.want == nil {
				assert.Equal(t, nil, result)
				return
			}
			require.NoError(t, err)
			pb, err := result.Proto()
			require.NoError(t, err)
			pbBlock, ok := pb.(*ethpb.SignedBeaconBlock)
			require.Equal(t, true, ok)
			if !reflect.DeepEqual(pbBlock, tt.want) {
				t.Error("Expected blocks to equal")
			}
		})
	}
}

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
func getRandFieldElement(seed int64) [32]byte {
	bytes := deterministicRandomness(seed)
	var r fr.Element
	r.SetBytes(bytes[:])

	return GoKZG.SerializeScalar(r)
}

// Returns a random blob using the passed seed as entropy
func getRandBlob(seed int64) kzg.Blob {
	var blob kzg.Blob
	for i := 0; i < len(blob); i += 32 {
		fieldElementBytes := getRandFieldElement(seed + int64(i))
		copy(blob[i:i+32], fieldElementBytes[:])
	}
	return blob
}

func generateCommitmentAndProof(blob *kzg.Blob) (*kzg.Commitment, *kzg.Proof, error) {
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

func generateRandomBlocSignedBeaconBlockkAndVerifiedRoBlobs(t *testing.T, blobCount int) (interfaces.SignedBeaconBlock, []*blocks.VerifiedROBlob) {
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

	return signedBeaconBlock, verifiedROBlobs
}

func TestBlobsFromStoredDataColumns(t *testing.T) {
	const blobCount = 5

	blobsIndex := make(map[uint64]bool, blobCount)
	for i := range blobCount {
		blobsIndex[uint64(i)] = true
	}

	var (
		nilError            *core.RpcError
		noDataColumnsIndice []int
	)
	allDataColumnsIndice := make([]int, 0, fieldparams.NumberOfColumns)
	for i := range fieldparams.NumberOfColumns {
		allDataColumnsIndice = append(allDataColumnsIndice, i)
	}

	originalColumnsIndice := allDataColumnsIndice[:fieldparams.NumberOfColumns/2]
	extendedColumnsIndice := allDataColumnsIndice[fieldparams.NumberOfColumns/2:]

	testCases := []struct {
		errorReason           core.ErrorReason
		isError               bool
		subscribeToAllSubnets bool
		storedColumnsIndice   []int
		name                  string
	}{
		{
			name:                  "Cannot theoretically nor actually reconstruct",
			subscribeToAllSubnets: false,
			storedColumnsIndice:   noDataColumnsIndice,
			isError:               true,
			errorReason:           core.NotFound,
		},
		{
			name:                  "Can theoretically but not actually reconstruct",
			subscribeToAllSubnets: true,
			storedColumnsIndice:   noDataColumnsIndice,
			isError:               true,
			errorReason:           core.NotFound,
		},
		{
			name:                  "No need to reconstruct",
			subscribeToAllSubnets: true,
			storedColumnsIndice:   originalColumnsIndice,
			isError:               false,
		},
		{
			name:                  "Reconstruction needed",
			subscribeToAllSubnets: false,
			storedColumnsIndice:   extendedColumnsIndice,
			isError:               false,
		},
	}

	// Load the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	// Create a dummy signed beacon blocks and dummy verified RO blobs.
	signedBeaconBlock, verifiedRoBlobs := generateRandomBlocSignedBeaconBlockkAndVerifiedRoBlobs(t, blobCount)

	// Extract the root from the signed beacon block.
	blockRoot, err := signedBeaconBlock.Block().HashTreeRoot()
	require.NoError(t, err)

	// Extract blobs from verified RO blobs.
	blobs := make([]kzg.Blob, 0, blobCount)
	for _, verifiedRoBlob := range verifiedRoBlobs {
		blob := verifiedRoBlob.BlobSidecar.Blob
		blobs = append(blobs, kzg.Blob(blob))
	}

	// Convert blobs to data columns.
	dataColumnSidecars, err := peerdas.DataColumnSidecars(signedBeaconBlock, blobs)
	require.NoError(t, err)

	// Create verified RO data columns.
	verifiedRoDataColumns := make([]*blocks.VerifiedRODataColumn, 0, fieldparams.NumberOfColumns)
	for _, dataColumnSidecar := range dataColumnSidecars {
		roDataColumn, err := blocks.NewRODataColumn(dataColumnSidecar)
		require.NoError(t, err)

		verifiedRoDataColumn := blocks.NewVerifiedRODataColumn(roDataColumn)
		verifiedRoDataColumns = append(verifiedRoDataColumns, &verifiedRoDataColumn)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set the subscription to all subnets flags.
			resetFlags := flags.Get()
			params.SetupTestConfigCleanup(t)
			gFlags := new(flags.GlobalFlags)
			gFlags.SubscribeToAllSubnets = tc.subscribeToAllSubnets
			flags.Init(gFlags)

			// Define a blob storage.
			blobStorage := filesystem.NewEphemeralBlobStorage(t)

			// Save the data columns in the store.
			for _, columnIndex := range tc.storedColumnsIndice {
				verifiedRoDataColumn := verifiedRoDataColumns[columnIndex]
				err := blobStorage.SaveDataColumn(*verifiedRoDataColumn)
				require.NoError(t, err)
			}

			// Define the blocker.
			blocker := &BeaconDbBlocker{
				BlobStorage: blobStorage,
			}

			// Get the blobs from the data columns.
			actual, err := blocker.blobsFromStoredDataColumns(blobsIndex, blockRoot[:])
			if tc.isError {
				require.Equal(t, tc.errorReason, err.Reason)
			} else {
				require.Equal(t, nilError, err)
				expected := verifiedRoBlobs
				require.DeepSSZEqual(t, expected, actual)
			}

			// Reset flags.
			flags.Init(resetFlags)
		})
	}
}

func TestGetBlob(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.DenebForkEpoch = 1
	params.OverrideBeaconConfig(cfg)
	ctx := context.Background()
	db := testDB.SetupDB(t)
	denebBlock, blobs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 123, 4)
	require.NoError(t, db.SaveBlock(context.Background(), denebBlock))
	_, bs := filesystem.NewEphemeralBlobStorageWithFs(t)
	testSidecars, err := verification.BlobSidecarSliceNoop(blobs)
	require.NoError(t, err)
	for i := range testSidecars {
		require.NoError(t, bs.Save(testSidecars[i]))
	}
	blockRoot := blobs[0].BlockRoot()
	t.Run("genesis", func(t *testing.T) {
		blocker := &BeaconDbBlocker{}
		_, rpcErr := blocker.Blobs(ctx, "genesis", nil)
		assert.Equal(t, http.StatusBadRequest, core.ErrorReasonToHTTP(rpcErr.Reason))
		assert.StringContains(t, "blobs are not supported for Phase 0 fork", rpcErr.Err.Error())
	})
	t.Run("head", func(t *testing.T) {
		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{Root: blockRoot[:]},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "head", nil)
		assert.Equal(t, rpcErr == nil, true)
		require.Equal(t, 4, len(verifiedBlobs))
		sidecar := verifiedBlobs[0].BlobSidecar
		require.NotNil(t, sidecar)
		assert.Equal(t, uint64(0), sidecar.Index)
		assert.DeepEqual(t, blobs[0].Blob, sidecar.Blob)
		assert.DeepEqual(t, blobs[0].KzgCommitment, sidecar.KzgCommitment)
		assert.DeepEqual(t, blobs[0].KzgProof, sidecar.KzgProof)
		sidecar = verifiedBlobs[1].BlobSidecar
		require.NotNil(t, sidecar)
		assert.Equal(t, uint64(1), sidecar.Index)
		assert.DeepEqual(t, blobs[1].Blob, sidecar.Blob)
		assert.DeepEqual(t, blobs[1].KzgCommitment, sidecar.KzgCommitment)
		assert.DeepEqual(t, blobs[1].KzgProof, sidecar.KzgProof)
		sidecar = verifiedBlobs[2].BlobSidecar
		require.NotNil(t, sidecar)
		assert.Equal(t, uint64(2), sidecar.Index)
		assert.DeepEqual(t, blobs[2].Blob, sidecar.Blob)
		assert.DeepEqual(t, blobs[2].KzgCommitment, sidecar.KzgCommitment)
		assert.DeepEqual(t, blobs[2].KzgProof, sidecar.KzgProof)
		sidecar = verifiedBlobs[3].BlobSidecar
		require.NotNil(t, sidecar)
		assert.Equal(t, uint64(3), sidecar.Index)
		assert.DeepEqual(t, blobs[3].Blob, sidecar.Blob)
		assert.DeepEqual(t, blobs[3].KzgCommitment, sidecar.KzgCommitment)
		assert.DeepEqual(t, blobs[3].KzgProof, sidecar.KzgProof)
	})
	t.Run("finalized", func(t *testing.T) {
		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}

		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "finalized", nil)
		assert.Equal(t, rpcErr == nil, true)
		require.Equal(t, 4, len(verifiedBlobs))
	})
	t.Run("justified", func(t *testing.T) {
		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{CurrentJustifiedCheckPoint: &ethpb.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}

		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "justified", nil)
		assert.Equal(t, rpcErr == nil, true)
		require.Equal(t, 4, len(verifiedBlobs))
	})
	t.Run("root", func(t *testing.T) {
		blocker := &BeaconDbBlocker{
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		verifiedBlobs, rpcErr := blocker.Blobs(ctx, hexutil.Encode(blockRoot[:]), nil)
		assert.Equal(t, rpcErr == nil, true)
		require.Equal(t, 4, len(verifiedBlobs))
	})
	t.Run("slot", func(t *testing.T) {
		blocker := &BeaconDbBlocker{
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "123", nil)
		assert.Equal(t, rpcErr == nil, true)
		require.Equal(t, 4, len(verifiedBlobs))
	})
	t.Run("one blob only", func(t *testing.T) {
		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "123", map[uint64]bool{2: true})
		assert.Equal(t, rpcErr == nil, true)
		require.Equal(t, 1, len(verifiedBlobs))
		sidecar := verifiedBlobs[0].BlobSidecar
		require.NotNil(t, sidecar)
		assert.Equal(t, uint64(2), sidecar.Index)
		assert.DeepEqual(t, blobs[2].Blob, sidecar.Blob)
		assert.DeepEqual(t, blobs[2].KzgCommitment, sidecar.KzgCommitment)
		assert.DeepEqual(t, blobs[2].KzgProof, sidecar.KzgProof)
	})
	t.Run("no blobs returns an empty array", func(t *testing.T) {
		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: filesystem.NewEphemeralBlobStorage(t),
		}
		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "123", nil)
		assert.Equal(t, rpcErr == nil, true)
		require.Equal(t, 0, len(verifiedBlobs))
	})
}
