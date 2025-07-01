package lookup

import (
	"fmt"
	"math"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/testutil"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestGetBlock(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	ctx := t.Context()

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

func TestGetBlob(t *testing.T) {
	const (
		slot          = 123
		blobCount     = 4
		denebForEpoch = 1
		fuluForkEpoch = 2
	)

	setupDeneb := func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.DenebForkEpoch = denebForEpoch
		params.OverrideBeaconConfig(cfg)
	}

	setupFulu := func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.DenebForkEpoch = denebForEpoch
		cfg.FuluForkEpoch = fuluForkEpoch
		params.OverrideBeaconConfig(cfg)
	}

	ctx := t.Context()
	db := testDB.SetupDB(t)

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	// Create and save Deneb block and blob sidecars.
	_, blobStorage := filesystem.NewEphemeralBlobStorageAndFs(t)

	denebBlock, storedBlobSidecars := util.GenerateTestDenebBlockWithSidecar(t, [fieldparams.RootLength]byte{}, slot, blobCount)
	denebBlockRoot := denebBlock.Root()

	verifiedStoredSidecars := verification.FakeVerifySliceForTest(t, storedBlobSidecars)
	for i := range verifiedStoredSidecars {
		err := blobStorage.Save(verifiedStoredSidecars[i])
		require.NoError(t, err)
	}

	err = db.SaveBlock(t.Context(), denebBlock)
	require.NoError(t, err)

	// Create Electra block and blob sidecars. (Electra block = Fulu block),
	// save the block, convert blob sidecars to data column sidecars and save the block.
	fuluForkSlot := fuluForkEpoch * params.BeaconConfig().SlotsPerEpoch
	fuluBlock, fuluBlobSidecars := util.GenerateTestElectraBlockWithSidecar(t, [fieldparams.RootLength]byte{}, fuluForkSlot, blobCount)
	fuluBlockRoot := fuluBlock.Root()

	cellsAndProofsList := make([]kzg.CellsAndProofs, 0, len(fuluBlobSidecars))
	for _, blob := range fuluBlobSidecars {
		var kzgBlob kzg.Blob
		copy(kzgBlob[:], blob.Blob)
		cellsAndProogs, err := kzg.ComputeCellsAndKZGProofs(&kzgBlob)
		require.NoError(t, err)
		cellsAndProofsList = append(cellsAndProofsList, cellsAndProogs)
	}

	dataColumnSidecarPb, err := peerdas.DataColumnSidecars(fuluBlock, cellsAndProofsList)
	require.NoError(t, err)

	verifiedRoDataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, len(dataColumnSidecarPb))
	for _, sidecarPb := range dataColumnSidecarPb {
		roDataColumn, err := blocks.NewRODataColumnWithRoot(sidecarPb, fuluBlockRoot)
		require.NoError(t, err)

		verifiedRoDataColumn := blocks.NewVerifiedRODataColumn(roDataColumn)
		verifiedRoDataColumnSidecars = append(verifiedRoDataColumnSidecars, verifiedRoDataColumn)
	}

	err = db.SaveBlock(t.Context(), fuluBlock)
	require.NoError(t, err)

	t.Run("genesis", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{}
		_, rpcErr := blocker.Blobs(ctx, "genesis", nil)
		require.Equal(t, http.StatusBadRequest, core.ErrorReasonToHTTP(rpcErr.Reason))
		require.StringContains(t, "blobs are not supported for Phase 0 fork", rpcErr.Err.Error())
	})

	t.Run("head", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{Root: denebBlockRoot[:]},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: blobStorage,
		}

		retrievedVerifiedSidecars, rpcErr := blocker.Blobs(ctx, "head", nil)
		require.IsNil(t, rpcErr)
		require.Equal(t, blobCount, len(retrievedVerifiedSidecars))

		for i := range blobCount {
			expected := verifiedStoredSidecars[i]

			actual := retrievedVerifiedSidecars[i].BlobSidecar
			require.NotNil(t, actual)

			require.Equal(t, expected.Index, actual.Index)
			require.DeepEqual(t, expected.Blob, actual.Blob)
			require.DeepEqual(t, expected.KzgCommitment, actual.KzgCommitment)
			require.DeepEqual(t, expected.KzgProof, actual.KzgProof)
		}
	})

	t.Run("finalized", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Root: denebBlockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: blobStorage,
		}

		verifiedSidecars, rpcErr := blocker.Blobs(ctx, "finalized", nil)
		require.IsNil(t, rpcErr)
		require.Equal(t, blobCount, len(verifiedSidecars))
	})

	t.Run("justified", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{CurrentJustifiedCheckPoint: &ethpb.Checkpoint{Root: denebBlockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: blobStorage,
		}

		verifiedSidecars, rpcErr := blocker.Blobs(ctx, "justified", nil)
		require.IsNil(t, rpcErr)
		require.Equal(t, blobCount, len(verifiedSidecars))
	})

	t.Run("root", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: blobStorage,
		}

		verifiedBlobs, rpcErr := blocker.Blobs(ctx, hexutil.Encode(denebBlockRoot[:]), nil)
		require.IsNil(t, rpcErr)
		require.Equal(t, blobCount, len(verifiedBlobs))
	})

	t.Run("slot", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: blobStorage,
		}

		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "123", nil)
		require.IsNil(t, rpcErr)
		require.Equal(t, blobCount, len(verifiedBlobs))
	})

	t.Run("one blob only", func(t *testing.T) {
		const index = 2

		setupDeneb(t)

		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Root: denebBlockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: blobStorage,
		}

		retrievedVerifiedSidecars, rpcErr := blocker.Blobs(ctx, "123", []int{index})
		require.IsNil(t, rpcErr)
		require.Equal(t, 1, len(retrievedVerifiedSidecars))

		expected := verifiedStoredSidecars[index]
		actual := retrievedVerifiedSidecars[0].BlobSidecar
		require.NotNil(t, actual)

		require.Equal(t, uint64(index), actual.Index)
		require.DeepEqual(t, expected.Blob, actual.Blob)
		require.DeepEqual(t, expected.KzgCommitment, actual.KzgCommitment)
		require.DeepEqual(t, expected.KzgProof, actual.KzgProof)
	})

	t.Run("no blobs returns an empty array", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Root: denebBlockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: filesystem.NewEphemeralBlobStorage(t),
		}

		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "123", nil)
		require.IsNil(t, rpcErr)
		require.Equal(t, 0, len(verifiedBlobs))
	})

	t.Run("no blob at index", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Root: denebBlockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: blobStorage,
		}

		noBlobIndex := len(storedBlobSidecars) + 1
		_, rpcErr := blocker.Blobs(ctx, "123", []int{0, noBlobIndex})
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.NotFound), rpcErr.Reason)
	})

	t.Run("index too big", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Root: denebBlockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: blobStorage,
		}
		_, rpcErr := blocker.Blobs(ctx, "123", []int{0, math.MaxInt})
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.BadRequest), rpcErr.Reason)
	})

	t.Run("not enough stored data column sidecars", func(t *testing.T) {
		setupFulu(t)

		_, dataColumnStorage := filesystem.NewEphemeralDataColumnStorageAndFs(t)
		err = dataColumnStorage.Save(verifiedRoDataColumnSidecars[:fieldparams.CellsPerBlob-1])
		require.NoError(t, err)

		blocker := &BeaconDbBlocker{
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:          db,
			BlobStorage:       blobStorage,
			DataColumnStorage: dataColumnStorage,
		}

		_, rpcErr := blocker.Blobs(ctx, hexutil.Encode(fuluBlockRoot[:]), nil)
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.NotFound), rpcErr.Reason)
	})

	t.Run("reconstruction needed", func(t *testing.T) {
		setupFulu(t)

		_, dataColumnStorage := filesystem.NewEphemeralDataColumnStorageAndFs(t)
		err = dataColumnStorage.Save(verifiedRoDataColumnSidecars[1 : peerdas.MinimumColumnsCountToReconstruct()+1])
		require.NoError(t, err)

		blocker := &BeaconDbBlocker{
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:          db,
			BlobStorage:       blobStorage,
			DataColumnStorage: dataColumnStorage,
		}

		retrievedVerifiedRoBlobs, rpcErr := blocker.Blobs(ctx, hexutil.Encode(fuluBlockRoot[:]), nil)
		require.IsNil(t, rpcErr)
		require.Equal(t, len(fuluBlobSidecars), len(retrievedVerifiedRoBlobs))

		for i, retrievedVerifiedRoBlob := range retrievedVerifiedRoBlobs {
			retrievedBlobSidecarPb := retrievedVerifiedRoBlob.BlobSidecar
			initialBlobSidecarPb := fuluBlobSidecars[i].BlobSidecar
			require.DeepSSZEqual(t, initialBlobSidecarPb, retrievedBlobSidecarPb)
		}
	})

	t.Run("no reconstruction needed", func(t *testing.T) {
		setupFulu(t)

		_, dataColumnStorage := filesystem.NewEphemeralDataColumnStorageAndFs(t)
		err = dataColumnStorage.Save(verifiedRoDataColumnSidecars)
		require.NoError(t, err)

		blocker := &BeaconDbBlocker{
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:          db,
			BlobStorage:       blobStorage,
			DataColumnStorage: dataColumnStorage,
		}

		retrievedVerifiedRoBlobs, rpcErr := blocker.Blobs(ctx, hexutil.Encode(fuluBlockRoot[:]), nil)
		require.IsNil(t, rpcErr)
		require.Equal(t, len(fuluBlobSidecars), len(retrievedVerifiedRoBlobs))

		for i, retrievedVerifiedRoBlob := range retrievedVerifiedRoBlobs {
			retrievedBlobSidecarPb := retrievedVerifiedRoBlob.BlobSidecar
			initialBlobSidecarPb := fuluBlobSidecars[i].BlobSidecar
			require.DeepSSZEqual(t, initialBlobSidecarPb, retrievedBlobSidecarPb)
		}
	})
}
