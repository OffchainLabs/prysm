package blockchain

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	mockExecution "github.com/OffchainLabs/prysm/v6/beacon-chain/execution/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/types"
	lightClient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations/kv"
	mockp2p "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/genesis"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func Test_pruneAttsFromPool_Electra(t *testing.T) {
	ctx := t.Context()
	logHook := logTest.NewGlobal()

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.TargetCommitteeSize = 8
	params.OverrideBeaconConfig(cfg)

	s := testServiceNoDB(t)
	s.cfg.AttPool = kv.NewAttCaches()

	data := &ethpb.AttestationData{
		BeaconBlockRoot: make([]byte, 32),
		Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
		Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
	}

	cb := primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(0, true)
	att1 := &ethpb.AttestationElectra{
		AggregationBits: bitfield.Bitlist{0b10000000, 0b00000001},
		Data:            data,
		Signature:       make([]byte, 96),
		CommitteeBits:   cb,
	}

	cb = primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(1, true)
	att2 := &ethpb.AttestationElectra{
		AggregationBits: bitfield.Bitlist{0b11110111, 0b00000001},
		Data:            data,
		Signature:       make([]byte, 96),
		CommitteeBits:   cb,
	}

	cb = primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(3, true)
	att3 := &ethpb.AttestationElectra{
		AggregationBits: bitfield.Bitlist{0b11110111, 0b00000001},
		Data:            data,
		Signature:       make([]byte, 96),
		CommitteeBits:   cb,
	}

	require.NoError(t, s.cfg.AttPool.SaveUnaggregatedAttestation(att1))
	require.NoError(t, s.cfg.AttPool.SaveAggregatedAttestation(att2))
	require.NoError(t, s.cfg.AttPool.SaveAggregatedAttestation(att3))

	cb = primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(0, true)
	cb.SetBitAt(1, true)
	onChainAtt := &ethpb.AttestationElectra{
		AggregationBits: bitfield.Bitlist{0b10000000, 0b11110111, 0b00000001},
		Data:            data,
		Signature:       make([]byte, 96),
		CommitteeBits:   cb,
	}
	bl := &ethpb.SignedBeaconBlockElectra{
		Block: &ethpb.BeaconBlockElectra{
			Body: &ethpb.BeaconBlockBodyElectra{
				Attestations: []*ethpb.AttestationElectra{onChainAtt},
			},
		},
		Signature: make([]byte, 96),
	}
	rob, err := consensusblocks.NewSignedBeaconBlock(bl)
	require.NoError(t, err)
	st, _ := util.DeterministicGenesisStateElectra(t, 1024)
	committees, err := helpers.BeaconCommittees(ctx, st, 0)
	require.NoError(t, err)
	// Sanity check to make sure the on-chain att will be decomposed
	// into the correct number of aggregates.
	require.Equal(t, 4, len(committees))

	s.pruneAttsFromPool(ctx, st, rob)
	require.LogsDoNotContain(t, logHook, "Could not prune attestations")

	attsInPool := s.cfg.AttPool.UnaggregatedAttestations()
	assert.Equal(t, 0, len(attsInPool))
	attsInPool = s.cfg.AttPool.AggregatedAttestations()
	require.Equal(t, 1, len(attsInPool))
	assert.DeepEqual(t, att3, attsInPool[0])
}

func TestStore_OnBlockBatch(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	st, keys := util.DeterministicGenesisState(t, 64)
	require.NoError(t, service.saveGenesisData(ctx, st))
	bState := st.Copy()

	var blks []consensusblocks.ROBlock
	for i := 0; i < 97; i++ {
		b, err := util.GenerateFullBlock(bState, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		bState, err = transition.ExecuteStateTransition(ctx, bState, wsb)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, service.saveInitSyncBlock(ctx, root, wsb))
		wsb, err = consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		rwsb, err := consensusblocks.NewROBlock(wsb)
		require.NoError(t, err)
		blks = append(blks, rwsb)
	}
	err := service.onBlockBatch(ctx, blks, &das.MockAvailabilityStore{})
	require.NoError(t, err)
	jcp := service.CurrentJustifiedCheckpt()
	jroot := bytesutil.ToBytes32(jcp.Root)
	require.Equal(t, blks[63].Root(), jroot)
	require.Equal(t, primitives.Epoch(2), service.cfg.ForkChoiceStore.JustifiedCheckpoint().Epoch)
}

func TestStore_OnBlockBatch_NotifyNewPayload(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	st, keys := util.DeterministicGenesisState(t, 64)
	require.NoError(t, service.saveGenesisData(ctx, st))
	bState := st.Copy()

	var blks []consensusblocks.ROBlock
	blkCount := 4
	for i := 0; i <= blkCount; i++ {
		b, err := util.GenerateFullBlock(bState, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		bState, err = transition.ExecuteStateTransition(ctx, bState, wsb)
		require.NoError(t, err)
		rwsb, err := consensusblocks.NewROBlock(wsb)
		require.NoError(t, err)
		require.NoError(t, service.saveInitSyncBlock(ctx, rwsb.Root(), wsb))
		blks = append(blks, rwsb)
	}
	require.NoError(t, service.onBlockBatch(ctx, blks, &das.MockAvailabilityStore{}))
}

func TestCachedPreState_CanGetFromStateSummary(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB := tr.ctx, tr.db

	st, keys := util.DeterministicGenesisState(t, 64)
	require.NoError(t, service.saveGenesisData(ctx, st))
	b, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(1))
	require.NoError(t, err)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveBlock(ctx, wsb))

	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 1, Root: root[:]}))
	require.NoError(t, service.cfg.StateGen.SaveState(ctx, root, st))
	require.NoError(t, service.verifyBlkPreState(ctx, wsb.Block().ParentRoot()))
}

func TestFillForkChoiceMissingBlocks_CanSave(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB := tr.ctx, tr.db

	st, _ := util.DeterministicGenesisState(t, 64)
	require.NoError(t, service.saveGenesisData(ctx, st))

	roots, err := blockTree1(t, beaconDB, service.originBlockRoot[:])
	require.NoError(t, err)
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	blk := util.NewBeaconBlock()
	blk.Block.Slot = 9
	blk.Block.ParentRoot = roots[8]
	wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	// save invalid block at slot 0 because doubly linked tree enforces that
	// the parent of the last block inserted is the tree node.
	fcp := &ethpb.Checkpoint{Epoch: 0, Root: service.originBlockRoot[:]}
	r0 := bytesutil.ToBytes32(roots[0])
	state, blkRoot, err := prepareForkchoiceState(ctx, 0, r0, service.originBlockRoot, [32]byte{}, fcp, fcp)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	fcp2 := &forkchoicetypes.Checkpoint{Epoch: 0, Root: r0}
	require.NoError(t, service.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(fcp2))
	err = service.fillInForkChoiceMissingBlocks(
		t.Context(), wsb, beaconState.FinalizedCheckpoint(), beaconState.CurrentJustifiedCheckpoint())
	require.NoError(t, err)

	// 5 nodes from the block tree 1. B0 - B3 - B4 - B6 - B8
	// plus 1 node for genesis block root.
	assert.Equal(t, 6, service.cfg.ForkChoiceStore.NodeCount(), "Miss match nodes")
	assert.Equal(t, true, service.cfg.ForkChoiceStore.HasNode(service.originBlockRoot), "Didn't save node")
	assert.Equal(t, true, service.cfg.ForkChoiceStore.HasNode(r0), "Didn't save node")
	assert.Equal(t, true, service.cfg.ForkChoiceStore.HasNode(bytesutil.ToBytes32(roots[3])), "Didn't save node")
	assert.Equal(t, true, service.cfg.ForkChoiceStore.HasNode(bytesutil.ToBytes32(roots[4])), "Didn't save node")
	assert.Equal(t, true, service.cfg.ForkChoiceStore.HasNode(bytesutil.ToBytes32(roots[6])), "Didn't save node")
	assert.Equal(t, true, service.cfg.ForkChoiceStore.HasNode(bytesutil.ToBytes32(roots[8])), "Didn't save node")
}

func TestFillForkChoiceMissingBlocks_RootsMatch(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB := tr.ctx, tr.db

	st, _ := util.DeterministicGenesisState(t, 64)
	require.NoError(t, service.saveGenesisData(ctx, st))

	roots, err := blockTree1(t, beaconDB, service.originBlockRoot[:])
	require.NoError(t, err)

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	blk := util.NewBeaconBlock()
	blk.Block.Slot = 9
	blk.Block.ParentRoot = roots[8]

	wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	// save invalid block at slot 0 because doubly linked tree enforces that
	// the parent of the last block inserted is the tree node.
	fcp := &ethpb.Checkpoint{Epoch: 0, Root: service.originBlockRoot[:]}
	r0 := bytesutil.ToBytes32(roots[0])
	state, blkRoot, err := prepareForkchoiceState(ctx, 0, r0, service.originBlockRoot, [32]byte{}, fcp, fcp)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	fcp2 := &forkchoicetypes.Checkpoint{Epoch: 0, Root: r0}
	require.NoError(t, service.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(fcp2))

	err = service.fillInForkChoiceMissingBlocks(
		t.Context(), wsb, beaconState.FinalizedCheckpoint(), beaconState.CurrentJustifiedCheckpoint())
	require.NoError(t, err)

	// 5 nodes from the block tree 1. B0 - B3 - B4 - B6 - B8
	// plus the origin block root
	assert.Equal(t, 6, service.cfg.ForkChoiceStore.NodeCount(), "Miss match nodes")
	// Ensure all roots and their respective blocks exist.
	wantedRoots := [][]byte{roots[0], roots[3], roots[4], roots[6], roots[8]}
	for i, rt := range wantedRoots {
		assert.Equal(t, true, service.cfg.ForkChoiceStore.HasNode(bytesutil.ToBytes32(rt)), fmt.Sprintf("Didn't save node: %d", i))
		assert.Equal(t, true, service.cfg.BeaconDB.HasBlock(t.Context(), bytesutil.ToBytes32(rt)))
	}
}

func TestFillForkChoiceMissingBlocks_FilterFinalized(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB := tr.ctx, tr.db

	var genesisStateRoot [32]byte
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	validGenesisRoot, err := genesis.Block.HashTreeRoot()
	assert.NoError(t, err)
	st, err := util.NewBeaconState()
	require.NoError(t, err)

	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st.Copy(), validGenesisRoot))

	// Define a tree branch, slot 63 <- 64 <- 65
	b63 := util.NewBeaconBlock()
	b63.Block.Slot = 63
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, b63)
	r63, err := b63.Block.HashTreeRoot()
	require.NoError(t, err)
	b64 := util.NewBeaconBlock()
	b64.Block.Slot = 64
	b64.Block.ParentRoot = r63[:]
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, b64)
	r64, err := b64.Block.HashTreeRoot()
	require.NoError(t, err)
	b65 := util.NewBeaconBlock()
	b65.Block.Slot = 65
	b65.Block.ParentRoot = r64[:]
	r65, err := b65.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, b65)
	b66 := util.NewBeaconBlock()
	b66.Block.Slot = 66
	b66.Block.ParentRoot = r65[:]
	wsb := util.SaveBlock(t, ctx, service.cfg.BeaconDB, b66)

	beaconState, _ := util.DeterministicGenesisState(t, 32)

	// Set finalized epoch to 2.
	require.NoError(t, service.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: 2, Root: r64}))
	err = service.fillInForkChoiceMissingBlocks(
		t.Context(), wsb, beaconState.FinalizedCheckpoint(), beaconState.CurrentJustifiedCheckpoint())
	require.NoError(t, err)

	// There should be 1 node: block 65
	assert.Equal(t, 1, service.cfg.ForkChoiceStore.NodeCount(), "Miss match nodes")
	assert.Equal(t, true, service.cfg.ForkChoiceStore.HasNode(r65), "Didn't save node")
}

func TestFillForkChoiceMissingBlocks_FinalizedSibling(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB := tr.ctx, tr.db

	var genesisStateRoot [32]byte
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	validGenesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	st, err := util.NewBeaconState()
	require.NoError(t, err)

	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st.Copy(), validGenesisRoot))
	roots, err := blockTree1(t, beaconDB, validGenesisRoot[:])
	require.NoError(t, err)

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	blk := util.NewBeaconBlock()
	blk.Block.Slot = 9
	blk.Block.ParentRoot = roots[8]

	wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	err = service.fillInForkChoiceMissingBlocks(
		t.Context(), wsb, beaconState.FinalizedCheckpoint(), beaconState.CurrentJustifiedCheckpoint())
	require.Equal(t, ErrNotDescendantOfFinalized.Error(), err.Error())
}

func TestFillForkChoiceMissingBlocks_ErrorCases(t *testing.T) {
	tests := []struct {
		name           string
		finalizedEpoch primitives.Epoch
		justifiedEpoch primitives.Epoch
		expectedError  error
	}{
		{
			name:           "finalized epoch greater than justified epoch",
			finalizedEpoch: 5,
			justifiedEpoch: 3,
			expectedError:  ErrInvalidCheckpointArgs,
		},
		{
			name:           "valid case - finalized equal to justified",
			finalizedEpoch: 3,
			justifiedEpoch: 3,
			expectedError:  nil,
		},
		{
			name:           "valid case - finalized less than justified",
			finalizedEpoch: 2,
			justifiedEpoch: 3,
			expectedError:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, tr := minimalTestService(t)
			ctx, beaconDB := tr.ctx, tr.db

			st, _ := util.DeterministicGenesisState(t, 64)
			require.NoError(t, service.saveGenesisData(ctx, st))

			// Create a simple block for testing
			blk := util.NewBeaconBlock()
			blk.Block.Slot = 10
			blk.Block.ParentRoot = service.originBlockRoot[:]
			wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)
			util.SaveBlock(t, ctx, beaconDB, blk)

			// Create checkpoints with test case epochs
			finalizedCheckpoint := &ethpb.Checkpoint{
				Epoch: tt.finalizedEpoch,
				Root:  service.originBlockRoot[:],
			}
			justifiedCheckpoint := &ethpb.Checkpoint{
				Epoch: tt.justifiedEpoch,
				Root:  service.originBlockRoot[:],
			}

			// Set up forkchoice store to avoid other errors
			fcp := &ethpb.Checkpoint{Epoch: 0, Root: service.originBlockRoot[:]}
			state, blkRoot, err := prepareForkchoiceState(ctx, 0, service.originBlockRoot, service.originBlockRoot, [32]byte{}, fcp, fcp)
			require.NoError(t, err)
			require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))

			err = service.fillInForkChoiceMissingBlocks(
				t.Context(), wsb, finalizedCheckpoint, justifiedCheckpoint)

			if tt.expectedError != nil {
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				// For valid cases, we might get other errors (like block not being descendant of finalized)
				// but we shouldn't get the checkpoint validation error
				if err != nil && errors.Is(err, tt.expectedError) {
					t.Errorf("Unexpected checkpoint validation error: %v", err)
				}
			}
		})
	}
}

// blockTree1 constructs the following tree:
//
//	/- B1
//
// B0           /- B5 - B7
//
//	\- B3 - B4 - B6 - B8
func blockTree1(t *testing.T, beaconDB db.Database, genesisRoot []byte) ([][]byte, error) {
	genesisRoot = bytesutil.PadTo(genesisRoot, 32)
	b0 := util.NewBeaconBlock()
	b0.Block.Slot = 0
	b0.Block.ParentRoot = genesisRoot
	r0, err := b0.Block.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	b1 := util.NewBeaconBlock()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = r0[:]
	r1, err := b1.Block.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	b3 := util.NewBeaconBlock()
	b3.Block.Slot = 3
	b3.Block.ParentRoot = r0[:]
	r3, err := b3.Block.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	b4 := util.NewBeaconBlock()
	b4.Block.Slot = 4
	b4.Block.ParentRoot = r3[:]
	r4, err := b4.Block.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	b5 := util.NewBeaconBlock()
	b5.Block.Slot = 5
	b5.Block.ParentRoot = r4[:]
	r5, err := b5.Block.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	b6 := util.NewBeaconBlock()
	b6.Block.Slot = 6
	b6.Block.ParentRoot = r4[:]
	r6, err := b6.Block.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	b7 := util.NewBeaconBlock()
	b7.Block.Slot = 7
	b7.Block.ParentRoot = r5[:]
	r7, err := b7.Block.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	b8 := util.NewBeaconBlock()
	b8.Block.Slot = 8
	b8.Block.ParentRoot = r6[:]
	r8, err := b8.Block.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	st, err := util.NewBeaconState()
	require.NoError(t, err)

	for _, b := range []*ethpb.SignedBeaconBlock{b0, b1, b3, b4, b5, b6, b7, b8} {
		beaconBlock := util.NewBeaconBlock()
		beaconBlock.Block.Slot = b.Block.Slot
		beaconBlock.Block.ParentRoot = bytesutil.PadTo(b.Block.ParentRoot, 32)
		wsb, err := consensusblocks.NewSignedBeaconBlock(beaconBlock)
		require.NoError(t, err)
		if err := beaconDB.SaveBlock(t.Context(), wsb); err != nil {
			return nil, err
		}
		if err := beaconDB.SaveState(t.Context(), st.Copy(), bytesutil.ToBytes32(beaconBlock.Block.ParentRoot)); err != nil {
			return nil, errors.Wrap(err, "could not save state")
		}
	}
	if err := beaconDB.SaveState(t.Context(), st.Copy(), r1); err != nil {
		return nil, err
	}
	if err := beaconDB.SaveState(t.Context(), st.Copy(), r7); err != nil {
		return nil, err
	}
	if err := beaconDB.SaveState(t.Context(), st.Copy(), r8); err != nil {
		return nil, err
	}
	return [][]byte{r0[:], r1[:], nil, r3[:], r4[:], r5[:], r6[:], r7[:], r8[:]}, nil
}

func TestCurrentSlot_HandlesOverflow(t *testing.T) {
	svc := testServiceNoDB(t)
	svc.genesisTime = prysmTime.Now().Add(1 * time.Hour)

	slot := svc.CurrentSlot()
	require.Equal(t, primitives.Slot(0), slot, "Unexpected slot")
}
func TestAncestorByDB_CtxErr(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	opts := testServiceOptsWithDB(t)
	service, err := NewService(ctx, opts...)
	require.NoError(t, err)

	cancel()
	_, err = service.ancestorByDB(ctx, [32]byte{}, 0)
	require.ErrorContains(t, "context canceled", err)
}

func TestAncestor_HandleSkipSlot(t *testing.T) {
	service, tr := minimalTestService(t)
	beaconDB := tr.db

	b1 := util.NewBeaconBlock()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	b100 := util.NewBeaconBlock()
	b100.Block.Slot = 100
	b100.Block.ParentRoot = r1[:]
	r100, err := b100.Block.HashTreeRoot()
	require.NoError(t, err)
	b200 := util.NewBeaconBlock()
	b200.Block.Slot = 200
	b200.Block.ParentRoot = r100[:]
	r200, err := b200.Block.HashTreeRoot()
	require.NoError(t, err)
	for _, b := range []*ethpb.SignedBeaconBlock{b1, b100, b200} {
		beaconBlock := util.NewBeaconBlock()
		beaconBlock.Block.Slot = b.Block.Slot
		beaconBlock.Block.ParentRoot = bytesutil.PadTo(b.Block.ParentRoot, 32)
		util.SaveBlock(t, t.Context(), beaconDB, beaconBlock)
	}

	// Slots 100 to 200 are skip slots. Requesting root at 150 will yield root at 100. The last physical block.
	r, err := service.Ancestor(t.Context(), r200[:], 150)
	require.NoError(t, err)
	if bytesutil.ToBytes32(r) != r100 {
		t.Error("Did not get correct root")
	}

	// Slots 1 to 100 are skip slots. Requesting root at 50 will yield root at 1. The last physical block.
	r, err = service.Ancestor(t.Context(), r200[:], 50)
	require.NoError(t, err)
	if bytesutil.ToBytes32(r) != r1 {
		t.Error("Did not get correct root")
	}
}

func TestAncestor_CanUseForkchoice(t *testing.T) {
	ctx := t.Context()
	opts := testServiceOptsWithDB(t)
	service, err := NewService(ctx, opts...)
	require.NoError(t, err)

	b1 := util.NewBeaconBlock()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	b100 := util.NewBeaconBlock()
	b100.Block.Slot = 100
	b100.Block.ParentRoot = r1[:]
	r100, err := b100.Block.HashTreeRoot()
	require.NoError(t, err)
	b200 := util.NewBeaconBlock()
	b200.Block.Slot = 200
	b200.Block.ParentRoot = r100[:]
	r200, err := b200.Block.HashTreeRoot()
	require.NoError(t, err)
	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	for _, b := range []*ethpb.SignedBeaconBlock{b1, b100, b200} {
		beaconBlock := util.NewBeaconBlock()
		beaconBlock.Block.Slot = b.Block.Slot
		beaconBlock.Block.ParentRoot = bytesutil.PadTo(b.Block.ParentRoot, 32)
		r, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		st, blkRoot, err := prepareForkchoiceState(t.Context(), b.Block.Slot, r, bytesutil.ToBytes32(b.Block.ParentRoot), params.BeaconConfig().ZeroHash, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))
	}

	r, err := service.Ancestor(t.Context(), r200[:], 150)
	require.NoError(t, err)
	if bytesutil.ToBytes32(r) != r100 {
		t.Error("Did not get correct root")
	}
}

func TestAncestor_CanUseDB(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB := tr.ctx, tr.db

	b1 := util.NewBeaconBlock()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	b100 := util.NewBeaconBlock()
	b100.Block.Slot = 100
	b100.Block.ParentRoot = r1[:]
	r100, err := b100.Block.HashTreeRoot()
	require.NoError(t, err)
	b200 := util.NewBeaconBlock()
	b200.Block.Slot = 200
	b200.Block.ParentRoot = r100[:]
	r200, err := b200.Block.HashTreeRoot()
	require.NoError(t, err)
	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	for _, b := range []*ethpb.SignedBeaconBlock{b1, b100, b200} {
		beaconBlock := util.NewBeaconBlock()
		beaconBlock.Block.Slot = b.Block.Slot
		beaconBlock.Block.ParentRoot = bytesutil.PadTo(b.Block.ParentRoot, 32)
		util.SaveBlock(t, t.Context(), beaconDB, beaconBlock)
	}

	st, blkRoot, err := prepareForkchoiceState(t.Context(), 200, r200, r200, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))

	r, err := service.Ancestor(t.Context(), r200[:], 150)
	require.NoError(t, err)
	if bytesutil.ToBytes32(r) != r100 {
		t.Error("Did not get correct root")
	}
}

func TestEnsureRootNotZeroHashes(t *testing.T) {
	ctx := t.Context()
	opts := testServiceOptsNoDB()
	service, err := NewService(ctx, opts...)
	require.NoError(t, err)
	service.originBlockRoot = [32]byte{'a'}

	r := service.ensureRootNotZeros(params.BeaconConfig().ZeroHash)
	assert.Equal(t, service.originBlockRoot, r, "Did not get wanted justified root")
	root := [32]byte{'b'}
	r = service.ensureRootNotZeros(root)
	assert.Equal(t, root, r, "Did not get wanted justified root")
}

func TestHandleEpochBoundary_UpdateFirstSlot(t *testing.T) {
	ctx := t.Context()
	opts := testServiceOptsNoDB()
	service, err := NewService(ctx, opts...)
	require.NoError(t, err)

	s, _ := util.DeterministicGenesisState(t, 1024)
	service.head = &head{state: s}
	require.NoError(t, s.SetSlot(2*params.BeaconConfig().SlotsPerEpoch))
	require.NoError(t, service.handleEpochBoundary(ctx, s.Slot(), s, []byte{}))
}

func TestOnBlock_CanFinalize_WithOnTick(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, fcs := tr.ctx, tr.fcs

	gs, keys := util.DeterministicGenesisState(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))
	require.NoError(t, fcs.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Root: service.originBlockRoot}))

	testState := gs.Copy()
	for i := primitives.Slot(1); i <= 4*params.BeaconConfig().SlotsPerEpoch; i++ {
		blk, err := util.GenerateFullBlock(testState, keys, util.DefaultBlockGenConfig(), i)
		require.NoError(t, err)
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		require.NoError(t, fcs.NewSlot(ctx, i))
		// Save current justified and finalized epochs for future use.
		currStoreJustifiedEpoch := service.CurrentJustifiedCheckpt().Epoch
		currStoreFinalizedEpoch := service.FinalizedCheckpt().Epoch

		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, r, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, r)
		require.NoError(t, err)
		require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, true}))
		require.NoError(t, service.updateJustificationOnBlock(ctx, preState, postState, currStoreJustifiedEpoch))
		_, err = service.updateFinalizationOnBlock(ctx, preState, postState, currStoreFinalizedEpoch)
		require.NoError(t, err)

		testState, err = service.cfg.StateGen.StateByRoot(ctx, r)
		require.NoError(t, err)
	}
	cp := service.CurrentJustifiedCheckpt()
	require.Equal(t, primitives.Epoch(3), cp.Epoch)
	cp = service.FinalizedCheckpt()
	require.Equal(t, primitives.Epoch(2), cp.Epoch)

	// The update should persist in DB.
	j, err := service.cfg.BeaconDB.JustifiedCheckpoint(ctx)
	require.NoError(t, err)
	cp = service.CurrentJustifiedCheckpt()
	require.Equal(t, j.Epoch, cp.Epoch)
	f, err := service.cfg.BeaconDB.FinalizedCheckpoint(ctx)
	require.NoError(t, err)
	cp = service.FinalizedCheckpt()
	require.Equal(t, f.Epoch, cp.Epoch)
}

func TestOnBlock_CanFinalize(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	gs, keys := util.DeterministicGenesisState(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))

	testState := gs.Copy()
	for i := primitives.Slot(1); i <= 4*params.BeaconConfig().SlotsPerEpoch; i++ {
		blk, err := util.GenerateFullBlock(testState, keys, util.DefaultBlockGenConfig(), i)
		require.NoError(t, err)
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		// Save current justified and finalized epochs for future use.
		currStoreJustifiedEpoch := service.CurrentJustifiedCheckpt().Epoch
		currStoreFinalizedEpoch := service.FinalizedCheckpt().Epoch

		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, r, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, r)
		require.NoError(t, err)
		require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, true}))
		require.NoError(t, service.updateJustificationOnBlock(ctx, preState, postState, currStoreJustifiedEpoch))
		_, err = service.updateFinalizationOnBlock(ctx, preState, postState, currStoreFinalizedEpoch)
		require.NoError(t, err)

		testState, err = service.cfg.StateGen.StateByRoot(ctx, r)
		require.NoError(t, err)
	}
	cp := service.CurrentJustifiedCheckpt()
	require.Equal(t, primitives.Epoch(3), cp.Epoch)
	cp = service.FinalizedCheckpt()
	require.Equal(t, primitives.Epoch(2), cp.Epoch)

	// The update should persist in DB.
	j, err := service.cfg.BeaconDB.JustifiedCheckpoint(ctx)
	require.NoError(t, err)
	cp = service.CurrentJustifiedCheckpt()
	require.Equal(t, j.Epoch, cp.Epoch)
	f, err := service.cfg.BeaconDB.FinalizedCheckpoint(ctx)
	require.NoError(t, err)
	cp = service.FinalizedCheckpt()
	require.Equal(t, f.Epoch, cp.Epoch)
}

func TestOnBlock_NilBlock(t *testing.T) {
	service, tr := minimalTestService(t)
	signed := &consensusblocks.SignedBeaconBlock{}
	roblock := consensusblocks.ROBlock{ReadOnlySignedBeaconBlock: signed}
	err := service.postBlockProcess(&postBlockProcessConfig{tr.ctx, roblock, [32]byte{}, nil, true})
	require.Equal(t, true, IsInvalidBlock(err))
}

func TestOnBlock_InvalidSignature(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	gs, keys := util.DeterministicGenesisState(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))

	blk, err := util.GenerateFullBlock(gs, keys, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	blk.Signature = []byte{'a'} // Mutate the signature.
	wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	preState, err := service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	_, err = service.validateStateTransition(ctx, preState, wsb)
	require.Equal(t, true, IsInvalidBlock(err))
}

func TestOnBlock_CallNewPayloadAndForkchoiceUpdated(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 1
	config.BellatrixForkEpoch = 2
	params.OverrideBeaconConfig(config)

	service, tr := minimalTestService(t)
	ctx := tr.ctx

	gs, keys := util.DeterministicGenesisState(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))
	testState := gs.Copy()
	for i := primitives.Slot(1); i < params.BeaconConfig().SlotsPerEpoch; i++ {
		blk, err := util.GenerateFullBlock(testState, keys, util.DefaultBlockGenConfig(), i)
		require.NoError(t, err)
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)

		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, r, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, r)
		require.NoError(t, err)
		require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false}))
		testState, err = service.cfg.StateGen.StateByRoot(ctx, r)
		require.NoError(t, err)
	}
}

func TestInsertFinalizedDeposits(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, depositCache := tr.ctx, tr.dc

	gs, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))
	gs = gs.Copy()
	assert.NoError(t, gs.SetEth1Data(&ethpb.Eth1Data{DepositCount: 10, BlockHash: make([]byte, 32)}))
	assert.NoError(t, gs.SetEth1DepositIndex(8))
	assert.NoError(t, service.cfg.StateGen.SaveState(ctx, [32]byte{'m', 'o', 'c', 'k'}, gs))
	var zeroSig [96]byte
	for i := uint64(0); i < uint64(4*params.BeaconConfig().SlotsPerEpoch); i++ {
		root := []byte(strconv.Itoa(int(i)))
		assert.NoError(t, depositCache.InsertDeposit(ctx, &ethpb.Deposit{Data: &ethpb.Deposit_Data{
			PublicKey:             bytesutil.FromBytes48([fieldparams.BLSPubkeyLength]byte{}),
			WithdrawalCredentials: params.BeaconConfig().ZeroHash[:],
			Amount:                0,
			Signature:             zeroSig[:],
		}, Proof: [][]byte{root}}, 100+i, int64(i), bytesutil.ToBytes32(root)))
	}
	service.insertFinalizedDepositsAndPrune(ctx, [32]byte{'m', 'o', 'c', 'k'})
	fDeposits, err := depositCache.FinalizedDeposits(ctx)
	require.NoError(t, err)
	assert.Equal(t, 7, int(fDeposits.MerkleTrieIndex()), "Finalized deposits not inserted correctly")
	deps := depositCache.AllDeposits(ctx, big.NewInt(107))
	for _, d := range deps {
		assert.DeepEqual(t, [][]byte(nil), d.Proof, "Proofs are not empty")
	}
}

func TestInsertFinalizedDeposits_PrunePendingDeposits(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, depositCache := tr.ctx, tr.dc

	gs, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))
	gs = gs.Copy()
	assert.NoError(t, gs.SetEth1Data(&ethpb.Eth1Data{DepositCount: 10, BlockHash: make([]byte, 32)}))
	assert.NoError(t, gs.SetEth1DepositIndex(8))
	assert.NoError(t, service.cfg.StateGen.SaveState(ctx, [32]byte{'m', 'o', 'c', 'k'}, gs))
	var zeroSig [96]byte
	for i := uint64(0); i < uint64(4*params.BeaconConfig().SlotsPerEpoch); i++ {
		root := []byte(strconv.Itoa(int(i)))
		assert.NoError(t, depositCache.InsertDeposit(ctx, &ethpb.Deposit{Data: &ethpb.Deposit_Data{
			PublicKey:             bytesutil.FromBytes48([fieldparams.BLSPubkeyLength]byte{}),
			WithdrawalCredentials: params.BeaconConfig().ZeroHash[:],
			Amount:                0,
			Signature:             zeroSig[:],
		}, Proof: [][]byte{root}}, 100+i, int64(i), bytesutil.ToBytes32(root)))
		depositCache.InsertPendingDeposit(ctx, &ethpb.Deposit{Data: &ethpb.Deposit_Data{
			PublicKey:             bytesutil.FromBytes48([fieldparams.BLSPubkeyLength]byte{}),
			WithdrawalCredentials: params.BeaconConfig().ZeroHash[:],
			Amount:                0,
			Signature:             zeroSig[:],
		}, Proof: [][]byte{root}}, 100+i, int64(i), bytesutil.ToBytes32(root))
	}
	service.insertFinalizedDepositsAndPrune(ctx, [32]byte{'m', 'o', 'c', 'k'})
	fDeposits, err := depositCache.FinalizedDeposits(ctx)
	require.NoError(t, err)
	assert.Equal(t, 7, int(fDeposits.MerkleTrieIndex()), "Finalized deposits not inserted correctly")
	deps := depositCache.AllDeposits(ctx, big.NewInt(107))
	for _, d := range deps {
		assert.DeepEqual(t, [][]byte(nil), d.Proof, "Proofs are not empty")
	}
	pendingDeps := depositCache.PendingContainers(ctx, nil)
	for _, d := range pendingDeps {
		assert.DeepEqual(t, true, d.Index >= 8, "Pending deposits were not pruned")
	}
}

func TestInsertFinalizedDeposits_MultipleFinalizedRoutines(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, depositCache := tr.ctx, tr.dc

	gs, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))
	gs = gs.Copy()
	assert.NoError(t, gs.SetEth1Data(&ethpb.Eth1Data{DepositCount: 7, BlockHash: make([]byte, 32)}))
	assert.NoError(t, gs.SetEth1DepositIndex(6))
	assert.NoError(t, service.cfg.StateGen.SaveState(ctx, [32]byte{'m', 'o', 'c', 'k'}, gs))
	gs2 := gs.Copy()
	assert.NoError(t, gs2.SetEth1Data(&ethpb.Eth1Data{DepositCount: 15, BlockHash: make([]byte, 32)}))
	assert.NoError(t, gs2.SetEth1DepositIndex(13))
	assert.NoError(t, service.cfg.StateGen.SaveState(ctx, [32]byte{'m', 'o', 'c', 'k', '2'}, gs2))
	var zeroSig [96]byte
	for i := uint64(0); i < uint64(4*params.BeaconConfig().SlotsPerEpoch); i++ {
		root := []byte(strconv.Itoa(int(i)))
		assert.NoError(t, depositCache.InsertDeposit(ctx, &ethpb.Deposit{Data: &ethpb.Deposit_Data{
			PublicKey:             bytesutil.FromBytes48([fieldparams.BLSPubkeyLength]byte{}),
			WithdrawalCredentials: params.BeaconConfig().ZeroHash[:],
			Amount:                0,
			Signature:             zeroSig[:],
		}, Proof: [][]byte{root}}, 100+i, int64(i), bytesutil.ToBytes32(root)))
	}
	// Insert 3 deposits before hand.
	require.NoError(t, depositCache.InsertFinalizedDeposits(ctx, 2, [32]byte{}, 0))
	service.insertFinalizedDepositsAndPrune(ctx, [32]byte{'m', 'o', 'c', 'k'})
	fDeposits, err := depositCache.FinalizedDeposits(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, int(fDeposits.MerkleTrieIndex()), "Finalized deposits not inserted correctly")

	deps := depositCache.AllDeposits(ctx, big.NewInt(105))
	for _, d := range deps {
		assert.DeepEqual(t, [][]byte(nil), d.Proof, "Proofs are not empty")
	}

	// Insert New Finalized State with higher deposit count.
	service.insertFinalizedDepositsAndPrune(ctx, [32]byte{'m', 'o', 'c', 'k', '2'})
	fDeposits, err = depositCache.FinalizedDeposits(ctx)
	require.NoError(t, err)
	assert.Equal(t, 12, int(fDeposits.MerkleTrieIndex()), "Finalized deposits not inserted correctly")
	deps = depositCache.AllDeposits(ctx, big.NewInt(112))
	for _, d := range deps {
		assert.DeepEqual(t, [][]byte(nil), d.Proof, "Proofs are not empty")
	}
}

func TestRemoveBlockAttestationsInPool(t *testing.T) {
	logHook := logTest.NewGlobal()

	genesis, keys := util.DeterministicGenesisState(t, 64)
	b, err := util.GenerateFullBlock(genesis, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Root: r[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveGenesisBlockRoot(ctx, r))

	atts := make([]ethpb.Att, len(b.Block.Body.Attestations))
	for i, a := range b.Block.Body.Attestations {
		atts[i] = a
	}
	require.NoError(t, service.cfg.AttPool.SaveAggregatedAttestations(atts))
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	service.pruneAttsFromPool(t.Context(), nil /* state not needed pre-Electra */, wsb)
	require.LogsDoNotContain(t, logHook, "Could not prune attestations")
	require.Equal(t, 0, service.cfg.AttPool.AggregatedAttestationCount())
}

func Test_getStateVersionAndPayload(t *testing.T) {
	tests := []struct {
		name    string
		st      state.BeaconState
		version int
		header  *enginev1.ExecutionPayloadHeader
	}{
		{
			name: "phase 0 state",
			st: func() state.BeaconState {
				s, _ := util.DeterministicGenesisState(t, 1)
				return s
			}(),
			version: version.Phase0,
			header:  (*enginev1.ExecutionPayloadHeader)(nil),
		},
		{
			name: "altair state",
			st: func() state.BeaconState {
				s, _ := util.DeterministicGenesisStateAltair(t, 1)
				return s
			}(),
			version: version.Altair,
			header:  (*enginev1.ExecutionPayloadHeader)(nil),
		},
		{
			name: "bellatrix state",
			st: func() state.BeaconState {
				s, _ := util.DeterministicGenesisStateBellatrix(t, 1)
				wrappedHeader, err := consensusblocks.WrappedExecutionPayloadHeader(&enginev1.ExecutionPayloadHeader{
					BlockNumber: 1,
				})
				require.NoError(t, err)
				require.NoError(t, s.SetLatestExecutionPayloadHeader(wrappedHeader))
				return s
			}(),
			version: version.Bellatrix,
			header: &enginev1.ExecutionPayloadHeader{
				BlockNumber: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver, header, err := getStateVersionAndPayload(tt.st)
			require.NoError(t, err)
			require.Equal(t, tt.version, ver)
			if header != nil {
				protoHeader, ok := header.Proto().(*enginev1.ExecutionPayloadHeader)
				require.Equal(t, true, ok)
				require.DeepEqual(t, tt.header, protoHeader)
			}
		})
	}
}

func Test_validateMergeTransitionBlock(t *testing.T) {
	cfg := params.BeaconConfig()
	cfg.TerminalTotalDifficulty = "2"
	cfg.TerminalBlockHash = params.BeaconConfig().ZeroHash
	params.OverrideBeaconConfig(cfg)

	service, tr := minimalTestService(t, WithPayloadIDCache(cache.NewPayloadIDCache()))
	ctx := tr.ctx

	aHash := common.BytesToHash([]byte("a"))
	bHash := common.BytesToHash([]byte("b"))

	tests := []struct {
		name         string
		stateVersion int
		header       interfaces.ExecutionData
		payload      *enginev1.ExecutionPayload
		errString    string
	}{
		{
			name:         "state older than Bellatrix, nil payload",
			stateVersion: 1,
			payload:      nil,
		},
		{
			name:         "state older than Bellatrix, empty payload",
			stateVersion: 1,
			payload: &enginev1.ExecutionPayload{
				ParentHash:    make([]byte, fieldparams.RootLength),
				FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:     make([]byte, fieldparams.RootLength),
				ReceiptsRoot:  make([]byte, fieldparams.RootLength),
				LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:    make([]byte, fieldparams.RootLength),
				ExtraData:     make([]byte, 0),
				BaseFeePerGas: make([]byte, fieldparams.RootLength),
				BlockHash:     make([]byte, fieldparams.RootLength),
				Transactions:  make([][]byte, 0),
			},
		},
		{
			name:         "state older than Bellatrix, non empty payload",
			stateVersion: 1,
			payload: &enginev1.ExecutionPayload{
				ParentHash: aHash[:],
			},
		},
		{
			name:         "state is Bellatrix, nil payload",
			stateVersion: 2,
			payload:      nil,
		},
		{
			name:         "state is Bellatrix, empty payload",
			stateVersion: 2,
			payload: &enginev1.ExecutionPayload{
				ParentHash:    make([]byte, fieldparams.RootLength),
				FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:     make([]byte, fieldparams.RootLength),
				ReceiptsRoot:  make([]byte, fieldparams.RootLength),
				LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:    make([]byte, fieldparams.RootLength),
				BaseFeePerGas: make([]byte, fieldparams.RootLength),
				BlockHash:     make([]byte, fieldparams.RootLength),
			},
		},
		{
			name:         "state is Bellatrix, non empty payload, empty header",
			stateVersion: 2,
			payload: &enginev1.ExecutionPayload{
				ParentHash: aHash[:],
			},
			header: func() interfaces.ExecutionData {
				h, err := consensusblocks.WrappedExecutionPayloadHeader(&enginev1.ExecutionPayloadHeader{
					ParentHash:       make([]byte, fieldparams.RootLength),
					FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
					StateRoot:        make([]byte, fieldparams.RootLength),
					ReceiptsRoot:     make([]byte, fieldparams.RootLength),
					LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
					PrevRandao:       make([]byte, fieldparams.RootLength),
					ExtraData:        make([]byte, 0),
					BaseFeePerGas:    make([]byte, fieldparams.RootLength),
					BlockHash:        make([]byte, fieldparams.RootLength),
					TransactionsRoot: make([]byte, fieldparams.RootLength),
				})
				require.NoError(t, err)
				return h
			}(),
		},
		{
			name:         "state is Bellatrix, non empty payload, non empty header",
			stateVersion: 2,
			payload: &enginev1.ExecutionPayload{
				ParentHash: aHash[:],
			},
			header: func() interfaces.ExecutionData {
				h, err := consensusblocks.WrappedExecutionPayloadHeader(&enginev1.ExecutionPayloadHeader{
					BlockNumber: 1,
				})
				require.NoError(t, err)
				return h
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &mockExecution.EngineClient{BlockByHashMap: map[[32]byte]*enginev1.ExecutionBlock{}}
			e.BlockByHashMap[aHash] = &enginev1.ExecutionBlock{
				Header: gethtypes.Header{
					ParentHash: bHash,
				},
				TotalDifficulty: "0x2",
			}
			e.BlockByHashMap[bHash] = &enginev1.ExecutionBlock{
				Header: gethtypes.Header{
					ParentHash: common.BytesToHash([]byte("3")),
				},
				TotalDifficulty: "0x1",
			}
			service.cfg.ExecutionEngineCaller = e
			b := util.HydrateSignedBeaconBlockBellatrix(&ethpb.SignedBeaconBlockBellatrix{})
			b.Block.Body.ExecutionPayload = tt.payload
			blk, err := consensusblocks.NewSignedBeaconBlock(b)
			require.NoError(t, err)
			err = service.validateMergeTransitionBlock(ctx, tt.stateVersion, tt.header, blk)
			if tt.errString != "" {
				require.ErrorContains(t, tt.errString, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestService_insertSlashingsToForkChoiceStore(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	att1 := util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{Epoch: 1},
		},
		AttestingIndices: []uint64{0, 1},
	})
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(att1.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sig0 := privKeys[0].Sign(signingRoot[:])
	sig1 := privKeys[1].Sign(signingRoot[:])
	aggregateSig := bls.AggregateSignatures([]bls.Signature{sig0, sig1})
	att1.Signature = aggregateSig.Marshal()

	att2 := util.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
		AttestingIndices: []uint64{0, 1},
	})
	signingRoot, err = signing.ComputeSigningRoot(att2.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sig0 = privKeys[0].Sign(signingRoot[:])
	sig1 = privKeys[1].Sign(signingRoot[:])
	aggregateSig = bls.AggregateSignatures([]bls.Signature{sig0, sig1})
	att2.Signature = aggregateSig.Marshal()
	slashings := []*ethpb.AttesterSlashing{
		{
			Attestation_1: att1,
			Attestation_2: att2,
		},
	}
	b := util.NewBeaconBlock()
	b.Block.Body.AttesterSlashings = slashings
	wb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	service.InsertSlashingsToForkChoiceStore(ctx, wb.Block().Body().AttesterSlashings())
}

func TestService_insertSlashingsToForkChoiceStoreElectra(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	beaconState, privKeys := util.DeterministicGenesisStateElectra(t, 100)
	att1 := util.HydrateIndexedAttestationElectra(&ethpb.IndexedAttestationElectra{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{Epoch: 1},
		},
		AttestingIndices: []uint64{0, 1},
	})
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(att1.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sig0 := privKeys[0].Sign(signingRoot[:])
	sig1 := privKeys[1].Sign(signingRoot[:])
	aggregateSig := bls.AggregateSignatures([]bls.Signature{sig0, sig1})
	att1.Signature = aggregateSig.Marshal()

	att2 := util.HydrateIndexedAttestationElectra(&ethpb.IndexedAttestationElectra{
		AttestingIndices: []uint64{0, 1},
	})
	signingRoot, err = signing.ComputeSigningRoot(att2.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sig0 = privKeys[0].Sign(signingRoot[:])
	sig1 = privKeys[1].Sign(signingRoot[:])
	aggregateSig = bls.AggregateSignatures([]bls.Signature{sig0, sig1})
	att2.Signature = aggregateSig.Marshal()
	slashings := []*ethpb.AttesterSlashingElectra{
		{
			Attestation_1: att1,
			Attestation_2: att2,
		},
	}
	b := util.NewBeaconBlockElectra()
	b.Block.Body.AttesterSlashings = slashings
	wb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	service.InsertSlashingsToForkChoiceStore(ctx, wb.Block().Body().AttesterSlashings())
}

func TestOnBlock_ProcessBlocksParallel(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	gs, keys := util.DeterministicGenesisState(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))

	blk1, err := util.GenerateFullBlock(gs, keys, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb1, err := consensusblocks.NewSignedBeaconBlock(blk1)
	require.NoError(t, err)
	blk2, err := util.GenerateFullBlock(gs, keys, util.DefaultBlockGenConfig(), 2)
	require.NoError(t, err)
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb2, err := consensusblocks.NewSignedBeaconBlock(blk2)
	require.NoError(t, err)
	blk3, err := util.GenerateFullBlock(gs, keys, util.DefaultBlockGenConfig(), 3)
	require.NoError(t, err)
	r3, err := blk3.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb3, err := consensusblocks.NewSignedBeaconBlock(blk3)
	require.NoError(t, err)
	blk4, err := util.GenerateFullBlock(gs, keys, util.DefaultBlockGenConfig(), 4)
	require.NoError(t, err)
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb4, err := consensusblocks.NewSignedBeaconBlock(blk4)
	require.NoError(t, err)

	logHook := logTest.NewGlobal()
	for i := 0; i < 10; i++ {
		fc := &ethpb.Checkpoint{}
		st, blkRoot, err := prepareForkchoiceState(ctx, 0, wsb1.Block().ParentRoot(), [32]byte{}, [32]byte{}, fc, fc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))
		var wg sync.WaitGroup
		wg.Add(4)
		var lock sync.Mutex
		go func() {
			preState, err := service.getBlockPreState(ctx, wsb1.Block())
			require.NoError(t, err)
			postState, err := service.validateStateTransition(ctx, preState, wsb1)
			require.NoError(t, err)
			lock.Lock()
			roblock, err := consensusblocks.NewROBlockWithRoot(wsb1, r1)
			require.NoError(t, err)
			require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, true}))
			lock.Unlock()
			wg.Done()
		}()
		go func() {
			preState, err := service.getBlockPreState(ctx, wsb2.Block())
			require.NoError(t, err)
			postState, err := service.validateStateTransition(ctx, preState, wsb2)
			require.NoError(t, err)
			lock.Lock()
			roblock, err := consensusblocks.NewROBlockWithRoot(wsb2, r2)
			require.NoError(t, err)
			require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, true}))
			lock.Unlock()
			wg.Done()
		}()
		go func() {
			preState, err := service.getBlockPreState(ctx, wsb3.Block())
			require.NoError(t, err)
			postState, err := service.validateStateTransition(ctx, preState, wsb3)
			require.NoError(t, err)
			lock.Lock()
			roblock, err := consensusblocks.NewROBlockWithRoot(wsb3, r3)
			require.NoError(t, err)
			require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, true}))
			lock.Unlock()
			wg.Done()
		}()
		go func() {
			preState, err := service.getBlockPreState(ctx, wsb4.Block())
			require.NoError(t, err)
			postState, err := service.validateStateTransition(ctx, preState, wsb4)
			require.NoError(t, err)
			lock.Lock()
			roblock, err := consensusblocks.NewROBlockWithRoot(wsb4, r4)
			require.NoError(t, err)
			require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, true}))
			lock.Unlock()
			wg.Done()
		}()
		wg.Wait()
		require.LogsDoNotContain(t, logHook, "New head does not exist in DB. Do nothing")
		require.NoError(t, service.cfg.BeaconDB.DeleteBlock(ctx, r1))
		require.NoError(t, service.cfg.BeaconDB.DeleteBlock(ctx, r2))
		require.NoError(t, service.cfg.BeaconDB.DeleteBlock(ctx, r3))
		require.NoError(t, service.cfg.BeaconDB.DeleteBlock(ctx, r4))
		service.cfg.ForkChoiceStore = doublylinkedtree.New()
	}
}

func Test_verifyBlkFinalizedSlot_invalidBlock(t *testing.T) {
	service, _ := minimalTestService(t)

	require.NoError(t, service.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: 1}))
	blk := util.HydrateBeaconBlock(&ethpb.BeaconBlock{Slot: 1})
	wb, err := consensusblocks.NewBeaconBlock(blk)
	require.NoError(t, err)
	err = service.verifyBlkFinalizedSlot(wb)
	require.Equal(t, true, IsInvalidBlock(err))
}

// See the description in #10777 and #10782 for the full setup
// We sync optimistically a chain of blocks. Block 17 is the last block in Epoch
// 2. Block 18 justifies block 12 (the first in Epoch 2) and Block 19 returns
// INVALID from FCU, with LVH block 17. No head is viable. We check
// that the node is optimistic and that we can actually import a block on top of
// 17 and recover.
func TestStore_NoViableHead_FCU(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.SlotsPerEpoch = 6
	config.AltairForkEpoch = 1
	config.BellatrixForkEpoch = 2
	params.OverrideBeaconConfig(config)

	mockEngine := &mockExecution.EngineClient{ErrNewPayload: execution.ErrAcceptedSyncingPayloadStatus, ErrForkchoiceUpdated: execution.ErrAcceptedSyncingPayloadStatus}
	service, tr := minimalTestService(t, WithExecutionEngineCaller(mockEngine))
	ctx := tr.ctx

	st, keys := util.DeterministicGenesisState(t, 64)
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

	for i := 1; i < 6; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)

		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false}))
	}

	for i := 6; i < 12; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlockAltair(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
		require.NoError(t, err)
	}

	for i := 12; i < 18; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
		require.NoError(t, err)
	}
	// Check that we haven't justified the second epoch yet
	jc := service.cfg.ForkChoiceStore.JustifiedCheckpoint()
	require.Equal(t, primitives.Epoch(0), jc.Epoch)

	// import a block that justifies the second epoch
	driftGenesisTime(service, 18, 0)
	validHeadState, err := service.HeadState(ctx)
	require.NoError(t, err)
	b, err := util.GenerateFullBlockBellatrix(validHeadState, keys, util.DefaultBlockGenConfig(), 18)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	firstInvalidRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	preState, err := service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err := service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, firstInvalidRoot, wsb, postState))
	roblock, err := consensusblocks.NewROBlockWithRoot(wsb, firstInvalidRoot)
	require.NoError(t, err)
	err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
	require.NoError(t, err)
	jc = service.cfg.ForkChoiceStore.JustifiedCheckpoint()
	require.Equal(t, primitives.Epoch(2), jc.Epoch)

	sjc := validHeadState.CurrentJustifiedCheckpoint()
	require.Equal(t, primitives.Epoch(0), sjc.Epoch)
	lvh := b.Block.Body.ExecutionPayload.ParentHash
	// check our head
	require.Equal(t, firstInvalidRoot, service.cfg.ForkChoiceStore.CachedHeadRoot())

	// import another block to find out that it was invalid
	mockEngine = &mockExecution.EngineClient{ErrNewPayload: execution.ErrAcceptedSyncingPayloadStatus, ErrForkchoiceUpdated: execution.ErrInvalidPayloadStatus, ForkChoiceUpdatedResp: lvh}
	service.cfg.ExecutionEngineCaller = mockEngine
	driftGenesisTime(service, 19, 0)
	st, err = service.HeadState(ctx)
	require.NoError(t, err)
	b, err = util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), 19)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	preState, err = service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err = service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
	roblock, err = consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
	require.ErrorContains(t, "received an INVALID payload from execution engine", err)
	// Check that forkchoice's head is the last invalid block imported. The
	// store's headroot is the previous head (since the invalid block did
	// not finish importing) one and that the node is optimistic
	require.Equal(t, root, service.cfg.ForkChoiceStore.CachedHeadRoot())
	headRoot, err := service.HeadRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, firstInvalidRoot, bytesutil.ToBytes32(headRoot))
	optimistic, err := service.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, true, optimistic)

	// import another block based on the last valid head state
	mockEngine = &mockExecution.EngineClient{}
	service.cfg.ExecutionEngineCaller = mockEngine
	driftGenesisTime(service, 20, 0)
	b, err = util.GenerateFullBlockBellatrix(validHeadState, keys, &util.BlockGenConfig{}, 20)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err = b.Block.HashTreeRoot()
	require.NoError(t, err)

	preState, err = service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err = service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
	roblock, err = consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, true})
	require.NoError(t, err)
	// Check the newly imported block is head, it justified the right
	// checkpoint and the node is no longer optimistic
	require.Equal(t, root, service.cfg.ForkChoiceStore.CachedHeadRoot())
	sjc = service.CurrentJustifiedCheckpt()
	require.Equal(t, jc.Epoch, sjc.Epoch)
	require.Equal(t, jc.Root, bytesutil.ToBytes32(sjc.Root))
	optimistic, err = service.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, false, optimistic)
}

// See the description in #10777 and #10782 for the full setup
// We sync optimistically a chain of blocks. Block 17 is the last block in Epoch
// 2. Block 18 justifies block 12 (the first in Epoch 2) and Block 19 returns
// INVALID from NewPayload, with LVH block 17. No head is viable. We check
// that the node is optimistic and that we can actually import a block on top of
// 17 and recover.
func TestStore_NoViableHead_NewPayload(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.SlotsPerEpoch = 6
	config.AltairForkEpoch = 1
	config.BellatrixForkEpoch = 2
	params.OverrideBeaconConfig(config)

	mockEngine := &mockExecution.EngineClient{ErrNewPayload: execution.ErrAcceptedSyncingPayloadStatus, ErrForkchoiceUpdated: execution.ErrAcceptedSyncingPayloadStatus}
	service, tr := minimalTestService(t, WithExecutionEngineCaller(mockEngine))
	ctx := tr.ctx

	st, keys := util.DeterministicGenesisState(t, 64)
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

	for i := 1; i < 6; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false}))
	}

	for i := 6; i < 12; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlockAltair(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
		require.NoError(t, err)
	}

	for i := 12; i < 18; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)

		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
		require.NoError(t, err)
	}
	// Check that we haven't justified the second epoch yet
	jc := service.cfg.ForkChoiceStore.JustifiedCheckpoint()
	require.Equal(t, primitives.Epoch(0), jc.Epoch)

	// import a block that justifies the second epoch
	driftGenesisTime(service, 18, 0)
	validHeadState, err := service.HeadState(ctx)
	require.NoError(t, err)
	b, err := util.GenerateFullBlockBellatrix(validHeadState, keys, util.DefaultBlockGenConfig(), 18)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	firstInvalidRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	preState, err := service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err := service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, firstInvalidRoot, wsb, postState))
	roblock, err := consensusblocks.NewROBlockWithRoot(wsb, firstInvalidRoot)
	require.NoError(t, err)
	err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
	require.NoError(t, err)
	jc = service.cfg.ForkChoiceStore.JustifiedCheckpoint()
	require.Equal(t, primitives.Epoch(2), jc.Epoch)

	sjc := validHeadState.CurrentJustifiedCheckpoint()
	require.Equal(t, primitives.Epoch(0), sjc.Epoch)
	lvh := b.Block.Body.ExecutionPayload.ParentHash
	// check our head
	require.Equal(t, firstInvalidRoot, service.cfg.ForkChoiceStore.CachedHeadRoot())

	// import another block to find out that it was invalid
	mockEngine = &mockExecution.EngineClient{ErrNewPayload: execution.ErrInvalidPayloadStatus, NewPayloadResp: lvh}
	service.cfg.ExecutionEngineCaller = mockEngine
	driftGenesisTime(service, 19, 0)
	st, err = service.HeadState(ctx)
	require.NoError(t, err)
	b, err = util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), 19)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	preState, err = service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	preStateVersion, preStateHeader, err := getStateVersionAndPayload(preState)
	require.NoError(t, err)
	rowsb, err := consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	_, err = service.validateExecutionOnBlock(ctx, preStateVersion, preStateHeader, rowsb)
	require.ErrorContains(t, "received an INVALID payload from execution engine", err)
	// Check that forkchoice's head and store's headroot are the previous head (since the invalid block did
	// not finish importing and it was never imported to forkchoice). Check
	// also that the node is optimistic
	require.Equal(t, firstInvalidRoot, service.cfg.ForkChoiceStore.CachedHeadRoot())
	headRoot, err := service.HeadRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, firstInvalidRoot, bytesutil.ToBytes32(headRoot))
	optimistic, err := service.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, true, optimistic)

	// import another block based on the last valid head state
	mockEngine = &mockExecution.EngineClient{}
	service.cfg.ExecutionEngineCaller = mockEngine
	driftGenesisTime(service, 20, 0)
	b, err = util.GenerateFullBlockBellatrix(validHeadState, keys, &util.BlockGenConfig{}, 20)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err = b.Block.HashTreeRoot()
	require.NoError(t, err)
	preState, err = service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err = service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
	roblock, err = consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, true})
	require.NoError(t, err)
	// Check the newly imported block is head, it justified the right
	// checkpoint and the node is no longer optimistic
	require.Equal(t, root, service.cfg.ForkChoiceStore.CachedHeadRoot())
	sjc = service.CurrentJustifiedCheckpt()
	require.Equal(t, jc.Epoch, sjc.Epoch)
	require.Equal(t, jc.Root, bytesutil.ToBytes32(sjc.Root))
	optimistic, err = service.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, false, optimistic)
}

// See the description in #10777 and #10782 for the full setup
// We sync optimistically a chain of blocks. Block 12 is the first block in Epoch
// 2 (and the merge block in this sequence). Block 18 justifies it and Block 19 returns
// INVALID from NewPayload, with LVH block 12. No head is viable. We check
// that the node is optimistic and that we can actually import a chain of blocks on top of
// 12 and recover. Notice that it takes two epochs to fully recover, and we stay
// optimistic for the whole time.
func TestStore_NoViableHead_Liveness(t *testing.T) {
	t.Skip("Requires #13664 to be fixed")
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.SlotsPerEpoch = 6
	config.AltairForkEpoch = 1
	config.BellatrixForkEpoch = 2
	params.OverrideBeaconConfig(config)

	mockEngine := &mockExecution.EngineClient{ErrNewPayload: execution.ErrAcceptedSyncingPayloadStatus, ErrForkchoiceUpdated: execution.ErrAcceptedSyncingPayloadStatus}
	service, tr := minimalTestService(t, WithExecutionEngineCaller(mockEngine))
	ctx := tr.ctx

	st, keys := util.DeterministicGenesisState(t, 64)
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

	for i := 1; i < 6; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)

		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false}))
	}

	for i := 6; i < 12; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlockAltair(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)

		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
		require.NoError(t, err)
	}

	// import the merge block
	driftGenesisTime(service, 12, 0)
	st, err = service.HeadState(ctx)
	require.NoError(t, err)
	b, err := util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), 12)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	lastValidRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	preState, err := service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err := service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, lastValidRoot, wsb, postState))
	roblock, err := consensusblocks.NewROBlockWithRoot(wsb, lastValidRoot)
	require.NoError(t, err)
	err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
	require.NoError(t, err)
	// save the post state and the payload Hash of this block since it will
	// be the LVH
	validHeadState, err := service.HeadState(ctx)
	require.NoError(t, err)
	lvh := b.Block.Body.ExecutionPayload.BlockHash
	validjc := validHeadState.CurrentJustifiedCheckpoint()
	require.Equal(t, primitives.Epoch(0), validjc.Epoch)

	// import blocks 13 through 18 to justify 12
	invalidRoots := make([][32]byte, 19-13)
	for i := 13; i < 19; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		invalidRoots[i-13], err = b.Block.HashTreeRoot()
		require.NoError(t, err)
		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, invalidRoots[i-13], wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, invalidRoots[i-13])
		require.NoError(t, err)
		err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
		require.NoError(t, err)
	}
	// Check that we have justified the second epoch
	jc := service.cfg.ForkChoiceStore.JustifiedCheckpoint()
	require.Equal(t, primitives.Epoch(2), jc.Epoch)
	invalidHeadRoot := service.cfg.ForkChoiceStore.CachedHeadRoot()

	// import block 19 to find out that the whole chain 13--18 was in fact
	// invalid
	mockEngine = &mockExecution.EngineClient{ErrNewPayload: execution.ErrInvalidPayloadStatus, NewPayloadResp: lvh}
	service.cfg.ExecutionEngineCaller = mockEngine
	driftGenesisTime(service, 19, 0)
	st, err = service.HeadState(ctx)
	require.NoError(t, err)
	b, err = util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), 19)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	preState, err = service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	preStateVersion, preStateHeader, err := getStateVersionAndPayload(preState)
	require.NoError(t, err)
	rowsb, err := consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	_, err = service.validateExecutionOnBlock(ctx, preStateVersion, preStateHeader, rowsb)
	require.ErrorContains(t, "received an INVALID payload from execution engine", err)

	// Check that forkchoice's head and store's headroot are the previous head (since the invalid block did
	// not finish importing and it was never imported to forkchoice). Check
	// also that the node is optimistic
	require.Equal(t, invalidHeadRoot, service.cfg.ForkChoiceStore.CachedHeadRoot())
	headRoot, err := service.HeadRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, invalidHeadRoot, bytesutil.ToBytes32(headRoot))
	optimistic, err := service.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, true, optimistic)

	// Check that the invalid blocks are not in database
	for i := 0; i < 19-13; i++ {
		require.Equal(t, false, service.cfg.BeaconDB.HasBlock(ctx, invalidRoots[i]))
	}

	// Check that the node's justified checkpoint does not agree with the
	// last valid state's justified checkpoint
	sjc := service.CurrentJustifiedCheckpt()
	require.Equal(t, primitives.Epoch(2), sjc.Epoch)

	// import another block based on the last valid head state
	mockEngine = &mockExecution.EngineClient{}
	service.cfg.ExecutionEngineCaller = mockEngine
	driftGenesisTime(service, 20, 0)
	b, err = util.GenerateFullBlockBellatrix(validHeadState, keys, &util.BlockGenConfig{}, 20)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err = b.Block.HashTreeRoot()
	require.NoError(t, err)
	preState, err = service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err = service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
	roblock, err = consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, true}))
	// Check that the head is still INVALID and the node is still optimistic
	require.Equal(t, invalidHeadRoot, service.cfg.ForkChoiceStore.CachedHeadRoot())
	optimistic, err = service.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, true, optimistic)
	st, err = service.cfg.StateGen.StateByRoot(ctx, root)
	require.NoError(t, err)
	// Import blocks 21--30 (Epoch 3 was not enough to justify 2)
	for i := 21; i < 30; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		require.NoError(t, err)
		b, err := util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, true})
		require.NoError(t, err)
		st, err = service.cfg.StateGen.StateByRoot(ctx, root)
		require.NoError(t, err)
	}
	// Head should still be INVALID and the node optimistic
	require.Equal(t, invalidHeadRoot, service.cfg.ForkChoiceStore.CachedHeadRoot())
	optimistic, err = service.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, true, optimistic)

	// Import block 30, it should justify Epoch 4 and become HEAD, the node
	// recovers
	driftGenesisTime(service, 30, 0)
	b, err = util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), 30)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err = b.Block.HashTreeRoot()
	require.NoError(t, err)

	preState, err = service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err = service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
	roblock, err = consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, true})
	require.NoError(t, err)
	require.Equal(t, root, service.cfg.ForkChoiceStore.CachedHeadRoot())
	sjc = service.CurrentJustifiedCheckpt()
	require.Equal(t, primitives.Epoch(4), sjc.Epoch)
	optimistic, err = service.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, false, optimistic)
}

// See the description in #10777 and #10782 for the full setup
// We sync optimistically a chain of blocks. Block 12 is the first block in Epoch
// 2 (and the merge block in this sequence). Block 18 justifies it and Block 19 returns
// INVALID from NewPayload, with LVH block 12. No head is viable. We check that
// the node can reboot from this state
func TestNoViableHead_Reboot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.SlotsPerEpoch = 6
	config.AltairForkEpoch = 1
	config.BellatrixForkEpoch = 2
	params.OverrideBeaconConfig(config)

	mockEngine := &mockExecution.EngineClient{ErrNewPayload: execution.ErrAcceptedSyncingPayloadStatus, ErrForkchoiceUpdated: execution.ErrAcceptedSyncingPayloadStatus}
	service, tr := minimalTestService(t, WithExecutionEngineCaller(mockEngine))
	ctx := tr.ctx

	genesisState, keys := util.DeterministicGenesisState(t, 64)
	stateRoot, err := genesisState.HashTreeRoot(ctx)
	require.NoError(t, err, "Could not hash genesis state")
	gb := blocks.NewGenesisBlock(stateRoot[:])
	wsb, err := consensusblocks.NewSignedBeaconBlock(gb)
	require.NoError(t, err)
	genesisRoot, err := gb.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb), "Could not save genesis block")
	require.NoError(t, service.saveGenesisData(ctx, genesisState))

	genesis.StoreStateDuringTest(t, genesisState)
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, genesisState, genesisRoot), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, genesisRoot), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveGenesisBlockRoot(ctx, genesisRoot), "Could not save genesis state")

	for i := 1; i < 6; i++ {
		t.Log(i)
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false}))
	}

	for i := 6; i < 12; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlockAltair(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
		require.NoError(t, err)
	}

	// import the merge block
	driftGenesisTime(service, 12, 0)
	st, err := service.HeadState(ctx)
	require.NoError(t, err)
	b, err := util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), 12)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	lastValidRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	preState, err := service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err := service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, lastValidRoot, wsb, postState))
	roblock, err := consensusblocks.NewROBlockWithRoot(wsb, lastValidRoot)
	require.NoError(t, err)
	err = service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false})
	require.NoError(t, err)
	// save the post state and the payload Hash of this block since it will
	// be the LVH
	validHeadState, err := service.HeadState(ctx)
	require.NoError(t, err)
	lvh := b.Block.Body.ExecutionPayload.BlockHash
	validjc := validHeadState.CurrentJustifiedCheckpoint()
	require.Equal(t, primitives.Epoch(0), validjc.Epoch)

	// import blocks 13 through 18 to justify 12
	for i := 13; i < 19; i++ {
		driftGenesisTime(service, primitives.Slot(i), 0)
		st, err := service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		// Save current justified and finalized epochs for future use.
		currStoreJustifiedEpoch := service.CurrentJustifiedCheckpt().Epoch
		currStoreFinalizedEpoch := service.FinalizedCheckpt().Epoch
		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false}))
		require.NoError(t, service.updateJustificationOnBlock(ctx, preState, postState, currStoreJustifiedEpoch))
		_, err = service.updateFinalizationOnBlock(ctx, preState, postState, currStoreFinalizedEpoch)
		require.NoError(t, err)
	}
	// Check that we have justified the second epoch
	jc := service.cfg.ForkChoiceStore.JustifiedCheckpoint()
	require.Equal(t, primitives.Epoch(2), jc.Epoch)

	// import block 19 to find out that the whole chain 13--18 was in fact
	// invalid
	mockEngine = &mockExecution.EngineClient{ErrNewPayload: execution.ErrInvalidPayloadStatus, NewPayloadResp: lvh}
	service.cfg.ExecutionEngineCaller = mockEngine
	driftGenesisTime(service, 19, 0)
	st, err = service.HeadState(ctx)
	require.NoError(t, err)
	b, err = util.GenerateFullBlockBellatrix(st, keys, util.DefaultBlockGenConfig(), 19)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	preState, err = service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	preStateVersion, preStateHeader, err := getStateVersionAndPayload(preState)
	require.NoError(t, err)
	rowsb, err := consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	_, err = service.validateExecutionOnBlock(ctx, preStateVersion, preStateHeader, rowsb)
	require.ErrorContains(t, "received an INVALID payload from execution engine", err)

	// Check that the headroot/state are not in DB and restart the node
	blk, err := service.cfg.BeaconDB.HeadBlock(ctx)
	require.NoError(t, err) // HeadBlock returns no error when headroot == nil
	require.Equal(t, blk, nil)

	service.cfg.ForkChoiceStore = doublylinkedtree.New()
	justified, err := service.cfg.BeaconDB.JustifiedCheckpoint(ctx)
	require.NoError(t, err)

	jroot := bytesutil.ToBytes32(justified.Root)
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, genesisState, jroot))
	service.cfg.ForkChoiceStore.SetBalancesByRooter(service.cfg.StateGen.ActiveNonSlashedBalancesByRoot)
	require.NoError(t, service.StartFromSavedState(genesisState))
	require.NoError(t, service.cfg.BeaconDB.SaveGenesisBlockRoot(ctx, genesisRoot))

	// Forkchoice has the genesisRoot loaded at startup
	require.Equal(t, genesisRoot, service.ensureRootNotZeros(service.cfg.ForkChoiceStore.CachedHeadRoot()))
	// Service's store has the justified checkpoint root as headRoot (verified below through justified checkpoint comparison)
	headRoot, err := service.HeadRoot(ctx)
	require.NoError(t, err)
	require.NotEqual(t, bytesutil.ToBytes32(params.BeaconConfig().ZeroHash[:]), bytesutil.ToBytes32(headRoot)) // Ensure head is not zero
	optimistic, err := service.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, true, optimistic) // Head is now optimistic when starting from justified checkpoint

	// Check that the node's justified checkpoint does not agree with the
	// last valid state's justified checkpoint
	sjc := service.CurrentJustifiedCheckpt()
	require.Equal(t, primitives.Epoch(2), sjc.Epoch)

	// import another block based on the last valid head state
	mockEngine = &mockExecution.EngineClient{}
	service.cfg.ExecutionEngineCaller = mockEngine
	driftGenesisTime(service, 20, 0)
	b, err = util.GenerateFullBlockBellatrix(validHeadState, keys, &util.BlockGenConfig{}, 20)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err = b.Block.HashTreeRoot()
	require.NoError(t, err)
	rwsb, err := consensusblocks.NewROBlock(wsb)
	require.NoError(t, err)
	// We use onBlockBatch here because the valid chain is missing in forkchoice
	require.NoError(t, service.onBlockBatch(ctx, []consensusblocks.ROBlock{rwsb}, &das.MockAvailabilityStore{}))
	// Check that the head is now VALID and the node is not optimistic
	require.Equal(t, genesisRoot, service.ensureRootNotZeros(service.cfg.ForkChoiceStore.CachedHeadRoot()))
	headRoot, err = service.HeadRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, root, bytesutil.ToBytes32(headRoot))

	optimistic, err = service.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, false, optimistic)
}

func TestOnBlock_HandleBlockAttestations(t *testing.T) {
	t.Run("pre-Electra", func(t *testing.T) {
		service, tr := minimalTestService(t)
		ctx := tr.ctx

		st, keys := util.DeterministicGenesisState(t, 64)
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

		st, err = service.HeadState(ctx)
		require.NoError(t, err)
		b, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 1)
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
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false}))

		st, err = service.HeadState(ctx)
		require.NoError(t, err)
		b, err = util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 2)
		require.NoError(t, err)
		wsb, err = consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		// prepare another block that is not inserted
		st3, err := transition.ExecuteStateTransition(ctx, st, wsb)
		require.NoError(t, err)
		b3, err := util.GenerateFullBlock(st3, keys, util.DefaultBlockGenConfig(), 3)
		require.NoError(t, err)
		wsb3, err := consensusblocks.NewSignedBeaconBlock(b3)
		require.NoError(t, err)

		require.Equal(t, 1, len(wsb.Block().Body().Attestations()))
		a := wsb.Block().Body().Attestations()[0]
		r := bytesutil.ToBytes32(a.GetData().BeaconBlockRoot)
		require.Equal(t, true, service.cfg.ForkChoiceStore.HasNode(r))

		require.Equal(t, 1, len(wsb.Block().Body().Attestations()))
		a3 := wsb3.Block().Body().Attestations()[0]
		r3 := bytesutil.ToBytes32(a3.GetData().BeaconBlockRoot)
		require.Equal(t, false, service.cfg.ForkChoiceStore.HasNode(r3))

		require.NoError(t, service.handleBlockAttestations(ctx, wsb.Block(), st)) // fine to use the same committee as st
		require.Equal(t, 0, service.cfg.AttPool.ForkchoiceAttestationCount())
		require.NoError(t, service.handleBlockAttestations(ctx, wsb3.Block(), st3)) // fine to use the same committee as st
		require.Equal(t, 1, len(service.cfg.AttPool.BlockAttestations()))
	})
	t.Run("post-Electra", func(t *testing.T) {
		service, tr := minimalTestService(t)
		ctx := tr.ctx

		st, keys := util.DeterministicGenesisStateElectra(t, 64)
		require.NoError(t, service.saveGenesisData(ctx, st))

		genesis, err := blocks.NewGenesisBlockForState(ctx, st)
		require.NoError(t, err)
		require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, genesis), "Could not save genesis block")
		parentRoot, err := genesis.Block().HashTreeRoot()
		require.NoError(t, err, "Could not get signing root")
		require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st, parentRoot), "Could not save genesis state")
		require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, parentRoot), "Could not save genesis state")

		st, err = service.HeadState(ctx)
		require.NoError(t, err)
		defaultConfig := util.DefaultBlockGenConfig()
		defaultConfig.NumWithdrawalRequests = 1
		defaultConfig.NumDepositRequests = 2
		defaultConfig.NumConsolidationRequests = 1
		b, err := util.GenerateFullBlockElectra(st, keys, defaultConfig, 1)
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		preState, err := service.getBlockPreState(ctx, wsb.Block())
		require.NoError(t, err)
		postState, err := service.validateStateTransition(ctx, preState, wsb)
		require.NoError(t, err)
		require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))
		roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
		require.NoError(t, err)
		require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false}))

		st, err = service.HeadState(ctx)
		require.NoError(t, err)
		b, err = util.GenerateFullBlockElectra(st, keys, defaultConfig, 2)
		require.NoError(t, err)
		wsb, err = consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		// prepare another block that is not inserted
		st3, err := transition.ExecuteStateTransition(ctx, st, wsb)
		require.NoError(t, err)
		b3, err := util.GenerateFullBlockElectra(st3, keys, defaultConfig, 3)
		require.NoError(t, err)
		wsb3, err := consensusblocks.NewSignedBeaconBlock(b3)
		require.NoError(t, err)

		require.Equal(t, 1, len(wsb.Block().Body().Attestations()))
		a := wsb.Block().Body().Attestations()[0]
		r := bytesutil.ToBytes32(a.GetData().BeaconBlockRoot)
		require.Equal(t, true, service.cfg.ForkChoiceStore.HasNode(r))

		require.Equal(t, 1, len(wsb.Block().Body().Attestations()))
		a3 := wsb3.Block().Body().Attestations()[0]
		r3 := bytesutil.ToBytes32(a3.GetData().BeaconBlockRoot)
		require.Equal(t, false, service.cfg.ForkChoiceStore.HasNode(r3))

		require.NoError(t, service.handleBlockAttestations(ctx, wsb.Block(), st)) // fine to use the same committee as st
		require.Equal(t, 0, service.cfg.AttPool.ForkchoiceAttestationCount())
		require.NoError(t, service.handleBlockAttestations(ctx, wsb3.Block(), st3)) // fine to use the same committee as st
		require.Equal(t, 1, len(service.cfg.AttPool.BlockAttestations()))
	})
}

func TestFillMissingBlockPayloadId_DiffSlotExitEarly(t *testing.T) {
	logHook := logTest.NewGlobal()
	service, tr := minimalTestService(t)
	service.SetGenesisTime(time.Now())
	service.lateBlockTasks(tr.ctx)
	require.LogsDoNotContain(t, logHook, "could not perform late block tasks")
}

func TestFillMissingBlockPayloadId_PrepareAllPayloads(t *testing.T) {
	logHook := logTest.NewGlobal()
	resetCfg := features.InitWithReset(&features.Flags{
		PrepareAllPayloads: true,
	})
	defer resetCfg()

	service, tr := minimalTestService(t)
	service.SetGenesisTime(time.Now())
	service.SetForkChoiceGenesisTime(time.Now())
	service.lateBlockTasks(tr.ctx)
	require.LogsDoNotContain(t, logHook, "could not perform late block tasks")
}

// Helper function to simulate the block being on time or delayed for proposer
// boost. It alters the genesisTime tracked by the store.
func driftGenesisTime(s *Service, slot primitives.Slot, delay time.Duration) {
	now := time.Now()
	slotDuration := time.Duration(slot) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	genesis := now.Add(-slotDuration - delay)
	s.SetGenesisTime(genesis)
	s.cfg.ForkChoiceStore.SetGenesisTime(genesis)
}

func TestMissingBlobIndices(t *testing.T) {
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	maxBlobs := params.BeaconConfig().MaxBlobsPerBlock(ds)
	cases := []struct {
		name     string
		expected [][]byte
		present  []uint64
		result   map[uint64]struct{}
		root     [fieldparams.RootLength]byte
		err      error
	}{
		{
			name: "zero len",
		},
		{
			name:     "expected exceeds max",
			expected: fakeCommitments(maxBlobs + 1),
			err:      errMaxBlobsExceeded,
		},
		{
			name:     "first missing",
			expected: fakeCommitments(maxBlobs),
			present:  []uint64{1, 2, 3, 4, 5},
			result:   fakeResult([]uint64{0}),
		},
		{
			name:     "all missing",
			expected: fakeCommitments(maxBlobs),
			result:   fakeResult([]uint64{0, 1, 2, 3, 4, 5}),
		},
		{
			name:     "none missing",
			expected: fakeCommitments(maxBlobs),
			present:  []uint64{0, 1, 2, 3, 4, 5},
			result:   fakeResult([]uint64{}),
		},
		{
			name:     "one commitment, missing",
			expected: fakeCommitments(1),
			present:  []uint64{},
			result:   fakeResult([]uint64{0}),
		},
		{
			name:     "3 commitments, 1 missing",
			expected: fakeCommitments(3),
			present:  []uint64{1},
			result:   fakeResult([]uint64{0, 2}),
		},
		{
			name:     "3 commitments, none missing",
			expected: fakeCommitments(3),
			present:  []uint64{0, 1, 2},
			result:   fakeResult([]uint64{}),
		},
		{
			name:     "3 commitments, all missing",
			expected: fakeCommitments(3),
			present:  []uint64{},
			result:   fakeResult([]uint64{0, 1, 2}),
		},
	}

	for _, c := range cases {
		bm, bs := filesystem.NewEphemeralBlobStorageWithMocker(t)
		t.Run(c.name, func(t *testing.T) {
			require.NoError(t, bm.CreateFakeIndices(c.root, ds, c.present...))
			missing, err := missingBlobIndices(bs, c.root, c.expected, ds)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, len(c.result), len(missing))
			for key := range c.result {
				require.Equal(t, true, missing[key])
			}
		})
	}
}

func TestMissingDataColumnIndices(t *testing.T) {
	countPlusOne := params.BeaconConfig().NumberOfColumns + 1
	tooManyColumns := make(map[uint64]bool, countPlusOne)
	for i := range countPlusOne {
		tooManyColumns[uint64(i)] = true
	}

	testCases := []struct {
		name          string
		storedIndices []uint64
		input         map[uint64]bool
		expected      map[uint64]bool
		err           error
	}{
		{
			name:  "zero len expected",
			input: map[uint64]bool{},
		},
		{
			name:  "expected exceeds max",
			input: tooManyColumns,
			err:   errMaxDataColumnsExceeded,
		},
		{
			name:          "all missing",
			storedIndices: []uint64{},
			input:         map[uint64]bool{0: true, 1: true, 2: true},
			expected:      map[uint64]bool{0: true, 1: true, 2: true},
		},
		{
			name:          "none missing",
			input:         map[uint64]bool{0: true, 1: true, 2: true},
			expected:      map[uint64]bool{},
			storedIndices: []uint64{0, 1, 2, 3, 4}, // Extra columns stored but not expected
		},
		{
			name:          "some missing",
			storedIndices: []uint64{0, 20},
			input:         map[uint64]bool{0: true, 10: true, 20: true, 30: true},
			expected:      map[uint64]bool{10: true, 30: true},
		},
	}

	var emptyRoot [fieldparams.RootLength]byte

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dcm, dcs := filesystem.NewEphemeralDataColumnStorageWithMocker(t)
			err := dcm.CreateFakeIndices(emptyRoot, 0, tc.storedIndices...)
			require.NoError(t, err)

			// Test the function
			actual, err := missingDataColumnIndices(dcs, emptyRoot, tc.input)
			require.ErrorIs(t, err, tc.err)

			require.Equal(t, len(tc.expected), len(actual))
			for key := range tc.expected {
				require.Equal(t, true, actual[key])
			}
		})
	}
}

func Test_getFCUArgs(t *testing.T) {
	s, tr := minimalTestService(t)
	ctx := tr.ctx
	st, keys := util.DeterministicGenesisState(t, 64)
	b, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	roblock, err := consensusblocks.NewROBlockWithRoot(wsb, [32]byte{'a'})
	require.NoError(t, err)
	cfg := &postBlockProcessConfig{
		ctx:            ctx,
		roblock:        roblock,
		postState:      st,
		isValidPayload: true,
	}
	// error branch
	fcuArgs := &fcuConfig{}
	err = s.getFCUArgs(cfg, fcuArgs)
	require.ErrorContains(t, "block does not exist", err)

	// canonical branch
	cfg.headRoot = cfg.roblock.Root()
	fcuArgs = &fcuConfig{}
	err = s.getFCUArgs(cfg, fcuArgs)
	require.NoError(t, err)
	require.Equal(t, cfg.roblock.Root(), fcuArgs.headRoot)
}

func TestRollbackBlock(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	st, keys := util.DeterministicGenesisState(t, 64)
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

	st, err = service.HeadState(ctx)
	require.NoError(t, err)
	b, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 1)
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

	require.Equal(t, true, service.cfg.BeaconDB.HasBlock(ctx, root))
	hasState, err := service.cfg.StateGen.HasState(ctx, root)
	require.NoError(t, err)
	require.Equal(t, true, hasState)

	// Set invalid parent root to trigger forkchoice error.
	wsb.SetParentRoot([]byte("bad"))
	roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)

	// Rollback block insertion into db and caches.
	require.ErrorContains(t, fmt.Sprintf("could not insert block %d to fork choice store", roblock.Block().Slot()), service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false}))

	// The block should no longer exist.
	require.Equal(t, false, service.cfg.BeaconDB.HasBlock(ctx, root))
	hasState, err = service.cfg.StateGen.HasState(ctx, root)
	require.NoError(t, err)
	require.Equal(t, false, hasState)
}

func TestRollbackBlock_SavePostStateInfo_ContextDeadline(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	st, keys := util.DeterministicGenesisState(t, 64)
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

	st, err = service.HeadState(ctx)
	require.NoError(t, err)
	b, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 128)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	preState, err := service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err := service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)

	// Save state summaries so that the cache is flushed and saved to disk
	// later.
	for i := 1; i <= 127; i++ {
		require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{
			Slot: primitives.Slot(i),
			Root: bytesutil.Bytes32(uint64(i)),
		}))
	}

	// Set deadlined context when saving block and state
	cancCtx, canc := context.WithCancel(ctx)
	canc()

	require.ErrorContains(t, context.Canceled.Error(), service.savePostStateInfo(cancCtx, root, wsb, postState))

	// The block should no longer exist.
	require.Equal(t, false, service.cfg.BeaconDB.HasBlock(ctx, root))
	hasState, err := service.cfg.StateGen.HasState(ctx, root)
	require.NoError(t, err)
	require.Equal(t, false, hasState)
}

func TestRollbackBlock_ContextDeadline(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	st, keys := util.DeterministicGenesisState(t, 64)
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

	st, err = service.HeadState(ctx)
	require.NoError(t, err)
	b, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 33)
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
	roblock, err := consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	require.NoError(t, service.postBlockProcess(&postBlockProcessConfig{ctx, roblock, [32]byte{}, postState, false}))

	b, err = util.GenerateFullBlock(postState, keys, util.DefaultBlockGenConfig(), 34)
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err = b.Block.HashTreeRoot()
	require.NoError(t, err)
	preState, err = service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err = service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))

	require.Equal(t, true, service.cfg.BeaconDB.HasBlock(ctx, root))
	hasState, err := service.cfg.StateGen.HasState(ctx, root)
	require.NoError(t, err)
	require.Equal(t, true, hasState)

	// Set deadlined context when processing the block
	cancCtx, canc := context.WithCancel(t.Context())
	canc()
	roblock, err = consensusblocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)

	parentRoot = roblock.Block().ParentRoot()

	cj := &ethpb.Checkpoint{}
	cj.Epoch = 1
	cj.Root = parentRoot[:]
	require.NoError(t, postState.SetCurrentJustifiedCheckpoint(cj))
	require.NoError(t, postState.SetFinalizedCheckpoint(cj))

	// Rollback block insertion into db and caches.
	require.ErrorContains(t, "context canceled", service.postBlockProcess(&postBlockProcessConfig{cancCtx, roblock, [32]byte{}, postState, false}))

	// The block should no longer exist.
	require.Equal(t, false, service.cfg.BeaconDB.HasBlock(ctx, root))
	hasState, err = service.cfg.StateGen.HasState(ctx, root)
	require.NoError(t, err)
	require.Equal(t, false, hasState)
}

func fakeCommitments(n int) [][]byte {
	f := make([][]byte, n)
	for i := range f {
		f[i] = make([]byte, 48)
	}
	return f
}

func fakeResult(missing []uint64) map[uint64]struct{} {
	r := make(map[uint64]struct{}, len(missing))
	for i := range missing {
		r[missing[i]] = struct{}{}
	}
	return r
}

func TestProcessLightClientUpdate(t *testing.T) {
	featCfg := &features.Flags{}
	featCfg.EnableLightClient = true
	reset := features.InitWithReset(featCfg)
	defer reset()

	s, tr := minimalTestService(t, WithLCStore())
	ctx := tr.ctx

	headState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.cfg.BeaconDB.SaveState(ctx, headState, [32]byte{1, 2}))
	require.NoError(t, s.cfg.BeaconDB.SaveHeadBlockRoot(ctx, [32]byte{1, 2}))

	for testVersion := version.Altair; testVersion <= version.Electra; testVersion++ {
		t.Run(version.String(testVersion), func(t *testing.T) {
			l := util.NewTestLightClient(t, testVersion)

			s.genesisTime = time.Unix(time.Now().Unix()-(int64(params.BeaconConfig().VersionToForkEpochMap()[testVersion])*int64(params.BeaconConfig().SlotsPerEpoch)*int64(params.BeaconConfig().SecondsPerSlot)), 0)

			err := s.cfg.BeaconDB.SaveBlock(ctx, l.AttestedBlock)
			require.NoError(t, err)
			attestedBlockRoot, err := l.AttestedBlock.Block().HashTreeRoot()
			require.NoError(t, err)
			err = s.cfg.BeaconDB.SaveState(ctx, l.AttestedState, attestedBlockRoot)
			require.NoError(t, err)

			currentBlockRoot, err := l.Block.Block().HashTreeRoot()
			require.NoError(t, err)
			roblock, err := consensusblocks.NewROBlockWithRoot(l.Block, currentBlockRoot)
			require.NoError(t, err)

			err = s.cfg.BeaconDB.SaveBlock(ctx, roblock)
			require.NoError(t, err)
			err = s.cfg.BeaconDB.SaveState(ctx, l.State, currentBlockRoot)
			require.NoError(t, err)
			err = s.cfg.BeaconDB.SaveHeadBlockRoot(ctx, currentBlockRoot)
			require.NoError(t, err)

			err = s.cfg.BeaconDB.SaveBlock(ctx, l.FinalizedBlock)
			require.NoError(t, err)

			cfg := &postBlockProcessConfig{
				ctx:            ctx,
				roblock:        roblock,
				postState:      l.State,
				isValidPayload: true,
			}

			period := slots.SyncCommitteePeriod(slots.ToEpoch(l.AttestedState.Slot()))

			t.Run("no old update", func(t *testing.T) {
				s.processLightClientUpdates(cfg)
				// Check that the light client update is saved
				u, err := s.lcStore.LightClientUpdate(ctx, period, l.Block)
				require.NoError(t, err)
				require.NotNil(t, u)
				attestedStateRoot, err := l.AttestedState.HashTreeRoot(ctx)
				require.NoError(t, err)
				require.Equal(t, attestedStateRoot, [32]byte(u.AttestedHeader().Beacon().StateRoot))
				require.Equal(t, u.Version(), testVersion)
			})

			t.Run("new update is better", func(t *testing.T) {
				// create and save old update
				oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(l.AttestedBlock)
				require.NoError(t, err)

				err = s.cfg.BeaconDB.SaveLightClientUpdate(ctx, period, oldUpdate)
				require.NoError(t, err)

				s.processLightClientUpdates(cfg)

				u, err := s.lcStore.LightClientUpdate(ctx, period, l.Block)
				require.NoError(t, err)
				require.NotNil(t, u)
				attestedStateRoot, err := l.AttestedState.HashTreeRoot(ctx)
				require.NoError(t, err)
				require.Equal(t, attestedStateRoot, [32]byte(u.AttestedHeader().Beacon().StateRoot))
				require.Equal(t, u.Version(), testVersion)
			})

			t.Run("old update is better", func(t *testing.T) {
				// create and save old update
				oldUpdate, err := lightClient.CreateDefaultLightClientUpdate(l.AttestedBlock)
				require.NoError(t, err)

				// set a better sync aggregate
				scb := make([]byte, 64)
				for i := 0; i < 5; i++ {
					scb[i] = 0x01
				}
				oldUpdate.SetSyncAggregate(&ethpb.SyncAggregate{
					SyncCommitteeBits:      scb,
					SyncCommitteeSignature: make([]byte, 96),
				})

				err = s.cfg.BeaconDB.SaveLightClientUpdate(ctx, period, oldUpdate)
				require.NoError(t, err)

				s.processLightClientUpdates(cfg)

				u, err := s.lcStore.LightClientUpdate(ctx, period, l.Block)
				require.NoError(t, err)
				require.NotNil(t, u)
				require.DeepEqual(t, oldUpdate, u)
				require.Equal(t, u.Version(), testVersion)
			})
		})
	}
}

type testIsAvailableParams struct {
	options                 []Option
	blobKzgCommitmentsCount uint64
	columnsToSave           []uint64
}

func testIsAvailableSetup(t *testing.T, p testIsAvailableParams) (context.Context, context.CancelFunc, *Service, [fieldparams.RootLength]byte, interfaces.SignedBeaconBlock) {
	ctx, cancel := context.WithCancel(t.Context())
	dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)

	options := append(p.options, WithDataColumnStorage(dataColumnStorage))
	service, _ := minimalTestService(t, options...)
	fs := util.SlotAtEpoch(t, params.BeaconConfig().FuluForkEpoch)

	genesisState, secretKeys := util.DeterministicGenesisStateElectra(t, 32, util.WithElectraStateSlot(fs))
	require.NoError(t, service.saveGenesisData(ctx, genesisState))

	conf := util.DefaultBlockGenConfig()
	conf.NumBlobKzgCommitments = p.blobKzgCommitmentsCount

	signedBeaconBlock, err := util.GenerateFullBlockFulu(genesisState, secretKeys, conf, fs+1)
	require.NoError(t, err)

	block := signedBeaconBlock.Block
	bodyRoot, err := block.Body.HashTreeRoot()
	require.NoError(t, err)

	root, err := block.HashTreeRoot()
	require.NoError(t, err)

	dataColumnsParams := make([]util.DataColumnParam, 0, len(p.columnsToSave))
	for _, i := range p.columnsToSave {
		dataColumnParam := util.DataColumnParam{
			Index:         i,
			Slot:          block.Slot,
			ProposerIndex: block.ProposerIndex,
			ParentRoot:    block.ParentRoot,
			StateRoot:     block.StateRoot,
			BodyRoot:      bodyRoot[:],
		}
		dataColumnsParams = append(dataColumnsParams, dataColumnParam)
	}

	_, verifiedRODataColumns := util.CreateTestVerifiedRoDataColumnSidecars(t, dataColumnsParams)

	err = dataColumnStorage.Save(verifiedRODataColumns)
	require.NoError(t, err)

	signed, err := consensusblocks.NewSignedBeaconBlock(signedBeaconBlock)
	require.NoError(t, err)

	return ctx, cancel, service, root, signed
}

func TestIsDataAvailable(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.AltairForkEpoch, cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch, cfg.DenebForkEpoch, cfg.ElectraForkEpoch, cfg.FuluForkEpoch = 0, 0, 0, 0, 0, 0
	params.OverrideBeaconConfig(cfg)
	t.Run("Fulu - out of retention window", func(t *testing.T) {
		params := testIsAvailableParams{}
		ctx, _, service, root, signed := testIsAvailableSetup(t, params)

		roBlock, err := consensusblocks.NewROBlockWithRoot(signed, root)
		require.NoError(t, err)
		err = service.isDataAvailable(ctx, roBlock)
		require.NoError(t, err)
	})

	t.Run("Fulu - no commitment in blocks", func(t *testing.T) {
		ctx, _, service, root, signed := testIsAvailableSetup(t, testIsAvailableParams{})

		roBlock, err := consensusblocks.NewROBlockWithRoot(signed, root)
		require.NoError(t, err)
		err = service.isDataAvailable(ctx, roBlock)
		require.NoError(t, err)
	})
	t.Run("Fulu - more than half of the columns in custody", func(t *testing.T) {
		minimumColumnsCountToReconstruct := peerdas.MinimumColumnCountToReconstruct()
		indices := make([]uint64, 0, minimumColumnsCountToReconstruct)
		for i := range minimumColumnsCountToReconstruct {
			indices = append(indices, i)
		}

		params := testIsAvailableParams{
			columnsToSave:           indices,
			blobKzgCommitmentsCount: 3,
		}

		ctx, _, service, root, signed := testIsAvailableSetup(t, params)

		roBlock, err := consensusblocks.NewROBlockWithRoot(signed, root)
		require.NoError(t, err)
		err = service.isDataAvailable(ctx, roBlock)
		require.NoError(t, err)
	})

	t.Run("Fulu - no missing data columns", func(t *testing.T) {
		params := testIsAvailableParams{
			columnsToSave:           []uint64{1, 17, 19, 42, 75, 87, 102, 117, 119}, // 119 is not needed
			blobKzgCommitmentsCount: 3,
		}

		ctx, _, service, root, signed := testIsAvailableSetup(t, params)

		roBlock, err := consensusblocks.NewROBlockWithRoot(signed, root)
		require.NoError(t, err)
		err = service.isDataAvailable(ctx, roBlock)
		require.NoError(t, err)
	})

	t.Run("Fulu - some initially missing data columns (no reconstruction)", func(t *testing.T) {
		startWaiting := make(chan bool)

		testParams := testIsAvailableParams{
			options:       []Option{WithStartWaitingDataColumnSidecars(startWaiting)},
			columnsToSave: []uint64{1, 17, 19, 75, 102, 117, 119}, // 119 is not needed, 42 and 87 are missing

			blobKzgCommitmentsCount: 3,
		}

		ctx, _, service, root, signed := testIsAvailableSetup(t, testParams)
		block := signed.Block()
		slot := block.Slot()
		proposerIndex := block.ProposerIndex()
		parentRoot := block.ParentRoot()
		stateRoot := block.StateRoot()
		bodyRoot, err := block.Body().HashTreeRoot()
		require.NoError(t, err)

		_, verifiedSidecarsWrongRoot := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Index: 42, Slot: slot + 1}, // Needed index, but not for this slot.
			})

		_, verifiedSidecars := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{
			{Index: 87, Slot: slot, ProposerIndex: proposerIndex, ParentRoot: parentRoot[:], StateRoot: stateRoot[:], BodyRoot: bodyRoot[:]}, // Needed index
			{Index: 1, Slot: slot, ProposerIndex: proposerIndex, ParentRoot: parentRoot[:], StateRoot: stateRoot[:], BodyRoot: bodyRoot[:]},  // Not needed index
			{Index: 42, Slot: slot, ProposerIndex: proposerIndex, ParentRoot: parentRoot[:], StateRoot: stateRoot[:], BodyRoot: bodyRoot[:]}, // Needed index
		})

		go func() {
			<-startWaiting

			err := service.dataColumnStorage.Save(verifiedSidecarsWrongRoot)
			require.NoError(t, err)

			err = service.dataColumnStorage.Save(verifiedSidecars)
			require.NoError(t, err)
		}()

		ctx, cancel := context.WithTimeout(ctx, time.Second*2)
		defer cancel()

		roBlock, err := consensusblocks.NewROBlockWithRoot(signed, root)
		require.NoError(t, err)
		err = service.isDataAvailable(ctx, roBlock)
		require.NoError(t, err)
	})

	t.Run("Fulu - some initially missing data columns (reconstruction)", func(t *testing.T) {
		const (
			missingColumns = uint64(2)
			cgc            = 128
		)

		startWaiting := make(chan bool)

		minimumColumnsCountToReconstruct := peerdas.MinimumColumnCountToReconstruct()
		indices := make([]uint64, 0, minimumColumnsCountToReconstruct-missingColumns)

		for i := range minimumColumnsCountToReconstruct - missingColumns {
			indices = append(indices, i)
		}

		testParams := testIsAvailableParams{
			options:                 []Option{WithStartWaitingDataColumnSidecars(startWaiting)},
			columnsToSave:           indices,
			blobKzgCommitmentsCount: 3,
		}

		ctx, _, service, root, signed := testIsAvailableSetup(t, testParams)
		_, _, err := service.cfg.P2P.UpdateCustodyInfo(0, cgc)
		require.NoError(t, err)
		block := signed.Block()
		slot := block.Slot()
		proposerIndex := block.ProposerIndex()
		parentRoot := block.ParentRoot()
		stateRoot := block.StateRoot()
		bodyRoot, err := block.Body().HashTreeRoot()
		require.NoError(t, err)

		dataColumnParams := make([]util.DataColumnParam, 0, missingColumns)
		for i := minimumColumnsCountToReconstruct - missingColumns; i < minimumColumnsCountToReconstruct; i++ {
			dataColumnParam := util.DataColumnParam{
				Index:         i,
				Slot:          slot,
				ProposerIndex: proposerIndex,
				ParentRoot:    parentRoot[:],
				StateRoot:     stateRoot[:],
				BodyRoot:      bodyRoot[:],
			}

			dataColumnParams = append(dataColumnParams, dataColumnParam)
		}

		_, verifiedSidecars := util.CreateTestVerifiedRoDataColumnSidecars(t, dataColumnParams)

		go func() {
			<-startWaiting

			err := service.dataColumnStorage.Save(verifiedSidecars)
			require.NoError(t, err)
		}()

		ctx, cancel := context.WithTimeout(ctx, time.Second*2)
		defer cancel()

		roBlock, err := consensusblocks.NewROBlockWithRoot(signed, root)
		require.NoError(t, err)
		err = service.isDataAvailable(ctx, roBlock)
		require.NoError(t, err)
	})

	t.Run("Fulu - some columns are definitively missing", func(t *testing.T) {
		startWaiting := make(chan bool)

		params := testIsAvailableParams{
			options:                 []Option{WithStartWaitingDataColumnSidecars(startWaiting)},
			blobKzgCommitmentsCount: 3,
		}

		ctx, cancel, service, root, signed := testIsAvailableSetup(t, params)

		go func() {
			<-startWaiting
			cancel()
		}()

		roBlock, err := consensusblocks.NewROBlockWithRoot(signed, root)
		require.NoError(t, err)
		err = service.isDataAvailable(ctx, roBlock)
		require.NotNil(t, err)
	})
}

// Test_postBlockProcess_EventSending tests that block processed events are only sent
// when block processing succeeds according to the decision tree:
//
// Block Processing Flow:
// ├─ InsertNode FAILS (fork choice timeout)
// │  └─ blockProcessed = false ❌ NO EVENT
// │
// ├─ InsertNode succeeds
// │  ├─ handleBlockAttestations FAILS
// │  │  └─ blockProcessed = false ❌ NO EVENT
// │  │
// │  ├─ Block is NON-CANONICAL (not head)
// │  │  └─ blockProcessed = true ✅ SEND EVENT (Line 111)
// │  │
// │  ├─ Block IS CANONICAL (new head)
// │  │  ├─ getFCUArgs FAILS
// │  │  │  └─ blockProcessed = true ✅ SEND EVENT (Line 117)
// │  │  │
// │  │  ├─ sendFCU FAILS
// │  │  │  └─ blockProcessed = false ❌ NO EVENT
// │  │  │
// │  │  └─ Full success
// │  │     └─ blockProcessed = true ✅ SEND EVENT (Line 125)
func Test_postBlockProcess_EventSending(t *testing.T) {
	ctx := context.Background()

	// Helper to create a minimal valid block and state
	createTestBlockAndState := func(t *testing.T, slot primitives.Slot, parentRoot [32]byte) (consensusblocks.ROBlock, state.BeaconState) {
		st, _ := util.DeterministicGenesisState(t, 64)
		require.NoError(t, st.SetSlot(slot))

		stateRoot, err := st.HashTreeRoot(ctx)
		require.NoError(t, err)

		blk := util.NewBeaconBlock()
		blk.Block.Slot = slot
		blk.Block.ProposerIndex = 0
		blk.Block.ParentRoot = parentRoot[:]
		blk.Block.StateRoot = stateRoot[:]

		signed := util.HydrateSignedBeaconBlock(blk)
		roBlock, err := consensusblocks.NewSignedBeaconBlock(signed)
		require.NoError(t, err)

		roBlk, err := consensusblocks.NewROBlock(roBlock)
		require.NoError(t, err)
		return roBlk, st
	}

	tests := []struct {
		name          string
		setupService  func(*Service, [32]byte)
		expectEvent   bool
		expectError   bool
		errorContains string
	}{
		{
			name: "Block successfully processed - sends event",
			setupService: func(s *Service, blockRoot [32]byte) {
				// Default setup should work
			},
			expectEvent: true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create service with required options
			opts := testServiceOptsWithDB(t)
			service, err := NewService(ctx, opts...)
			require.NoError(t, err)

			// Initialize fork choice with genesis block
			st, _ := util.DeterministicGenesisState(t, 64)
			require.NoError(t, st.SetSlot(0))
			genesisBlock := util.NewBeaconBlock()
			genesisBlock.Block.StateRoot = bytesutil.PadTo([]byte("genesisState"), 32)
			signedGenesis := util.HydrateSignedBeaconBlock(genesisBlock)
			block, err := consensusblocks.NewSignedBeaconBlock(signedGenesis)
			require.NoError(t, err)
			genesisRoot, err := block.Block().HashTreeRoot()
			require.NoError(t, err)
			require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, block))
			require.NoError(t, service.cfg.BeaconDB.SaveGenesisBlockRoot(ctx, genesisRoot))
			require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st, genesisRoot))

			genesisROBlock, err := consensusblocks.NewROBlock(block)
			require.NoError(t, err)
			require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, genesisROBlock))

			// Create test block and state with genesis as parent
			roBlock, postSt := createTestBlockAndState(t, 100, genesisRoot)

			// Apply additional service setup if provided
			if tt.setupService != nil {
				tt.setupService(service, roBlock.Root())
			}

			// Create post block process config
			cfg := &postBlockProcessConfig{
				ctx:            ctx,
				roblock:        roBlock,
				postState:      postSt,
				isValidPayload: true,
			}

			// Execute postBlockProcess
			err = service.postBlockProcess(cfg)

			// Check error expectation
			if tt.expectError {
				require.NotNil(t, err)
				if tt.errorContains != "" {
					require.ErrorContains(t, tt.errorContains, err)
				}
			} else {
				require.NoError(t, err)
			}

			// Give a moment for deferred functions to execute
			time.Sleep(10 * time.Millisecond)

			// Check event expectation
			notifier := service.cfg.StateNotifier.(*mock.MockStateNotifier)
			events := notifier.ReceivedEvents()

			if tt.expectEvent {
				require.NotEqual(t, 0, len(events), "Expected event to be sent but none were received")

				// Verify it's a BlockProcessed event
				foundBlockProcessed := false
				for _, evt := range events {
					if evt.Type == statefeed.BlockProcessed {
						foundBlockProcessed = true
						data, ok := evt.Data.(*statefeed.BlockProcessedData)
						require.Equal(t, true, ok, "Event data should be BlockProcessedData")
						require.Equal(t, roBlock.Root(), data.BlockRoot, "Event should contain correct block root")
						break
					}
				}
				require.Equal(t, true, foundBlockProcessed, "Expected BlockProcessed event type")
			} else {
				// For no-event cases, verify no BlockProcessed events were sent
				for _, evt := range events {
					require.NotEqual(t, statefeed.BlockProcessed, evt.Type,
						"Expected no BlockProcessed event but one was sent")
				}
			}
		})
	}
}

func setupLightClientTestRequirements(ctx context.Context, t *testing.T, s *Service, v int, options ...util.LightClientOption) (*util.TestLightClient, *postBlockProcessConfig) {
	var l *util.TestLightClient
	switch v {
	case version.Altair:
		l = util.NewTestLightClient(t, version.Altair, options...)
	case version.Bellatrix:
		l = util.NewTestLightClient(t, version.Bellatrix, options...)
	case version.Capella:
		l = util.NewTestLightClient(t, version.Capella, options...)
	case version.Deneb:
		l = util.NewTestLightClient(t, version.Deneb, options...)
	case version.Electra:
		l = util.NewTestLightClient(t, version.Electra, options...)
	default:
		t.Errorf("Unsupported fork version %s", version.String(v))
		return nil, nil
	}

	err := s.cfg.BeaconDB.SaveBlock(ctx, l.AttestedBlock)
	require.NoError(t, err)
	attestedBlockRoot, err := l.AttestedBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	err = s.cfg.BeaconDB.SaveState(ctx, l.AttestedState, attestedBlockRoot)
	require.NoError(t, err)

	currentBlockRoot, err := l.Block.Block().HashTreeRoot()
	require.NoError(t, err)
	roblock, err := consensusblocks.NewROBlockWithRoot(l.Block, currentBlockRoot)
	require.NoError(t, err)

	err = s.cfg.BeaconDB.SaveBlock(ctx, roblock)
	require.NoError(t, err)
	err = s.cfg.BeaconDB.SaveState(ctx, l.State, currentBlockRoot)
	require.NoError(t, err)

	err = s.cfg.BeaconDB.SaveBlock(ctx, l.FinalizedBlock)
	require.NoError(t, err)

	cfg := &postBlockProcessConfig{
		ctx:            ctx,
		roblock:        roblock,
		postState:      l.State,
		isValidPayload: true,
	}

	return l, cfg
}

func TestProcessLightClientOptimisticUpdate(t *testing.T) {
	featCfg := &features.Flags{}
	featCfg.EnableLightClient = true
	reset := features.InitWithReset(featCfg)
	defer reset()

	params.SetupTestConfigCleanup(t)
	beaconCfg := params.BeaconConfig()
	beaconCfg.AltairForkEpoch = 1
	beaconCfg.BellatrixForkEpoch = 2
	beaconCfg.CapellaForkEpoch = 3
	beaconCfg.DenebForkEpoch = 4
	beaconCfg.ElectraForkEpoch = 5
	params.OverrideBeaconConfig(beaconCfg)

	s, tr := minimalTestService(t)
	s.cfg.P2P = &mockp2p.FakeP2P{}
	ctx := tr.ctx

	headState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.cfg.BeaconDB.SaveState(ctx, headState, [32]byte{1, 2}))
	require.NoError(t, s.cfg.BeaconDB.SaveHeadBlockRoot(ctx, [32]byte{1, 2}))

	testCases := []struct {
		name          string
		oldOptions    []util.LightClientOption
		newOptions    []util.LightClientOption
		expectReplace bool
	}{
		{
			name:          "No old update",
			oldOptions:    nil,
			newOptions:    []util.LightClientOption{},
			expectReplace: true,
		},
		{
			name:          "Same age",
			oldOptions:    []util.LightClientOption{},
			newOptions:    []util.LightClientOption{util.WithSupermajority(0)}, // supermajority does not matter here and is only added to result in two different updates
			expectReplace: false,
		},
		{
			name:          "Old update is better - age",
			oldOptions:    []util.LightClientOption{util.WithIncreasedAttestedSlot(1)},
			newOptions:    []util.LightClientOption{},
			expectReplace: false,
		},
		{
			name:          "New update is better - age",
			oldOptions:    []util.LightClientOption{},
			newOptions:    []util.LightClientOption{util.WithIncreasedAttestedSlot(1)},
			expectReplace: true,
		},
	}

	for _, tc := range testCases {
		for testVersion := 1; testVersion < 6; testVersion++ { // test all forks
			var forkEpoch uint64
			var expectedVersion int

			switch testVersion {
			case 1:
				forkEpoch = uint64(params.BeaconConfig().AltairForkEpoch)
				expectedVersion = version.Altair
			case 2:
				forkEpoch = uint64(params.BeaconConfig().BellatrixForkEpoch)
				expectedVersion = version.Bellatrix
			case 3:
				forkEpoch = uint64(params.BeaconConfig().CapellaForkEpoch)
				expectedVersion = version.Capella
			case 4:
				forkEpoch = uint64(params.BeaconConfig().DenebForkEpoch)
				expectedVersion = version.Deneb
			case 5:
				forkEpoch = uint64(params.BeaconConfig().ElectraForkEpoch)
				expectedVersion = version.Electra
			default:
				t.Errorf("Unsupported fork version %s", version.String(testVersion))
			}

			t.Run(version.String(testVersion)+"_"+tc.name, func(t *testing.T) {
				s.genesisTime = time.Unix(time.Now().Unix()-(int64(forkEpoch)*int64(params.BeaconConfig().SlotsPerEpoch)*int64(params.BeaconConfig().SecondsPerSlot)), 0)
				s.lcStore = lightClient.NewLightClientStore(s.cfg.P2P, s.cfg.StateNotifier.StateFeed(), s.cfg.BeaconDB)

				var oldActualUpdate interfaces.LightClientOptimisticUpdate
				var err error
				if tc.oldOptions != nil {
					// config for old update
					lOld, cfgOld := setupLightClientTestRequirements(ctx, t, s, testVersion, tc.oldOptions...)
					s.processLightClientUpdates(cfgOld)

					oldActualUpdate, err = lightClient.NewLightClientOptimisticUpdateFromBeaconState(lOld.Ctx, lOld.State, lOld.Block, lOld.AttestedState, lOld.AttestedBlock)
					require.NoError(t, err)

					// check that the old update is saved
					oldUpdate := s.lcStore.LastOptimisticUpdate()
					require.NotNil(t, oldUpdate)

					require.DeepEqual(t, oldUpdate, oldActualUpdate, "old update should be saved")
				}

				// config for new update
				lNew, cfgNew := setupLightClientTestRequirements(ctx, t, s, testVersion, tc.newOptions...)
				s.processLightClientUpdates(cfgNew)

				newActualUpdate, err := lightClient.NewLightClientOptimisticUpdateFromBeaconState(lNew.Ctx, lNew.State, lNew.Block, lNew.AttestedState, lNew.AttestedBlock)
				require.NoError(t, err)

				require.DeepNotEqual(t, newActualUpdate, oldActualUpdate, "new update should not be equal to old update")

				// check that the new update is saved or skipped
				newUpdate := s.lcStore.LastOptimisticUpdate()
				require.NotNil(t, newUpdate)

				if tc.expectReplace {
					require.DeepEqual(t, newActualUpdate, newUpdate)
					require.Equal(t, expectedVersion, newUpdate.Version())
				} else {
					require.DeepEqual(t, oldActualUpdate, newUpdate)
					require.Equal(t, expectedVersion, newUpdate.Version())
				}
			})
		}
	}
}

func TestProcessLightClientFinalityUpdate(t *testing.T) {
	featCfg := &features.Flags{}
	featCfg.EnableLightClient = true
	reset := features.InitWithReset(featCfg)
	defer reset()

	params.SetupTestConfigCleanup(t)
	beaconCfg := params.BeaconConfig()
	beaconCfg.AltairForkEpoch = 1
	beaconCfg.BellatrixForkEpoch = 2
	beaconCfg.CapellaForkEpoch = 3
	beaconCfg.DenebForkEpoch = 4
	beaconCfg.ElectraForkEpoch = 5
	params.OverrideBeaconConfig(beaconCfg)

	s, tr := minimalTestService(t)
	s.cfg.P2P = &mockp2p.FakeP2P{}
	ctx := tr.ctx
	s.head = &head{}

	testCases := []struct {
		name          string
		oldOptions    []util.LightClientOption
		newOptions    []util.LightClientOption
		expectReplace bool
	}{
		{
			name:          "No old update",
			oldOptions:    nil,
			newOptions:    []util.LightClientOption{},
			expectReplace: true,
		},
		{
			name:          "Old update is better - finalized slot is higher",
			oldOptions:    []util.LightClientOption{util.WithIncreasedFinalizedSlot(1)},
			newOptions:    []util.LightClientOption{},
			expectReplace: false,
		},
		{
			name:          "Old update is better - attested slot is higher",
			oldOptions:    []util.LightClientOption{util.WithIncreasedAttestedSlot(1)},
			newOptions:    []util.LightClientOption{},
			expectReplace: false,
		},
		{
			name:          "Old update is better - signature slot is higher",
			oldOptions:    []util.LightClientOption{util.WithIncreasedSignatureSlot(1)},
			newOptions:    []util.LightClientOption{},
			expectReplace: false,
		},
		{
			name:          "New update is better - finalized slot is higher",
			oldOptions:    []util.LightClientOption{},
			newOptions:    []util.LightClientOption{util.WithIncreasedAttestedSlot(1)},
			expectReplace: true,
		},
		{
			name:          "New update is better - attested slot is higher",
			oldOptions:    []util.LightClientOption{},
			newOptions:    []util.LightClientOption{util.WithIncreasedAttestedSlot(1)},
			expectReplace: true,
		},
		{
			name:          "New update is better - signature slot is higher",
			oldOptions:    []util.LightClientOption{},
			newOptions:    []util.LightClientOption{util.WithIncreasedSignatureSlot(1)},
			expectReplace: true,
		},
	}

	for _, tc := range testCases {
		for testVersion := 1; testVersion < 6; testVersion++ { // test all forks
			var forkEpoch uint64
			var expectedVersion int

			switch testVersion {
			case 1:
				forkEpoch = uint64(params.BeaconConfig().AltairForkEpoch)
				expectedVersion = version.Altair
			case 2:
				forkEpoch = uint64(params.BeaconConfig().BellatrixForkEpoch)
				expectedVersion = version.Bellatrix
			case 3:
				forkEpoch = uint64(params.BeaconConfig().CapellaForkEpoch)
				expectedVersion = version.Capella
			case 4:
				forkEpoch = uint64(params.BeaconConfig().DenebForkEpoch)
				expectedVersion = version.Deneb
			case 5:
				forkEpoch = uint64(params.BeaconConfig().ElectraForkEpoch)
				expectedVersion = version.Electra
			default:
				t.Errorf("Unsupported fork version %s", version.String(testVersion))
			}

			t.Run(version.String(testVersion)+"_"+tc.name, func(t *testing.T) {
				s.genesisTime = time.Unix(time.Now().Unix()-(int64(forkEpoch)*int64(params.BeaconConfig().SlotsPerEpoch)*int64(params.BeaconConfig().SecondsPerSlot)), 0)
				s.lcStore = lightClient.NewLightClientStore(s.cfg.P2P, s.cfg.StateNotifier.StateFeed(), s.cfg.BeaconDB)

				var actualOldUpdate, actualNewUpdate interfaces.LightClientFinalityUpdate

				if tc.oldOptions != nil {
					// config for old update
					lOld, cfgOld := setupLightClientTestRequirements(ctx, t, s, testVersion, tc.oldOptions...)
					blkRoot, err := lOld.Block.Block().HashTreeRoot()
					require.NoError(t, err)
					s.head.block = lOld.Block
					s.head.root = blkRoot
					s.processLightClientUpdates(cfgOld)

					// check that the old update is saved
					actualOldUpdate, err = lightClient.NewLightClientFinalityUpdateFromBeaconState(ctx, cfgOld.postState, cfgOld.roblock, lOld.AttestedState, lOld.AttestedBlock, lOld.FinalizedBlock)
					require.NoError(t, err)
					oldUpdate := s.lcStore.LastFinalityUpdate()
					require.DeepEqual(t, actualOldUpdate, oldUpdate)
				}

				// config for new update
				lNew, cfgNew := setupLightClientTestRequirements(ctx, t, s, testVersion, tc.newOptions...)
				blkRoot, err := lNew.Block.Block().HashTreeRoot()
				require.NoError(t, err)
				s.head.block = lNew.Block
				s.head.root = blkRoot
				s.processLightClientUpdates(cfgNew)

				// check that the actual old update and the actual new update are different
				actualNewUpdate, err = lightClient.NewLightClientFinalityUpdateFromBeaconState(ctx, cfgNew.postState, cfgNew.roblock, lNew.AttestedState, lNew.AttestedBlock, lNew.FinalizedBlock)
				require.NoError(t, err)
				require.DeepNotEqual(t, actualOldUpdate, actualNewUpdate)

				// check that the new update is saved or skipped
				newUpdate := s.lcStore.LastFinalityUpdate()

				if tc.expectReplace {
					require.DeepEqual(t, actualNewUpdate, newUpdate)
					require.Equal(t, expectedVersion, newUpdate.Version())
				} else {
					require.DeepEqual(t, actualOldUpdate, newUpdate)
					require.Equal(t, expectedVersion, newUpdate.Version())
				}
			})
		}
	}
}
