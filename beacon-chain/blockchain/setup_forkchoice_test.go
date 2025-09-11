package blockchain

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
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
	preState, err := service.getBlockPreState(ctx, wsb.Block())
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
	require.Equal(t, 2, service.cfg.ForkChoiceStore.NodeCount())
}

// Test_setupForkchoiceTree_JustifiedBlockInsertion tests the regression where
// the justified checkpoint block was not being inserted into forkchoice during startup.
func Test_setupForkchoiceTree_JustifiedBlockInsertion(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	genesisState, _ := util.DeterministicGenesisState(t, 64)
	stateRoot, err := genesisState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(stateRoot[:])
	wsb, err := consensusblocks.NewSignedBeaconBlock(genesis)
	require.NoError(t, err)
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb))
	require.NoError(t, service.saveGenesisData(ctx, genesisState))

	// Create a justified block that is different from finalized (genesis)
	justifiedBlock := &ethpb.SignedBeaconBlock{
		Block: &ethpb.BeaconBlock{
			Slot:       1,
			ParentRoot: genesisRoot[:],
			StateRoot:  stateRoot[:],
			Body: &ethpb.BeaconBlockBody{
				RandaoReveal: make([]byte, 96),
				Eth1Data: &ethpb.Eth1Data{
					DepositRoot:  make([]byte, 32),
					DepositCount: 0,
					BlockHash:    make([]byte, 32),
				},
				Graffiti:          make([]byte, 32),
				ProposerSlashings: []*ethpb.ProposerSlashing{},
				AttesterSlashings: []*ethpb.AttesterSlashing{},
				Attestations:      []*ethpb.Attestation{},
				Deposits:          []*ethpb.Deposit{},
				VoluntaryExits:    []*ethpb.SignedVoluntaryExit{},
			},
		},
		Signature: make([]byte, 96),
	}
	wsbJustified, err := consensusblocks.NewSignedBeaconBlock(justifiedBlock)
	require.NoError(t, err)
	justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsbJustified))
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, genesisState, justifiedRoot))

	// Set justified checkpoint different from finalized
	require.NoError(t, service.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx, &ethpb.Checkpoint{
		Epoch: 0,
		Root:  justifiedRoot[:],
	}))
	require.NoError(t, service.cfg.BeaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{
		Epoch: 0,
		Root:  genesisRoot[:],
	}))

	// This should load the justified checkpoint and insert the justified block
	require.NoError(t, service.setupForkchoice(genesisState))

	// Before the fix: NodeCount would be 1 (only finalized block)
	// After the fix: NodeCount should be 2 (finalized + justified blocks)
	require.Equal(t, 2, service.cfg.ForkChoiceStore.NodeCount())

	// Verify the justified checkpoint block is in forkchoice
	service.cfg.ForkChoiceStore.RLock()
	hasJustified := service.cfg.ForkChoiceStore.HasNode(justifiedRoot)
	highestRoot := service.cfg.ForkChoiceStore.HighestReceivedBlockRoot()
	service.cfg.ForkChoiceStore.RUnlock()
	
	require.Equal(t, true, hasJustified, "Justified checkpoint block should be in forkchoice")
	require.Equal(t, justifiedRoot, highestRoot, "Highest received block should be the justified checkpoint")
}
