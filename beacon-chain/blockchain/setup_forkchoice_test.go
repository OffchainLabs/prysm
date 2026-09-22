package blockchain

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func Test_startupHeadRoot(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx
	hook := logTest.NewGlobal()
	cp := service.FinalizedCheckpt()
	require.DeepEqual(t, cp.Root, params.BeaconConfig().ZeroHash[:])
	gr := [32]byte{'r', 'o', 'o', 't'}
	service.originBlockRoot = gr
	require.NoError(t, service.cfg.BeaconDB.SaveGenesisBlockRoot(ctx, gr))
	t.Run("start from finalized", func(t *testing.T) {
		require.Equal(t, service.startupHeadRoot(), gr)
	})
	t.Run("head requested, error path", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{
			ForceHead: "head",
		})
		defer resetCfg()
		require.Equal(t, service.startupHeadRoot(), gr)
		require.LogsContain(t, hook, "Could not get head block root, starting with justified block as head")
	})

	st, _ := util.DeterministicGenesisState(t, 64)
	hr := [32]byte{'h', 'e', 'a', 'd'}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st, hr), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, hr), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, hr))

	t.Run("start from head", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{
			ForceHead: "head",
		})
		defer resetCfg()
		require.Equal(t, service.startupHeadRoot(), hr)
	})
}

func Test_setupForkchoiceTree_Finalized(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	st, _ := util.DeterministicGenesisState(t, 64)
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err, "Could not hash genesis state")

	require.NoError(t, service.saveGenesisData(ctx, st))

	genesis := blocks.NewGenesisBlock(stateRoot[:])
	wsb, err := consensusblocks.NewSignedBeaconBlock(genesis)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb), "Could not save genesis block")
	parentRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st, parentRoot), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, parentRoot), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx, &ethpb.Checkpoint{Root: parentRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Root: parentRoot[:]}))
	require.NoError(t, service.setupForkchoiceTree(st))
	require.Equal(t, 1, service.cfg.ForkChoiceStore.NodeCount())
}

func Test_setupForkchoiceTree_Head(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx
	resetCfg := features.InitWithReset(&features.Flags{
		ForceHead: "head",
	})
	defer resetCfg()

	genesisState, keys := util.DeterministicGenesisState(t, 64)
	stateRoot, err := genesisState.HashTreeRoot(ctx)
	require.NoError(t, err, "Could not hash genesis state")
	genesis := blocks.NewGenesisBlock(stateRoot[:])
	wsb, err := consensusblocks.NewSignedBeaconBlock(genesis)
	require.NoError(t, err)
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb), "Could not save genesis block")
	require.NoError(t, service.saveGenesisData(ctx, genesisState))

	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, genesisState, genesisRoot), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, genesisRoot), "Could not save genesis state")

	st, err := service.HeadState(ctx)
	require.NoError(t, err)
	b, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(1))
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	preState, err := service.GetBlockPreState(ctx, roblock)
	require.NoError(t, err)
	postState, err := service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))

	b, err = util.GenerateFullBlock(postState, keys, util.DefaultBlockGenConfig(), primitives.Slot(2))
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err = b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, root, wsb, preState))

	require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, root))
	cp := service.FinalizedCheckpt()
	fRoot := service.ensureRootNotZeros([32]byte(cp.Root))
	require.NotEqual(t, fRoot, root)
	require.Equal(t, root, service.startupHeadRoot())
	require.NoError(t, service.setupForkchoiceTree(st))
	require.Equal(t, 3, service.cfg.ForkChoiceStore.NodeCount())
}

func gloasChainBlock(t *testing.T, slot primitives.Slot, parentRoot [32]byte, parentBlockHash, blockHash []byte) *forkchoicetypes.BlockAndCheckpoints {
	t.Helper()
	bid := util.HydrateSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BlockHash:       blockHash,
			ParentBlockHash: parentBlockHash,
		},
	})
	blk := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body:       &ethpb.BeaconBlockBodyGloas{SignedExecutionPayloadBid: bid},
		},
	})
	wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	root, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	return &forkchoicetypes.BlockAndCheckpoints{Block: roblock}
}

func gloasEnvelope(blockRoot [32]byte, blockHash []byte) *ethpb.SignedExecutionPayloadEnvelope {
	return &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    make([]byte, 32),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     blockHash,
			},
			ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
			BeaconBlockRoot:       blockRoot[:],
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}
}

func Test_resolveChainPayloadStatus(t *testing.T) {
	hashA := bytesutil.PadTo([]byte("a"), 32)
	hashB := bytesutil.PadTo([]byte("b"), 32)

	newGloasChain := func(t *testing.T) []*forkchoicetypes.BlockAndCheckpoints {
		first := gloasChainBlock(t, 1, [32]byte{}, make([]byte, 32), hashA)
		second := gloasChainBlock(t, 2, first.Block.Root(), hashA, hashB)
		return []*forkchoicetypes.BlockAndCheckpoints{first, second}
	}

	t.Run("last block payload in db", func(t *testing.T) {
		service, tr := minimalTestService(t)
		ctx := tr.ctx
		chain := newGloasChain(t)
		require.NoError(t, service.cfg.BeaconDB.SaveExecutionPayloadEnvelope(ctx, gloasEnvelope(chain[1].Block.Root(), hashB)))
		service.resolveChainPayloadStatus(ctx, chain)
		require.Equal(t, true, chain[0].HasPayload)
		require.Equal(t, true, chain[1].HasPayload)
	})

	t.Run("last block payload not in db", func(t *testing.T) {
		service, tr := minimalTestService(t)
		chain := newGloasChain(t)
		service.resolveChainPayloadStatus(tr.ctx, chain)
		require.Equal(t, true, chain[0].HasPayload)
		require.Equal(t, false, chain[1].HasPayload)
	})

	t.Run("pre-Gloas last block", func(t *testing.T) {
		service, tr := minimalTestService(t)
		blk := util.HydrateSignedBeaconBlockDeneb(&ethpb.SignedBeaconBlockDeneb{})
		wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		chain := []*forkchoicetypes.BlockAndCheckpoints{{Block: roblock}}
		service.resolveChainPayloadStatus(tr.ctx, chain)
		require.Equal(t, false, chain[0].HasPayload)
	})

	t.Run("empty chain", func(t *testing.T) {
		service, tr := minimalTestService(t)
		service.resolveChainPayloadStatus(tr.ctx, nil)
	})
}

// Regression test: the justified checkpoint in the DB references a root whose
// block is absent (BeaconDB.Block returns nil, nil for it). Startup must fall
// back to the finalized block as head instead of panicking on the nil block.
func Test_setupForkchoiceTree_MissingHeadBlock(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx
	hook := logTest.NewGlobal()

	st, _ := util.DeterministicGenesisState(t, 64)
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err, "Could not hash genesis state")
	require.NoError(t, service.saveGenesisData(ctx, st))

	genesis := blocks.NewGenesisBlock(stateRoot[:])
	wsb, err := consensusblocks.NewSignedBeaconBlock(genesis)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb), "Could not save genesis block")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st, genesisRoot), "Could not save genesis state")

	// The justified checkpoint points to a root whose state exists but whose
	// block is not in the DB, e.g. left behind by an unclean stop.
	missingRoot := [32]byte{'m', 'i', 's', 's', 'i', 'n', 'g'}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st, missingRoot), "Could not save state")
	require.NoError(t, service.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: 1, Root: missingRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Root: genesisRoot[:]}))
	require.NoError(t, service.setupForkchoiceCheckpoints())

	blk, err := service.cfg.BeaconDB.Block(ctx, missingRoot)
	require.NoError(t, err)
	require.IsNil(t, blk)
	require.Equal(t, missingRoot, service.startupHeadRoot())

	require.NoError(t, service.setupForkchoiceTree(st))
	require.LogsContain(t, hook, "starting with finalized block as head")
	require.Equal(t, 1, service.cfg.ForkChoiceStore.NodeCount())
}
