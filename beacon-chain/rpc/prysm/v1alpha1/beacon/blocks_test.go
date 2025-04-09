package beacon

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	chainMock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	dbTest "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/core"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"google.golang.org/protobuf/proto"
)

// ensures that if any of the checkpoints are zero-valued, an error will be generated without genesis being present
func TestServer_GetChainHead_NoGenesis(t *testing.T) {
	db := dbTest.SetupDB(t)

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.SetSlot(1))

	genBlock := util.NewBeaconBlock()
	genBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'G'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, genBlock)
	gRoot, err := genBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	cases := []struct {
		name       string
		zeroSetter func(val *ethpb.Checkpoint) error
	}{
		{
			name:       "zero-value prev justified",
			zeroSetter: s.SetPreviousJustifiedCheckpoint,
		},
		{
			name:       "zero-value current justified",
			zeroSetter: s.SetCurrentJustifiedCheckpoint,
		},
		{
			name:       "zero-value finalized",
			zeroSetter: s.SetFinalizedCheckpoint,
		},
	}
	finalized := &ethpb.Checkpoint{Epoch: 1, Root: gRoot[:]}
	prevJustified := &ethpb.Checkpoint{Epoch: 2, Root: gRoot[:]}
	justified := &ethpb.Checkpoint{Epoch: 3, Root: gRoot[:]}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.NoError(t, s.SetPreviousJustifiedCheckpoint(prevJustified))
			require.NoError(t, s.SetCurrentJustifiedCheckpoint(justified))
			require.NoError(t, s.SetFinalizedCheckpoint(finalized))
			require.NoError(t, c.zeroSetter(&ethpb.Checkpoint{Epoch: 0, Root: params.BeaconConfig().ZeroHash[:]}))
		})
		wsb, err := blocks.NewSignedBeaconBlock(genBlock)
		require.NoError(t, err)
		bs := &Server{
			CoreService: &core.Service{
				BeaconDB:    db,
				HeadFetcher: &chainMock.ChainService{Block: wsb, State: s},
				FinalizedFetcher: &chainMock.ChainService{
					FinalizedCheckPoint:         s.FinalizedCheckpoint(),
					CurrentJustifiedCheckPoint:  s.CurrentJustifiedCheckpoint(),
					PreviousJustifiedCheckPoint: s.PreviousJustifiedCheckpoint(),
				},
				OptimisticModeFetcher: &chainMock.ChainService{},
			},
		}
		_, err = bs.GetChainHead(context.Background(), nil)
		require.ErrorContains(t, "could not get genesis block", err)
	}
}

func TestServer_GetChainHead_NoFinalizedBlock(t *testing.T) {
	db := dbTest.SetupDB(t)

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.SetSlot(1))
	require.NoError(t, s.SetPreviousJustifiedCheckpoint(&ethpb.Checkpoint{Epoch: 3, Root: bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)}))
	require.NoError(t, s.SetCurrentJustifiedCheckpoint(&ethpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte{'B'}, fieldparams.RootLength)}))
	require.NoError(t, s.SetFinalizedCheckpoint(&ethpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'C'}, fieldparams.RootLength)}))

	genBlock := util.NewBeaconBlock()
	genBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'G'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, genBlock)
	gRoot, err := genBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), gRoot))

	wsb, err := blocks.NewSignedBeaconBlock(genBlock)
	require.NoError(t, err)

	bs := &Server{
		CoreService: &core.Service{
			BeaconDB:    db,
			HeadFetcher: &chainMock.ChainService{Block: wsb, State: s},
			FinalizedFetcher: &chainMock.ChainService{
				FinalizedCheckPoint:         s.FinalizedCheckpoint(),
				CurrentJustifiedCheckPoint:  s.CurrentJustifiedCheckpoint(),
				PreviousJustifiedCheckPoint: s.PreviousJustifiedCheckpoint()},
			OptimisticModeFetcher: &chainMock.ChainService{},
		},
	}

	_, err = bs.GetChainHead(context.Background(), nil)
	require.ErrorContains(t, "could not get finalized block", err)
}

func TestServer_GetChainHead_NoHeadBlock(t *testing.T) {
	bs := &Server{
		CoreService: &core.Service{
			HeadFetcher:           &chainMock.ChainService{Block: nil},
			OptimisticModeFetcher: &chainMock.ChainService{},
		},
	}
	_, err := bs.GetChainHead(context.Background(), nil)
	assert.ErrorContains(t, "head block of chain was nil", err)
}

func TestServer_GetChainHead(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())

	db := dbTest.SetupDB(t)
	genBlock := util.NewBeaconBlock()
	genBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'G'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, genBlock)
	gRoot, err := genBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), gRoot))

	finalizedBlock := util.NewBeaconBlock()
	finalizedBlock.Block.Slot = 1
	finalizedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, finalizedBlock)
	fRoot, err := finalizedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	justifiedBlock := util.NewBeaconBlock()
	justifiedBlock.Block.Slot = 2
	justifiedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'B'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, justifiedBlock)
	jRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	prevJustifiedBlock := util.NewBeaconBlock()
	prevJustifiedBlock.Block.Slot = 3
	prevJustifiedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'C'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, prevJustifiedBlock)
	pjRoot, err := prevJustifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	s, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot:                        1,
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{Epoch: 3, Root: pjRoot[:]},
		CurrentJustifiedCheckpoint:  &ethpb.Checkpoint{Epoch: 2, Root: jRoot[:]},
		FinalizedCheckpoint:         &ethpb.Checkpoint{Epoch: 1, Root: fRoot[:]},
	})
	require.NoError(t, err)

	b := util.NewBeaconBlock()
	b.Block.Slot, err = slots.EpochStart(s.PreviousJustifiedCheckpoint().Epoch)
	require.NoError(t, err)
	b.Block.Slot++
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	bs := &Server{
		CoreService: &core.Service{
			BeaconDB:              db,
			HeadFetcher:           &chainMock.ChainService{Block: wsb, State: s},
			OptimisticModeFetcher: &chainMock.ChainService{},
			FinalizedFetcher: &chainMock.ChainService{
				FinalizedCheckPoint:         s.FinalizedCheckpoint(),
				CurrentJustifiedCheckPoint:  s.CurrentJustifiedCheckpoint(),
				PreviousJustifiedCheckPoint: s.PreviousJustifiedCheckpoint()},
		},
	}

	head, err := bs.GetChainHead(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, primitives.Epoch(3), head.PreviousJustifiedEpoch, "Unexpected PreviousJustifiedEpoch")
	assert.Equal(t, primitives.Epoch(2), head.JustifiedEpoch, "Unexpected JustifiedEpoch")
	assert.Equal(t, primitives.Epoch(1), head.FinalizedEpoch, "Unexpected FinalizedEpoch")
	assert.Equal(t, primitives.Slot(24), head.PreviousJustifiedSlot, "Unexpected PreviousJustifiedSlot")
	assert.Equal(t, primitives.Slot(16), head.JustifiedSlot, "Unexpected JustifiedSlot")
	assert.Equal(t, primitives.Slot(8), head.FinalizedSlot, "Unexpected FinalizedSlot")
	assert.DeepEqual(t, pjRoot[:], head.PreviousJustifiedBlockRoot, "Unexpected PreviousJustifiedBlockRoot")
	assert.DeepEqual(t, jRoot[:], head.JustifiedBlockRoot, "Unexpected JustifiedBlockRoot")
	assert.DeepEqual(t, fRoot[:], head.FinalizedBlockRoot, "Unexpected FinalizedBlockRoot")
	assert.Equal(t, false, head.OptimisticStatus)
}

func TestServer_ListBeaconBlocks_NoResults(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	bs := &Server{
		BeaconDB: db,
	}
	wanted := &ethpb.ListBeaconBlocksResponse{
		BlockContainers: make([]*ethpb.BeaconBlockContainer, 0),
		TotalSize:       int32(0),
		NextPageToken:   strconv.Itoa(0),
	}
	res, err := bs.ListBeaconBlocks(ctx, &ethpb.ListBlocksRequest{
		QueryFilter: &ethpb.ListBlocksRequest_Slot{
			Slot: 0,
		},
	})
	require.NoError(t, err)
	if !proto.Equal(wanted, res) {
		t.Errorf("Wanted %v, received %v", wanted, res)
	}
	res, err = bs.ListBeaconBlocks(ctx, &ethpb.ListBlocksRequest{
		QueryFilter: &ethpb.ListBlocksRequest_Slot{
			Slot: 0,
		},
	})
	require.NoError(t, err)
	if !proto.Equal(wanted, res) {
		t.Errorf("Wanted %v, received %v", wanted, res)
	}
	res, err = bs.ListBeaconBlocks(ctx, &ethpb.ListBlocksRequest{
		QueryFilter: &ethpb.ListBlocksRequest_Root{
			Root: make([]byte, 32),
		},
	})
	require.NoError(t, err)
	if !proto.Equal(wanted, res) {
		t.Errorf("Wanted %v, received %v", wanted, res)
	}
}

func TestServer_ListBeaconBlocks_Genesis(t *testing.T) {
	t.Run("phase 0 block", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = parentRoot[:]
		blkContainer := &ethpb.BeaconBlockContainer{
			Block: &ethpb.BeaconBlockContainer_Phase0Block{Phase0Block: blk}}
		wrappedB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBlocksGenesis(t, wrappedB, blkContainer)
	})
	t.Run("altair block", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockAltair()
		blk.Block.ParentRoot = parentRoot[:]
		wrapped, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		blkContainer := &ethpb.BeaconBlockContainer{
			Block: &ethpb.BeaconBlockContainer_AltairBlock{AltairBlock: blk}}
		runListBlocksGenesis(t, wrapped, blkContainer)
	})
	t.Run("bellatrix block", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockBellatrix()
		blk.Block.ParentRoot = parentRoot[:]
		wrapped, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		blinded, err := wrapped.ToBlinded()
		assert.NoError(t, err)
		pb, err := blinded.Proto()
		assert.NoError(t, err)
		blindedProto, ok := pb.(*ethpb.SignedBlindedBeaconBlockBellatrix)
		require.Equal(t, true, ok)
		blkContainer := &ethpb.BeaconBlockContainer{
			Block: &ethpb.BeaconBlockContainer_BlindedBellatrixBlock{BlindedBellatrixBlock: blindedProto}}
		runListBlocksGenesis(t, wrapped, blkContainer)
	})
	t.Run("capella block", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		wrapped, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		blinded, err := wrapped.ToBlinded()
		assert.NoError(t, err)
		pb, err := blinded.Proto()
		assert.NoError(t, err)
		blindedProto, ok := pb.(*ethpb.SignedBlindedBeaconBlockCapella)
		require.Equal(t, true, ok)
		assert.NoError(t, err)
		blkContainer := &ethpb.BeaconBlockContainer{
			Block: &ethpb.BeaconBlockContainer_BlindedCapellaBlock{BlindedCapellaBlock: blindedProto}}
		runListBlocksGenesis(t, wrapped, blkContainer)
	})
}

func runListBlocksGenesis(t *testing.T, blk interfaces.ReadOnlySignedBeaconBlock, blkContainer *ethpb.BeaconBlockContainer) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	bs := &Server{
		BeaconDB: db,
	}

	// Should throw an error if no genesis block is found.
	_, err := bs.ListBeaconBlocks(ctx, &ethpb.ListBlocksRequest{
		QueryFilter: &ethpb.ListBlocksRequest_Genesis{
			Genesis: true,
		},
	})
	require.ErrorContains(t, "Could not find genesis", err)

	root, err := blk.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, blk))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))
	blkContainer.BlockRoot = root[:]
	blkContainer.Canonical = true

	wanted := &ethpb.ListBeaconBlocksResponse{
		BlockContainers: []*ethpb.BeaconBlockContainer{blkContainer},
		NextPageToken:   "0",
		TotalSize:       1,
	}
	res, err := bs.ListBeaconBlocks(ctx, &ethpb.ListBlocksRequest{
		QueryFilter: &ethpb.ListBlocksRequest_Genesis{
			Genesis: true,
		},
	})
	require.NoError(t, err)
	if !proto.Equal(wanted, res) {
		t.Errorf("Wanted %v, received %v", wanted, res)
	}
}

func TestServer_ListBeaconBlocks_Genesis_MultiBlocks(t *testing.T) {
	t.Run("phase 0 block", func(t *testing.T) {
		parentRoot := [32]byte{1, 2, 3}
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = parentRoot[:]
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlock()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		genB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksGenesisMultiBlocks(t, genB, blockCreator)
	})
	t.Run("altair block", func(t *testing.T) {
		parentRoot := [32]byte{1, 2, 3}
		blk := util.NewBeaconBlockAltair()
		blk.Block.ParentRoot = parentRoot[:]
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockAltair()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		gBlock, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksGenesisMultiBlocks(t, gBlock, blockCreator)
	})
	t.Run("bellatrix block", func(t *testing.T) {
		parentRoot := [32]byte{1, 2, 3}
		blk := util.NewBeaconBlockBellatrix()
		blk.Block.ParentRoot = parentRoot[:]
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockBellatrix()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		gBlock, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksGenesisMultiBlocks(t, gBlock, blockCreator)
	})
	t.Run("capella block", func(t *testing.T) {
		parentRoot := [32]byte{1, 2, 3}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockCapella()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		gBlock, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksGenesisMultiBlocks(t, gBlock, blockCreator)
	})
}

func runListBeaconBlocksGenesisMultiBlocks(t *testing.T, genBlock interfaces.ReadOnlySignedBeaconBlock,
	blockCreator func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	bs := &Server{
		BeaconDB: db,
	}
	// Should return the proper genesis block if it exists.
	root, err := genBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, genBlock))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

	count := primitives.Slot(100)
	blks := make([]interfaces.ReadOnlySignedBeaconBlock, count)
	for i := primitives.Slot(0); i < count; i++ {
		blks[i] = blockCreator(i)
	}
	require.NoError(t, db.SaveBlocks(ctx, blks))

	// Should throw an error if more than one blk returned.
	_, err = bs.ListBeaconBlocks(ctx, &ethpb.ListBlocksRequest{
		QueryFilter: &ethpb.ListBlocksRequest_Genesis{
			Genesis: true,
		},
	})
	require.NoError(t, err)
}

func TestServer_ListBeaconBlocks_Pagination(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())
	t.Run("phase 0 block", func(t *testing.T) {
		blk := util.NewBeaconBlock()
		blk.Block.Slot = 300
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlock()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		containerCreator := func(i primitives.Slot, root []byte, canonical bool) *ethpb.BeaconBlockContainer {
			b := util.NewBeaconBlock()
			b.Block.Slot = i
			ctr := &ethpb.BeaconBlockContainer{
				Block: &ethpb.BeaconBlockContainer_Phase0Block{
					Phase0Block: util.HydrateSignedBeaconBlock(b)},
				BlockRoot: root,
				Canonical: canonical}
			return ctr
		}
		wrappedB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksPagination(t, wrappedB, blockCreator, containerCreator)
	})
	t.Run("altair block", func(t *testing.T) {
		blk := util.NewBeaconBlockAltair()
		blk.Block.Slot = 300
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockAltair()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		containerCreator := func(i primitives.Slot, root []byte, canonical bool) *ethpb.BeaconBlockContainer {
			b := util.NewBeaconBlockAltair()
			b.Block.Slot = i
			ctr := &ethpb.BeaconBlockContainer{
				Block: &ethpb.BeaconBlockContainer_AltairBlock{
					AltairBlock: util.HydrateSignedBeaconBlockAltair(b)},
				BlockRoot: root,
				Canonical: canonical}
			return ctr
		}
		orphanedB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksPagination(t, orphanedB, blockCreator, containerCreator)
	})
	t.Run("bellatrix block", func(t *testing.T) {
		resetFn := features.InitWithReset(&features.Flags{
			SaveFullExecutionPayloads: true,
		})
		defer resetFn()
		blk := util.NewBeaconBlockBellatrix()
		blk.Block.Slot = 300
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockBellatrix()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		containerCreator := func(i primitives.Slot, root []byte, canonical bool) *ethpb.BeaconBlockContainer {
			b := util.NewBeaconBlockBellatrix()
			b.Block.Slot = i
			ctr := &ethpb.BeaconBlockContainer{
				Block: &ethpb.BeaconBlockContainer_BellatrixBlock{
					BellatrixBlock: util.HydrateSignedBeaconBlockBellatrix(b)},
				BlockRoot: root,
				Canonical: canonical}
			return ctr
		}
		orphanedB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksPagination(t, orphanedB, blockCreator, containerCreator)
	})
	t.Run("capella block", func(t *testing.T) {
		resetFn := features.InitWithReset(&features.Flags{
			SaveFullExecutionPayloads: true,
		})
		defer resetFn()
		blk := util.NewBeaconBlockCapella()
		blk.Block.Slot = 300
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockCapella()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		containerCreator := func(i primitives.Slot, root []byte, canonical bool) *ethpb.BeaconBlockContainer {
			b := util.NewBeaconBlockCapella()
			b.Block.Slot = i
			ctr := &ethpb.BeaconBlockContainer{
				Block: &ethpb.BeaconBlockContainer_CapellaBlock{
					CapellaBlock: util.HydrateSignedBeaconBlockCapella(b)},
				BlockRoot: root,
				Canonical: canonical}
			return ctr
		}
		orphanedB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksPagination(t, orphanedB, blockCreator, containerCreator)
	})
}

func runListBeaconBlocksPagination(t *testing.T, orphanedBlk interfaces.ReadOnlySignedBeaconBlock,
	blockCreator func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock, containerCreator func(i primitives.Slot, root []byte, canonical bool) *ethpb.BeaconBlockContainer) {

	db := dbTest.SetupDB(t)
	chain := &chainMock.ChainService{
		CanonicalRoots: map[[32]byte]bool{},
	}
	ctx := context.Background()

	count := primitives.Slot(100)
	blks := make([]interfaces.ReadOnlySignedBeaconBlock, count)
	blkContainers := make([]*ethpb.BeaconBlockContainer, count)
	for i := primitives.Slot(0); i < count; i++ {
		b := blockCreator(i)
		root, err := b.Block().HashTreeRoot()
		require.NoError(t, err)
		chain.CanonicalRoots[root] = true
		blks[i] = b
		ctr, err := convertToBlockContainer(blks[i], root, true)
		require.NoError(t, err)
		blkContainers[i] = ctr
	}
	require.NoError(t, db.SaveBlocks(ctx, blks))

	orphanedBlkRoot, err := orphanedBlk.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, orphanedBlk))

	bs := &Server{
		BeaconDB:         db,
		CanonicalFetcher: chain,
	}

	root6, err := blks[6].Block().HashTreeRoot()
	require.NoError(t, err)

	tests := []struct {
		req *ethpb.ListBlocksRequest
		res *ethpb.ListBeaconBlocksResponse
	}{
		{req: &ethpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(0),
			QueryFilter: &ethpb.ListBlocksRequest_Slot{Slot: 5},
			PageSize:    3},
			res: &ethpb.ListBeaconBlocksResponse{
				BlockContainers: []*ethpb.BeaconBlockContainer{containerCreator(5, blkContainers[5].BlockRoot, blkContainers[5].Canonical)},
				NextPageToken:   "",
				TotalSize:       1,
			},
		},
		{req: &ethpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(0),
			QueryFilter: &ethpb.ListBlocksRequest_Root{Root: root6[:]},
			PageSize:    3},
			res: &ethpb.ListBeaconBlocksResponse{
				BlockContainers: []*ethpb.BeaconBlockContainer{containerCreator(6, blkContainers[6].BlockRoot, blkContainers[6].Canonical)},
				TotalSize:       1,
				NextPageToken:   strconv.Itoa(0)}},
		{req: &ethpb.ListBlocksRequest{QueryFilter: &ethpb.ListBlocksRequest_Root{Root: root6[:]}},
			res: &ethpb.ListBeaconBlocksResponse{
				BlockContainers: []*ethpb.BeaconBlockContainer{containerCreator(6, blkContainers[6].BlockRoot, blkContainers[6].Canonical)},
				TotalSize:       1, NextPageToken: strconv.Itoa(0)}},
		{req: &ethpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(0),
			QueryFilter: &ethpb.ListBlocksRequest_Epoch{Epoch: 0},
			PageSize:    100},
			res: &ethpb.ListBeaconBlocksResponse{
				BlockContainers: blkContainers[0:params.BeaconConfig().SlotsPerEpoch],
				NextPageToken:   "",
				TotalSize:       int32(params.BeaconConfig().SlotsPerEpoch)}},
		{req: &ethpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(1),
			QueryFilter: &ethpb.ListBlocksRequest_Epoch{Epoch: 5},
			PageSize:    3},
			res: &ethpb.ListBeaconBlocksResponse{
				BlockContainers: blkContainers[43:46],
				NextPageToken:   "2",
				TotalSize:       int32(params.BeaconConfig().SlotsPerEpoch)}},
		{req: &ethpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(1),
			QueryFilter: &ethpb.ListBlocksRequest_Epoch{Epoch: 11},
			PageSize:    7},
			res: &ethpb.ListBeaconBlocksResponse{
				BlockContainers: blkContainers[95:96],
				NextPageToken:   "",
				TotalSize:       int32(params.BeaconConfig().SlotsPerEpoch)}},
		{req: &ethpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(0),
			QueryFilter: &ethpb.ListBlocksRequest_Epoch{Epoch: 12},
			PageSize:    4},
			res: &ethpb.ListBeaconBlocksResponse{
				BlockContainers: blkContainers[96:100],
				NextPageToken:   "",
				TotalSize:       int32(params.BeaconConfig().SlotsPerEpoch / 2)}},
		{req: &ethpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(0),
			QueryFilter: &ethpb.ListBlocksRequest_Slot{Slot: 300},
			PageSize:    3},
			res: &ethpb.ListBeaconBlocksResponse{
				BlockContainers: []*ethpb.BeaconBlockContainer{containerCreator(300, orphanedBlkRoot[:], false)},
				NextPageToken:   "",
				TotalSize:       1}},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("test_%d", i), func(t *testing.T) {
			res, err := bs.ListBeaconBlocks(ctx, test.req)
			require.NoError(t, err)
			require.DeepSSZEqual(t, res, test.res)
		})
	}
}

func TestServer_ConvertToBlockContainer(t *testing.T) {
	b := util.NewBeaconBlockCapella()
	root, err := b.HashTreeRoot()
	require.NoError(t, err)
	wrapped, err := blocks.NewSignedBeaconBlock(b)
	assert.NoError(t, err)
	container, err := convertToBlockContainer(wrapped, root, true)
	require.NoError(t, err)
	require.NotNil(t, container.GetCapellaBlock())

	bb := util.NewBlindedBeaconBlockCapella()
	root, err = b.HashTreeRoot()
	require.NoError(t, err)
	wrapped, err = blocks.NewSignedBeaconBlock(bb)
	assert.NoError(t, err)
	container, err = convertToBlockContainer(wrapped, root, true)
	require.NoError(t, err)
	require.NotNil(t, container.GetBlindedCapellaBlock())
}
