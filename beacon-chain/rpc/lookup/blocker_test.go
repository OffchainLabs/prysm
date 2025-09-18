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
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/options"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/testutil"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
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
			DB:                         beaconDB,
			Block:                      wsb,
			Root:                       headBlock.BlockRoot,
			FinalizedCheckPoint:        &ethpb.Checkpoint{Root: blkContainers[64].BlockRoot},
			CurrentJustifiedCheckPoint: &ethpb.Checkpoint{Root: blkContainers[32].BlockRoot},
			CanonicalRoots:             canonicalRoots,
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
			name:    "justified",
			blockID: []byte("justified"),
			want:    blkContainers[32].Block.(*ethpb.BeaconBlockContainer_Phase0Block).Phase0Block,
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

func TestBlobsErrorHandling(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.DenebForkEpoch = 1
	params.OverrideBeaconConfig(cfg)

	ctx := t.Context()
	db := testDB.SetupDB(t)

	t.Run("non-existent block by root returns 404", func(t *testing.T) {
		blocker := &BeaconDbBlocker{
			BeaconDB: db,
		}

		_, rpcErr := blocker.Blobs(ctx, "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.NotFound), rpcErr.Reason)
		require.StringContains(t, "not found", rpcErr.Err.Error())
	})

	t.Run("non-existent block by slot returns 404", func(t *testing.T) {
		blocker := &BeaconDbBlocker{
			BeaconDB: db,
			ChainInfoFetcher: &mockChain.ChainService{},
		}

		_, rpcErr := blocker.Blobs(ctx, "999999")
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.NotFound), rpcErr.Reason)
		require.StringContains(t, "no blocks found at slot", rpcErr.Err.Error())
	})

	t.Run("genesis block not found returns 404", func(t *testing.T) {
		blocker := &BeaconDbBlocker{
			BeaconDB: db,
		}

		// Note: genesis blocks don't support blobs, so this returns BadRequest
		_, rpcErr := blocker.Blobs(ctx, "genesis")
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.BadRequest), rpcErr.Reason)
		require.StringContains(t, "not supported for Phase 0", rpcErr.Err.Error())
	})

	t.Run("finalized block not found returns 404", func(t *testing.T) {
		// Set up a finalized checkpoint pointing to a non-existent block
		nonExistentRoot := bytesutil.PadTo([]byte("nonexistent"), 32)
		blocker := &BeaconDbBlocker{
			BeaconDB: db,
			ChainInfoFetcher: &mockChain.ChainService{
				FinalizedCheckPoint: &ethpb.Checkpoint{Root: nonExistentRoot},
			},
		}

		_, rpcErr := blocker.Blobs(ctx, "finalized")
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.NotFound), rpcErr.Reason)
		require.StringContains(t, "finalized block", rpcErr.Err.Error())
		require.StringContains(t, "not found", rpcErr.Err.Error())
	})

	t.Run("justified block not found returns 404", func(t *testing.T) {
		// Set up a justified checkpoint pointing to a non-existent block
		nonExistentRoot := bytesutil.PadTo([]byte("nonexistent2"), 32)
		blocker := &BeaconDbBlocker{
			BeaconDB: db,
			ChainInfoFetcher: &mockChain.ChainService{
				CurrentJustifiedCheckPoint: &ethpb.Checkpoint{Root: nonExistentRoot},
			},
		}

		_, rpcErr := blocker.Blobs(ctx, "justified")
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.NotFound), rpcErr.Reason)
		require.StringContains(t, "justified block", rpcErr.Err.Error())
		require.StringContains(t, "not found", rpcErr.Err.Error())
	})

	t.Run("invalid block ID returns 400", func(t *testing.T) {
		blocker := &BeaconDbBlocker{
			BeaconDB: db,
		}

		_, rpcErr := blocker.Blobs(ctx, "invalid-hex")
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.BadRequest), rpcErr.Reason)
		require.StringContains(t, "could not parse block ID", rpcErr.Err.Error())
	})

	t.Run("database error returns 500", func(t *testing.T) {
		// Create a pre-Deneb block with valid slot
		predenebBlock := util.NewBeaconBlock()
		predenebBlock.Block.Slot = 100
		util.SaveBlock(t, ctx, db, predenebBlock)
		
		// Create blocker without ChainInfoFetcher to trigger internal error when checking canonical status
		blocker := &BeaconDbBlocker{
			BeaconDB: db,
		}

		_, rpcErr := blocker.Blobs(ctx, "100")
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.Internal), rpcErr.Reason)
	})
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

	roDataColumnSidecars, err := peerdas.DataColumnSidecars(cellsAndProofsList, peerdas.PopulateFromBlock(fuluBlock))
	require.NoError(t, err)

	verifiedRoDataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, len(roDataColumnSidecars))
	for _, roDataColumnSidecar := range roDataColumnSidecars {
		verifiedRoDataColumnSidecar := blocks.NewVerifiedRODataColumn(roDataColumnSidecar)
		verifiedRoDataColumnSidecars = append(verifiedRoDataColumnSidecars, verifiedRoDataColumnSidecar)
	}

	err = db.SaveBlock(t.Context(), fuluBlock)
	require.NoError(t, err)

	t.Run("genesis", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{}
		_, rpcErr := blocker.Blobs(ctx, "genesis")
		require.Equal(t, http.StatusBadRequest, core.ErrorReasonToHTTP(rpcErr.Reason))
		require.StringContains(t, "not supported for Phase 0 fork", rpcErr.Err.Error())
	})

	t.Run("head", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{
				Root:  denebBlockRoot[:],
				Block: denebBlock,
			},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: blobStorage,
		}

		retrievedVerifiedSidecars, rpcErr := blocker.Blobs(ctx, "head")
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

		verifiedSidecars, rpcErr := blocker.Blobs(ctx, "finalized")
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

		verifiedSidecars, rpcErr := blocker.Blobs(ctx, "justified")
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

		verifiedBlobs, rpcErr := blocker.Blobs(ctx, hexutil.Encode(denebBlockRoot[:]))
		require.IsNil(t, rpcErr)
		require.Equal(t, blobCount, len(verifiedBlobs))
	})

	t.Run("slot", func(t *testing.T) {
		setupDeneb(t)

		blocker := &BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: blobStorage,
		}

		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "123")
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

		retrievedVerifiedSidecars, rpcErr := blocker.Blobs(ctx, "123", options.WithIndices([]int{index}))
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

		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "123")
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
		_, rpcErr := blocker.Blobs(ctx, "123", options.WithIndices([]int{0, noBlobIndex}))
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
		_, rpcErr := blocker.Blobs(ctx, "123", options.WithIndices([]int{0, math.MaxInt}))
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

		_, rpcErr := blocker.Blobs(ctx, hexutil.Encode(fuluBlockRoot[:]))
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.NotFound), rpcErr.Reason)
	})

	t.Run("reconstruction needed", func(t *testing.T) {
		setupFulu(t)

		_, dataColumnStorage := filesystem.NewEphemeralDataColumnStorageAndFs(t)
		err = dataColumnStorage.Save(verifiedRoDataColumnSidecars[1 : peerdas.MinimumColumnCountToReconstruct()+1])
		require.NoError(t, err)

		blocker := &BeaconDbBlocker{
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:          db,
			BlobStorage:       blobStorage,
			DataColumnStorage: dataColumnStorage,
		}

		retrievedVerifiedRoBlobs, rpcErr := blocker.Blobs(ctx, hexutil.Encode(fuluBlockRoot[:]))
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

		retrievedVerifiedRoBlobs, rpcErr := blocker.Blobs(ctx, hexutil.Encode(fuluBlockRoot[:]))
		require.IsNil(t, rpcErr)
		require.Equal(t, len(fuluBlobSidecars), len(retrievedVerifiedRoBlobs))

		for i, retrievedVerifiedRoBlob := range retrievedVerifiedRoBlobs {
			retrievedBlobSidecarPb := retrievedVerifiedRoBlob.BlobSidecar
			initialBlobSidecarPb := fuluBlobSidecars[i].BlobSidecar
			require.DeepSSZEqual(t, initialBlobSidecarPb, retrievedBlobSidecarPb)
		}
	})

	t.Run("pre-deneb block should return 400", func(t *testing.T) {
		// Setup with Deneb fork at epoch 1, so slot 0 is before Deneb
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.DenebForkEpoch = 1
		params.OverrideBeaconConfig(cfg)

		// Create a pre-Deneb block (slot 0, which is epoch 0)
		predenebBlock := util.NewBeaconBlock()
		predenebBlock.Block.Slot = 0
		util.SaveBlock(t, ctx, db, predenebBlock)
		predenebBlockRoot, err := predenebBlock.Block.HashTreeRoot()
		require.NoError(t, err)

		blocker := &BeaconDbBlocker{
			BeaconDB: db,
		}

		_, rpcErr := blocker.Blobs(ctx, hexutil.Encode(predenebBlockRoot[:]))
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.BadRequest), rpcErr.Reason)
		require.Equal(t, http.StatusBadRequest, core.ErrorReasonToHTTP(rpcErr.Reason))
		require.StringContains(t, "not supported before", rpcErr.Err.Error())
	})
}

func TestBlobs_CommitmentOrdering(t *testing.T) {
	// Set up Fulu fork configuration
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.DenebForkEpoch = 1
	cfg.FuluForkEpoch = 2
	params.OverrideBeaconConfig(cfg)

	beaconDB := testDB.SetupDB(t)
	ctx := t.Context()

	// Start the trusted setup for KZG
	err := kzg.Start()
	require.NoError(t, err)

	// Create Fulu/Electra block with multiple blob commitments
	fuluForkSlot := primitives.Slot(cfg.FuluForkEpoch) * params.BeaconConfig().SlotsPerEpoch
	parent := [32]byte{}
	fuluBlock, fuluBlobs := util.GenerateTestElectraBlockWithSidecar(t, parent, fuluForkSlot, 3)

	// Save the block
	err = beaconDB.SaveBlock(ctx, fuluBlock)
	require.NoError(t, err)
	fuluBlockRoot := fuluBlock.Root()

	// Get the commitments from the generated block
	commitments, err := fuluBlock.Block().Body().BlobKzgCommitments()
	require.NoError(t, err)
	require.Equal(t, 3, len(commitments))

	// Convert blob sidecars to data column sidecars for Fulu
	cellsAndProofsList := make([]kzg.CellsAndProofs, 0, len(fuluBlobs))
	for _, blob := range fuluBlobs {
		var kzgBlob kzg.Blob
		copy(kzgBlob[:], blob.Blob)
		cellsAndProofs, err := kzg.ComputeCellsAndKZGProofs(&kzgBlob)
		require.NoError(t, err)
		cellsAndProofsList = append(cellsAndProofsList, cellsAndProofs)
	}

	dataColumnSidecarPb, err := peerdas.DataColumnSidecars(cellsAndProofsList, peerdas.PopulateFromBlock(fuluBlock))
	require.NoError(t, err)

	verifiedRoDataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, len(dataColumnSidecarPb))
	for _, roDataColumn := range dataColumnSidecarPb {
		verifiedRoDataColumn := blocks.NewVerifiedRODataColumn(roDataColumn)
		verifiedRoDataColumnSidecars = append(verifiedRoDataColumnSidecars, verifiedRoDataColumn)
	}

	// Set up data column storage and save data columns
	_, dataColumnStorage := filesystem.NewEphemeralDataColumnStorageAndFs(t)
	err = dataColumnStorage.Save(verifiedRoDataColumnSidecars)
	require.NoError(t, err)

	// Set up the blocker
	chainService := &mockChain.ChainService{
		Genesis: time.Now(),
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  fuluBlockRoot[:],
		},
	}
	blocker := &BeaconDbBlocker{
		BeaconDB:           beaconDB,
		ChainInfoFetcher:   chainService,
		GenesisTimeFetcher: chainService,
		BlobStorage:        filesystem.NewEphemeralBlobStorage(t),
		DataColumnStorage:  dataColumnStorage,
	}

	// Compute versioned hashes for commitments in their block order
	hash0 := primitives.ConvertKzgCommitmentToVersionedHash(commitments[0])
	hash1 := primitives.ConvertKzgCommitmentToVersionedHash(commitments[1])
	hash2 := primitives.ConvertKzgCommitmentToVersionedHash(commitments[2])

	t.Run("blobs returned in commitment order regardless of request order", func(t *testing.T) {
		// Request versioned hashes in reverse order: 2, 1, 0
		requestedHashes := [][]byte{hash2[:], hash1[:], hash0[:]}

		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "finalized", options.WithVersionedHashes(requestedHashes))
		if rpcErr != nil {
			t.Errorf("RPC Error: %v (reason: %v)", rpcErr.Err, rpcErr.Reason)
			return
		}
		require.Equal(t, 3, len(verifiedBlobs))

		// Verify blobs are returned in commitment order from the block (0, 1, 2)
		// In Fulu, blobs are reconstructed from data columns
		assert.Equal(t, uint64(0), verifiedBlobs[0].Index) // First commitment in block
		assert.Equal(t, uint64(1), verifiedBlobs[1].Index) // Second commitment in block
		assert.Equal(t, uint64(2), verifiedBlobs[2].Index) // Third commitment in block

		// Verify the blob content matches what we expect
		for i, verifiedBlob := range verifiedBlobs {
			require.NotNil(t, verifiedBlob.BlobSidecar)
			require.DeepEqual(t, fuluBlobs[i].Blob, verifiedBlob.Blob)
			require.DeepEqual(t, fuluBlobs[i].KzgCommitment, verifiedBlob.KzgCommitment)
		}
	})

	t.Run("subset of blobs maintains commitment order", func(t *testing.T) {
		// Request hashes for indices 1 and 0 (out of order)
		requestedHashes := [][]byte{hash1[:], hash0[:]}

		verifiedBlobs, rpcErr := blocker.Blobs(ctx, "finalized", options.WithVersionedHashes(requestedHashes))
		if rpcErr != nil {
			t.Errorf("RPC Error: %v (reason: %v)", rpcErr.Err, rpcErr.Reason)
			return
		}
		require.Equal(t, 2, len(verifiedBlobs))

		// Verify blobs are returned in commitment order from the block
		assert.Equal(t, uint64(0), verifiedBlobs[0].Index) // First commitment in block
		assert.Equal(t, uint64(1), verifiedBlobs[1].Index) // Second commitment in block

		// Verify the blob content matches what we expect
		require.DeepEqual(t, fuluBlobs[0].Blob, verifiedBlobs[0].Blob)
		require.DeepEqual(t, fuluBlobs[1].Blob, verifiedBlobs[1].Blob)
	})

	t.Run("request non-existent hash", func(t *testing.T) {
		// Create a fake versioned hash
		fakeHash := make([]byte, 32)
		for i := 0; i < 32; i++ {
			fakeHash[i] = 0xFF
		}

		// Request only the fake hash
		requestedHashes := [][]byte{fakeHash}

		_, rpcErr := blocker.Blobs(ctx, "finalized", options.WithVersionedHashes(requestedHashes))
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.NotFound), rpcErr.Reason)
		require.StringContains(t, "versioned hash(es) not found in block", rpcErr.Err.Error())
		require.StringContains(t, "requested 1 hashes, found 0", rpcErr.Err.Error())
		require.StringContains(t, "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", rpcErr.Err.Error())
	})

	t.Run("request multiple non-existent hashes", func(t *testing.T) {
		// Create two fake versioned hashes
		fakeHash1 := make([]byte, 32)
		fakeHash2 := make([]byte, 32)
		for i := 0; i < 32; i++ {
			fakeHash1[i] = 0xAA
			fakeHash2[i] = 0xBB
		}

		// Request valid hash with two fake hashes
		requestedHashes := [][]byte{fakeHash1, hash0[:], fakeHash2}

		_, rpcErr := blocker.Blobs(ctx, "finalized", options.WithVersionedHashes(requestedHashes))
		require.NotNil(t, rpcErr)
		require.Equal(t, core.ErrorReason(core.NotFound), rpcErr.Reason)
		require.StringContains(t, "versioned hash(es) not found in block", rpcErr.Err.Error())
		require.StringContains(t, "requested 3 hashes, found 1", rpcErr.Err.Error())
		// Check that both missing hashes are reported
		require.StringContains(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", rpcErr.Err.Error())
		require.StringContains(t, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", rpcErr.Err.Error())
	})
}
