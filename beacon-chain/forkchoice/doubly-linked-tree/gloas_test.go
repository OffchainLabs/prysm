package doublylinkedtree

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func setupGloas(t *testing.T, justified, finalized primitives.Epoch) *ForkChoice {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	return setup(justified, finalized)
}

func prepareGloasForkchoiceState(
	_ context.Context,
	slot primitives.Slot,
	blockRoot [32]byte,
	parentRoot [32]byte,
	blockHash [32]byte,
	parentBlockHash [32]byte,
	justifiedEpoch primitives.Epoch,
	finalizedEpoch primitives.Epoch,
) (state.BeaconState, blocks.ROBlock, error) {
	blockHeader := &ethpb.BeaconBlockHeader{
		ParentRoot: parentRoot[:],
	}

	justifiedCheckpoint := &ethpb.Checkpoint{
		Epoch: justifiedEpoch,
	}

	finalizedCheckpoint := &ethpb.Checkpoint{
		Epoch: finalizedEpoch,
	}

	builderPendingPayments := make([]*ethpb.BuilderPendingPayment, 64)
	for i := range builderPendingPayments {
		builderPendingPayments[i] = &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, 20),
			},
		}
	}

	base := &ethpb.BeaconStateGloas{
		Slot:                       slot,
		RandaoMixes:                make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		CurrentJustifiedCheckpoint: justifiedCheckpoint,
		FinalizedCheckpoint:        finalizedCheckpoint,
		LatestBlockHeader:          blockHeader,
		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
			BlockHash:          blockHash[:],
			ParentBlockHash:    parentBlockHash[:],
			ParentBlockRoot:    make([]byte, 32),
			PrevRandao:         make([]byte, 32),
			FeeRecipient:       make([]byte, 20),
			BlobKzgCommitments: [][]byte{make([]byte, 48)},
		},
		Builders:                     make([]*ethpb.Builder, 0),
		BuilderPendingPayments:       builderPendingPayments,
		ExecutionPayloadAvailability: make([]byte, 1024),
		LatestBlockHash:              make([]byte, 32),
		PayloadExpectedWithdrawals:   make([]*enginev1.Withdrawal, 0),
		ProposerLookahead:            make([]uint64, 64),
	}

	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	if err != nil {
		return nil, blocks.ROBlock{}, err
	}

	bid := util.HydrateSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BlockHash:       blockHash[:],
			ParentBlockHash: parentBlockHash[:],
		},
	})

	blk := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body: &ethpb.BeaconBlockBodyGloas{
				SignedExecutionPayloadBid: bid,
			},
		},
	})

	signed, err := blocks.NewSignedBeaconBlock(blk)
	if err != nil {
		return nil, blocks.ROBlock{}, err
	}
	roblock, err := blocks.NewROBlockWithRoot(signed, blockRoot)
	return st, roblock, err
}

func prepareGloasForkchoicePayload(
	blockRoot [32]byte,
) (interfaces.ROExecutionPayloadEnvelope, error) {
	env := &ethpb.ExecutionPayloadEnvelope{
		BeaconBlockRoot: blockRoot[:],
		Payload:         &enginev1.ExecutionPayloadDeneb{},
	}
	return blocks.WrappedROExecutionPayloadEnvelope(env)
}

func TestInsertGloasBlock_EmptyNodeOnly(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()

	root := indexToHash(1)
	blockHash := indexToHash(100)
	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, root, params.BeaconConfig().ZeroHash, blockHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	// Empty node should exist.
	en := f.store.emptyNodeByRoot[root]
	require.NotNil(t, en)

	// Full node should NOT exist.
	_, hasFull := f.store.fullNodeByRoot[root]
	assert.Equal(t, false, hasFull)

	// Parent should be the genesis full node.
	genesisRoot := params.BeaconConfig().ZeroHash
	genesisFull := f.store.fullNodeByRoot[genesisRoot]
	require.NotNil(t, genesisFull)
	assert.Equal(t, genesisFull, en.node.parent)
}

func TestInsertPayload_CreatesFullNode(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()

	root := indexToHash(1)
	blockHash := indexToHash(100)
	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, root, params.BeaconConfig().ZeroHash, blockHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))
	require.Equal(t, 2, len(f.store.emptyNodeByRoot))
	require.Equal(t, 1, len(f.store.fullNodeByRoot))

	pe, err := prepareGloasForkchoicePayload(root)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))
	require.Equal(t, 2, len(f.store.fullNodeByRoot))

	fn := f.store.fullNodeByRoot[root]
	require.NotNil(t, fn)

	en := f.store.emptyNodeByRoot[root]
	require.NotNil(t, en)

	// Empty and full share the same *Node.
	assert.Equal(t, en.node, fn.node)
	assert.Equal(t, true, fn.optimistic)
	assert.Equal(t, true, fn.full)
}

func TestInsertPayload_DuplicateIsNoop(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()

	root := indexToHash(1)
	blockHash := indexToHash(100)
	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, root, params.BeaconConfig().ZeroHash, blockHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	pe, err := prepareGloasForkchoicePayload(root)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))
	require.Equal(t, 2, len(f.store.fullNodeByRoot))

	fn := f.store.fullNodeByRoot[root]
	require.NotNil(t, fn)

	// Insert again — should be a no-op.
	require.NoError(t, f.InsertPayload(pe))
	assert.Equal(t, fn, f.store.fullNodeByRoot[root])
	require.Equal(t, 2, len(f.store.fullNodeByRoot))
}

func TestInsertPayload_WithoutEmptyNode_Errors(t *testing.T) {
	f := setupGloas(t, 0, 0)

	root := indexToHash(99)
	pe, err := prepareGloasForkchoicePayload(root)
	require.NoError(t, err)

	err = f.InsertPayload(pe)
	require.ErrorContains(t, ErrNilNode.Error(), err)
}

func TestGloasBlock_ChildBuildsOnEmpty(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()

	// Insert Gloas block A (empty only).
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, rootA, params.BeaconConfig().ZeroHash, blockHashA, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	// Insert Gloas block B as child of (A, empty)
	rootB := indexToHash(2)
	blockHashB := indexToHash(200)
	nonMatchingParentHash := indexToHash(999)
	st, roblock, err = prepareGloasForkchoiceState(ctx, 2, rootB, rootA, blockHashB, nonMatchingParentHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	emptyA := f.store.emptyNodeByRoot[rootA]
	require.NotNil(t, emptyA)
	nodeB := f.store.emptyNodeByRoot[rootB]
	require.NotNil(t, nodeB)
	require.Equal(t, emptyA, nodeB.node.parent)
}

func TestGloasBlock_ChildrenOfEmptyAndFull(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()

	// Insert Gloas block A (empty only).
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, rootA, params.BeaconConfig().ZeroHash, blockHashA, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))
	// Insert payload for A
	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	// Insert Gloas block B as child of (A, empty)
	rootB := indexToHash(2)
	blockHashB := indexToHash(200)
	nonMatchingParentHash := indexToHash(999)
	st, roblock, err = prepareGloasForkchoiceState(ctx, 2, rootB, rootA, blockHashB, nonMatchingParentHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	// Insert Gloas block C as child of (A, full)
	rootC := indexToHash(3)
	blockHashC := indexToHash(201)
	st, roblock, err = prepareGloasForkchoiceState(ctx, 3, rootC, rootA, blockHashC, blockHashA, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	emptyA := f.store.emptyNodeByRoot[rootA]
	require.NotNil(t, emptyA)
	nodeB := f.store.emptyNodeByRoot[rootB]
	require.NotNil(t, nodeB)
	require.Equal(t, emptyA, nodeB.node.parent)
	nodeC := f.store.emptyNodeByRoot[rootC]
	require.NotNil(t, nodeC)
	fullA := f.store.fullNodeByRoot[rootA]
	require.NotNil(t, fullA)
	require.Equal(t, fullA, nodeC.node.parent)
}

func TestBlockHash_ReturnsBlockHash(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()

	root := indexToHash(1)
	blockHash := indexToHash(100)
	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, root, params.BeaconConfig().ZeroHash, blockHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	got, err := f.BlockHash(root)
	require.NoError(t, err)
	assert.Equal(t, blockHash, got)
}

func TestBlockHash_UnknownRoot(t *testing.T) {
	f := setupGloas(t, 0, 0)

	unknownRoot := indexToHash(999)
	_, err := f.BlockHash(unknownRoot)
	require.ErrorContains(t, ErrNilNode.Error(), err)
}

func TestBlockHash_GenesisRoot(t *testing.T) {
	f := setupGloas(t, 0, 0)

	got, err := f.BlockHash(params.BeaconConfig().ZeroHash)
	require.NoError(t, err)
	assert.Equal(t, [32]byte{}, got)
}

func TestGloasBlock_ChildBuildsOnFull(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()

	// Insert Gloas block A (empty only).
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, rootA, params.BeaconConfig().ZeroHash, blockHashA, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	// Insert payload for A → creates the full node.
	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	fullA := f.store.fullNodeByRoot[rootA]
	require.NotNil(t, fullA)

	// Child for (A, full)
	rootB := indexToHash(2)
	blockHashB := indexToHash(200)
	st, roblock, err = prepareGloasForkchoiceState(ctx, 2, rootB, rootA, blockHashB, blockHashA, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	nodeB := f.store.emptyNodeByRoot[rootB]
	require.NotNil(t, nodeB)
	assert.Equal(t, fullA, nodeB.node.parent)
}

func TestGloasHeadComputation(t *testing.T) {
	f := setupGloas(t, 1, 1)
	s := f.store
	ctx := t.Context()
	balances := make([]uint64, 64)
	// 10 validators with balance 10: proposer boost = 8
	for i := range balances {
		balances[i] = 10
	}
	f.justifiedBalances = balances
	f.store.committeeWeight = uint64(len(balances)*10) / uint64(params.BeaconConfig().SlotsPerEpoch)
	zeroHash := params.BeaconConfig().ZeroHash

	// Head starts at finalized (genesis).
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, zeroHash, headRoot)

	// Insert block A at slot 32, building on genesis.
	//   genesis(full)
	//       |
	//      A(empty) <- head
	slotA := primitives.Slot(32)
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	driftGenesisTime(f, slotA, 0)
	st, blk, err := prepareGloasForkchoiceState(ctx, slotA, rootA, zeroHash, blockHashA, zeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootA, headRoot)
	emptyA := s.choosePayloadContent(s.headNode)
	require.NotNil(t, emptyA)
	require.Equal(t, false, emptyA.full)
	assert.Equal(t, uint64(8), s.headNode.weight) // head node has proposer boost
	assert.Equal(t, uint64(8), s.headNode.balance)
	assert.Equal(t, uint64(0), emptyA.balance) // The empty node does not get proposer boost, just the pending one
	assert.Equal(t, uint64(0), emptyA.weight)

	// Insert payload for A, head is still A.
	//   genesis(full)
	//       |
	//      A(pending)
	//       |
	//      A(full) <- head
	payloadDelay := time.Duration(params.BeaconConfig().SecondsPerSlot/2) * time.Second
	driftGenesisTime(f, slotA, payloadDelay)
	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootA, headRoot)
	fullA := s.choosePayloadContent(s.headNode)
	require.NotNil(t, fullA)
	require.Equal(t, true, fullA.full)
	assert.Equal(t, uint64(8), s.headNode.weight) // head node still has proposer boost
	assert.Equal(t, uint64(8), s.headNode.balance)
	assert.Equal(t, uint64(0), fullA.balance) // The full node does not get proposer boost, just the pending one
	assert.Equal(t, uint64(0), fullA.weight)

	// We move to the next slot. full remains head
	slotB := slotA + 1
	driftGenesisTime(f, slotB, 0)
	require.NoError(t, f.NewSlot(ctx, slotB))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootA, headRoot)
	fullA = s.choosePayloadContent(s.headNode)
	require.NotNil(t, fullA)
	require.Equal(t, true, fullA.full)
	assert.Equal(t, uint64(0), s.headNode.weight) // head node no longer has proposer boost
	assert.Equal(t, uint64(0), s.headNode.balance)
	assert.Equal(t, uint64(0), fullA.balance)
	assert.Equal(t, uint64(0), fullA.weight)

	// Insert block B at slotB, building on full A.
	//   genesis(full)
	//       |
	//      A(pending)
	//       |
	//      A(full)
	//       |
	//      B(empty) <- head
	rootB := indexToHash(2)
	blockHashB := indexToHash(200)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotB, rootB, rootA, blockHashB, blockHashA, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, headRoot)
	emptyB := s.choosePayloadContent(s.headNode)
	require.NotNil(t, emptyB)
	require.Equal(t, false, emptyB.full)
	assert.Equal(t, uint64(0), s.headNode.weight) // no proposer boost: parent weight too low
	assert.Equal(t, uint64(0), s.headNode.balance)
	assert.Equal(t, uint64(0), emptyB.balance)
	assert.Equal(t, uint64(0), emptyB.weight)
	assert.Equal(t, s.headNode.parent, fullA) // parent of head is full A
	assert.Equal(t, uint64(0), fullA.weight)  // the parent does not inherit proposer boost
	assert.Equal(t, uint64(0), fullA.balance)
	assert.Equal(t, uint64(0), fullA.node.balance)
	assert.Equal(t, uint64(0), emptyA.weight)     // neither does the empty block of A
	assert.Equal(t, uint64(0), fullA.node.weight) // no proposer boost propagated

	// Process an attestation for rootA at slotB, voting empty (payloadStatus=false).
	attesters := []uint64{0}
	f.ProcessAttestation(ctx, attesters, rootA, slotB, false)
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, rootA, headRoot) // head should now switch back to A since it has the attestation vote
	hn := s.choosePayloadContent(s.headNode)
	assert.Equal(t, emptyA, hn)

	assert.Equal(t, uint64(10), emptyA.balance)
	assert.Equal(t, uint64(10), emptyA.weight)
	assert.Equal(t, uint64(0), fullA.balance)
	assert.Equal(t, uint64(0), fullA.weight)
	assert.Equal(t, uint64(10), emptyA.node.weight) // Pending node of A has 1 vote supporting it, no proposer boost.
	assert.Equal(t, uint64(0), emptyA.node.balance)
	assert.Equal(t, uint64(0), fullA.weight)  // Full node of A has no proposer boost and no votes.
	assert.Equal(t, uint64(0), fullA.balance) // Full node of A has no proposer boost and no votes.

	// Process an attestation for rootA at slotB, voting full (payloadStatus=true).
	attesters = []uint64{1}
	f.ProcessAttestation(ctx, attesters, rootA, slotB, true)
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, rootB, headRoot) // head now switches to B pending
	hn = s.choosePayloadContent(s.headNode)
	assert.Equal(t, emptyB, hn)

	assert.Equal(t, uint64(10), emptyA.balance)
	assert.Equal(t, uint64(10), emptyA.weight)
	assert.Equal(t, uint64(10), fullA.balance)
	assert.Equal(t, uint64(10), fullA.weight)
	assert.Equal(t, uint64(20), emptyA.node.weight)
	assert.Equal(t, uint64(0), emptyA.node.balance)
	assert.Equal(t, uint64(0), emptyB.node.weight) // empty B has no proposer boost and no votes

	// Move to next slot, head should still be B but without proposer boost.
	slotC := slotB + 1
	driftGenesisTime(f, slotC, 0)
	require.NoError(t, f.NewSlot(ctx, slotC))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, headRoot)
	hn = s.choosePayloadContent(s.headNode)
	require.Equal(t, emptyB, hn)

	assert.Equal(t, uint64(0), emptyB.node.weight) // head node no longer has proposer boost
	assert.Equal(t, uint64(0), emptyB.node.balance)

	assert.Equal(t, uint64(10), emptyA.balance) // empty A still has the vote from the attestation
	assert.Equal(t, uint64(10), emptyA.weight)
	assert.Equal(t, uint64(10), fullA.balance)
	assert.Equal(t, uint64(10), fullA.weight)
	assert.Equal(t, uint64(20), emptyA.node.weight)
	assert.Equal(t, uint64(0), emptyA.node.balance)

	// Insert block C at slotC, building on empty B (no full B exists).
	//   genesis(full)
	//       |
	//      A(pending)
	//      /       \
	//   A(empty)  A(full)
	//              |
	//            B(pending)
	//              |
	//            B(empty)
	//              |
	//            C(pending)
	//              |
	//            C(empty) <- head
	rootC := indexToHash(3)
	blockHashC := indexToHash(300)
	nonMatchingHash := indexToHash(999)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotC, rootC, rootB, blockHashC, nonMatchingHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootC, headRoot)
	emptyC := s.choosePayloadContent(s.headNode)
	require.NotNil(t, emptyC)
	require.Equal(t, false, emptyC.full)
	assert.Equal(t, uint64(0), s.headNode.weight) // no proposer boost: parent weight too low

	assert.Equal(t, uint64(0), emptyB.weight)
	assert.Equal(t, uint64(0), emptyB.node.weight)

	assert.Equal(t, uint64(10), emptyA.weight)
	assert.Equal(t, uint64(10), fullA.weight)
	assert.Equal(t, uint64(20), emptyA.node.weight)

	// Insert payload for C, head should be C full.
	pe, err = prepareGloasForkchoicePayload(rootC)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootC, headRoot)
	fullC := s.choosePayloadContent(s.headNode)
	require.NotNil(t, fullC)
	require.Equal(t, true, fullC.full)

	// Move to slot D, insert block D building on C empty (not C full).
	// D has proposer boost but C full should still be head because
	// shouldExtendPayload returns true (D builds on empty C, not full C).
	slotD := slotC + 1
	driftGenesisTime(f, slotD, 0)
	require.NoError(t, f.NewSlot(ctx, slotD))
	rootD := indexToHash(4)
	blockHashD := indexToHash(400)
	nonMatchingHashD := indexToHash(998)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotD, rootD, rootC, blockHashD, nonMatchingHashD, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, rootD, headRoot)
	emptyD := s.emptyNodeByRoot[rootD]
	require.NotNil(t, emptyD)
	hn = s.choosePayloadContent(s.headNode)
	assert.Equal(t, emptyD, hn)

	assert.Equal(t, uint64(0), emptyD.weight)
	assert.Equal(t, uint64(0), emptyD.node.weight) // no proposer boost: parent weight too low

	assert.Equal(t, uint64(0), emptyC.weight)
	assert.Equal(t, uint64(0), fullC.weight)
	assert.Equal(t, uint64(0), emptyC.node.weight) // no proposer boost: parent weight too low

	// Set full PTC votes for C's payload. Head is still D
	for i := range uint64(fieldparams.PTCSize) {
		emptyC.node.setPayloadAvailabilityVote(i)
	}

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootD, headRoot)

	// Set data availability votes now for C, head should become C full
	for i := range uint64(fieldparams.PTCSize) {
		emptyC.node.setPayloadDataAvailabilityVote(i)
	}
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, rootC, headRoot)
	hn = s.choosePayloadContent(s.headNode)
	assert.Equal(t, fullC, hn) // C full is head, D cannot reorg the payload even though it has proposer boost.

	// Process an attestations for rootD this is the current slot. Three evil validators and 4 honest ones for full C.
	attesters = []uint64{2, 3, 4}
	f.ProcessAttestation(ctx, attesters, rootD, slotD, false)
	attesters = []uint64{5, 6, 7, 8}
	f.ProcessAttestation(ctx, attesters, rootC, slotD, true)
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, rootC, headRoot) // C is still head, D cannot reorg the payload even with the attestation votes.
	hn = s.choosePayloadContent(s.headNode)
	require.Equal(t, fullC, hn)

	assert.Equal(t, uint64(0), emptyD.weight)
	assert.Equal(t, uint64(30), emptyD.node.weight)

	assert.Equal(t, uint64(30), emptyC.weight) // No PB to the empty and full nodes, just the pending one
	assert.Equal(t, uint64(40), fullC.weight)
	assert.Equal(t, uint64(70), emptyC.node.weight)

	assert.Equal(t, uint64(70), emptyB.weight)
}

// TestGloasProposerBoostWithParentWeight is similar to TestGloasHeadComputation
// but adds an attestation on the parent so that shouldApplyProposerBoost
// passes at consecutive slots (parent.weight >= committeeWeight * threshold / 100).
func TestGloasProposerBoostWithParentWeight(t *testing.T) {
	f := setupGloas(t, 1, 1)
	s := f.store
	ctx := t.Context()
	balances := make([]uint64, 64)
	for i := range balances {
		balances[i] = 10
	}
	f.justifiedBalances = balances
	f.store.committeeWeight = uint64(len(balances)*10) / uint64(params.BeaconConfig().SlotsPerEpoch)
	zeroHash := params.BeaconConfig().ZeroHash

	// Insert A at slot 32 building on genesis.
	// Genesis at slot 0: slot gap (0+1 != 32) → boost always applies.
	slotA := primitives.Slot(32)
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	driftGenesisTime(f, slotA, 0)
	st, blk, err := prepareGloasForkchoiceState(ctx, slotA, rootA, zeroHash, blockHashA, zeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootA, headRoot)
	assert.Equal(t, uint64(8), s.headNode.weight) // A gets boost (slot gap with genesis)
	assert.Equal(t, uint64(8), s.headNode.balance)

	// Insert payload for A.
	payloadDelay := time.Duration(params.BeaconConfig().SecondsPerSlot/2) * time.Second
	driftGenesisTime(f, slotA, payloadDelay)
	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	// Attest for fullA so the parent has enough weight for consecutive-slot boost.
	// committeeWeight=20, threshold=20 → need parent.weight >= 4.
	f.ProcessAttestation(ctx, []uint64{9}, rootA, slotA, true)

	// Move to slot 33. Head() propagates the attestation weight.
	slotB := slotA + 1
	driftGenesisTime(f, slotB, 0)
	require.NoError(t, f.NewSlot(ctx, slotB))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootA, headRoot)

	fullA := s.fullNodeByRoot[rootA]
	require.NotNil(t, fullA)
	assert.Equal(t, uint64(10), fullA.weight)
	assert.Equal(t, uint64(10), fullA.balance)

	// Insert B at slot 33 building on fullA (consecutive slot).
	// shouldApplyProposerBoost: parent=fullA, weight=10 >= 4 → boost applies.
	//   genesis(full)
	//       |
	//      A(pending)
	//       |
	//      A(full)
	//       |
	//      B(pending)
	//       |
	//      B(empty) <- head
	rootB := indexToHash(2)
	blockHashB := indexToHash(200)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotB, rootB, rootA, blockHashB, blockHashA, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, headRoot)

	emptyA := s.emptyNodeByRoot[rootA]
	emptyB := s.emptyNodeByRoot[rootB]
	require.NotNil(t, emptyB)

	assert.Equal(t, uint64(8), s.headNode.weight) // B has proposer boost
	assert.Equal(t, uint64(8), s.headNode.balance)
	assert.Equal(t, uint64(0), emptyB.weight)
	assert.Equal(t, uint64(0), emptyB.balance)
	assert.Equal(t, s.headNode.parent, fullA)
	assert.Equal(t, uint64(10), fullA.weight) // fullA.balance(10) + B.node(8) - removeBoost(8) = 10
	assert.Equal(t, uint64(10), fullA.balance)
	assert.Equal(t, uint64(0), fullA.node.balance)
	assert.Equal(t, uint64(0), emptyA.weight)
	assert.Equal(t, uint64(18), fullA.node.weight) // A.node: 0 + fullA(18 pre-remove) + emptyA(0)

	// Move to slot 34. Boost clears, parent attestation persists.
	slotC := slotB + 1
	driftGenesisTime(f, slotC, 0)
	require.NoError(t, f.NewSlot(ctx, slotC))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, headRoot)

	assert.Equal(t, uint64(0), s.headNode.weight) // boost cleared
	assert.Equal(t, uint64(0), s.headNode.balance)
	assert.Equal(t, uint64(10), fullA.weight) // attestation weight persists
	assert.Equal(t, uint64(10), fullA.balance)
	assert.Equal(t, uint64(0), emptyA.weight)
	assert.Equal(t, uint64(10), fullA.node.weight) // A.node: 0 + fullA(10) + emptyA(0)

	// Insert C at slot 34 building on B (consecutive slot).
	// B has no attestation weight → shouldApplyProposerBoost returns false → no boost for C.
	rootC := indexToHash(3)
	blockHashC := indexToHash(300)
	nonMatchingHash := indexToHash(999)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotC, rootC, rootB, blockHashC, nonMatchingHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootC, headRoot)

	assert.Equal(t, uint64(0), s.headNode.weight) // C has no boost (parent weight 0)
	assert.Equal(t, uint64(0), s.headNode.balance)
}

func TestShouldExtendPayload(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()

	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, rootA, params.BeaconConfig().ZeroHash, blockHashA, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))
	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	fn := f.store.fullNodeByRoot[rootA]
	require.NotNil(t, fn)
	n := fn.node

	t.Run("nil full node returns false", func(t *testing.T) {
		assert.Equal(t, false, f.store.shouldExtendPayload(nil))
	})

	t.Run("no votes and no proposer boost returns true", func(t *testing.T) {
		f.store.proposerBoostRoot = [32]byte{}
		assert.Equal(t, true, f.store.shouldExtendPayload(fn))
	})

	t.Run("quorum met returns true", func(t *testing.T) {
		for i := uint64(0); i <= fieldparams.PTCSize/2; i++ {
			n.setPayloadAvailabilityVote(i)
			n.setPayloadDataAvailabilityVote(i)
		}
		assert.Equal(t, true, f.store.shouldExtendPayload(fn))
		n.payloadAvailabilityVote = bitfield.NewBitvector512()
		n.payloadDataAvailabilityVote = bitfield.NewBitvector512()
	})

	t.Run("only availability quorum not enough", func(t *testing.T) {
		for i := uint64(0); i <= fieldparams.PTCSize/2; i++ {
			n.setPayloadAvailabilityVote(i)
		}
		// Set a proposer boost so we don't short-circuit on empty boost root.
		rootB := indexToHash(2)
		f.store.proposerBoostRoot = rootB
		// No empty node for boost root -> returns true.
		assert.Equal(t, true, f.store.shouldExtendPayload(fn))
		n.payloadAvailabilityVote = bitfield.NewBitvector512()
	})

	t.Run("proposer boost root has no empty node returns true", func(t *testing.T) {
		f.store.proposerBoostRoot = indexToHash(99)
		assert.Equal(t, true, f.store.shouldExtendPayload(fn))
	})

	t.Run("boost child parent differs from fn returns true", func(t *testing.T) {
		rootB := indexToHash(2)
		blockHashB := indexToHash(200)
		st, roblock, err := prepareGloasForkchoiceState(ctx, 2, rootB, rootA, blockHashB, blockHashA, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, st, roblock))

		f.store.proposerBoostRoot = rootB
		boostNode := f.store.emptyNodeByRoot[rootB]
		require.NotNil(t, boostNode)
		// B's parent is full A, so parent.node == fn.node -> condition is false, falls through.
		assert.Equal(t, boostNode.node.parent.full, f.store.shouldExtendPayload(fn))
	})

	t.Run("boost child parent is fn and full returns true", func(t *testing.T) {
		rootB := indexToHash(2)
		f.store.proposerBoostRoot = rootB
		boostNode := f.store.emptyNodeByRoot[rootB]
		require.NotNil(t, boostNode)
		require.Equal(t, fn, boostNode.node.parent)
		assert.Equal(t, true, f.store.shouldExtendPayload(fn))
	})

	t.Run("boost child parent is fn but empty returns false", func(t *testing.T) {
		rootC := indexToHash(3)
		blockHashC := indexToHash(300)
		nonMatchingHash := indexToHash(999)
		st, roblock, err := prepareGloasForkchoiceState(ctx, 2, rootC, rootA, blockHashC, nonMatchingHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, st, roblock))

		f.store.proposerBoostRoot = rootC
		boostNode := f.store.emptyNodeByRoot[rootC]
		require.NotNil(t, boostNode)
		emptyA := f.store.emptyNodeByRoot[rootA]
		require.Equal(t, emptyA, boostNode.node.parent)
		assert.Equal(t, false, f.store.shouldExtendPayload(fn))
	})
}

func TestChoosePayloadContent(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()

	t.Run("nil node returns nil", func(t *testing.T) {
		assert.Equal(t, (*PayloadNode)(nil), f.store.choosePayloadContent(nil))
	})

	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, rootA, params.BeaconConfig().ZeroHash, blockHashA, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	emptyA := f.store.emptyNodeByRoot[rootA]
	require.NotNil(t, emptyA)
	n := emptyA.node

	t.Run("no full node returns empty", func(t *testing.T) {
		driftGenesisTime(f, 2, 0)
		assert.Equal(t, emptyA, f.store.choosePayloadContent(n))
	})

	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))
	fullA := f.store.fullNodeByRoot[rootA]
	require.NotNil(t, fullA)

	t.Run("full has more weight returns full", func(t *testing.T) {
		fullA.weight = 10
		emptyA.weight = 5
		assert.Equal(t, fullA, f.store.choosePayloadContent(n))
		fullA.weight = 0
		emptyA.weight = 0
	})

	t.Run("empty has more weight returns empty", func(t *testing.T) {
		fullA.weight = 5
		emptyA.weight = 10
		assert.Equal(t, emptyA, f.store.choosePayloadContent(n))
		fullA.weight = 0
		emptyA.weight = 0
	})

	t.Run("equal weight not previous slot returns full", func(t *testing.T) {
		driftGenesisTime(f, 3, 0)
		assert.Equal(t, fullA, f.store.choosePayloadContent(n))
	})

	t.Run("equal weight previous slot with extend returns full", func(t *testing.T) {
		driftGenesisTime(f, 2, 0)
		f.store.proposerBoostRoot = [32]byte{}
		assert.Equal(t, fullA, f.store.choosePayloadContent(n))
	})

	t.Run("equal weight previous slot without extend returns empty", func(t *testing.T) {
		driftGenesisTime(f, 2, 0)
		rootB := indexToHash(2)
		blockHashB := indexToHash(200)
		nonMatchingHash := indexToHash(999)
		st, roblock, err := prepareGloasForkchoiceState(ctx, 2, rootB, rootA, blockHashB, nonMatchingHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, st, roblock))
		f.store.proposerBoostRoot = rootB
		assert.Equal(t, emptyA, f.store.choosePayloadContent(n))
	})
}

func TestGloasForkedBranches(t *testing.T) {
	f := setupGloas(t, 1, 1)
	s := f.store
	ctx := t.Context()
	balances := make([]uint64, 64)
	for i := range balances {
		balances[i] = 10
	}
	f.justifiedBalances = balances
	s.committeeWeight = uint64(len(balances)*10) / uint64(params.BeaconConfig().SlotsPerEpoch)
	zeroHash := params.BeaconConfig().ZeroHash

	// Build:
	//   genesis(full)
	//       |
	//     A(pending) -- slot 32
	//     /        \
	//   A(empty)  A(full)
	//     |          |
	//   B(pending) C(pending) -- slot 33
	//     |          |
	//   B(empty)  C(empty) + C(full)

	slotA := primitives.Slot(32)
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	driftGenesisTime(f, slotA, 0)
	st, blk, err := prepareGloasForkchoiceState(ctx, slotA, rootA, zeroHash, blockHashA, zeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	slotB := slotA + 1
	driftGenesisTime(f, slotB, 0)
	require.NoError(t, f.NewSlot(ctx, slotB))

	// B builds on A(empty) — non-matching parent hash. Gets proposer boost.
	rootB := indexToHash(2)
	blockHashB := indexToHash(200)
	nonMatchingHash := indexToHash(999)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotB, rootB, rootA, blockHashB, nonMatchingHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	// C builds on A(full) — matching blockHashA.
	rootC := indexToHash(3)
	blockHashC := indexToHash(300)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotB, rootC, rootA, blockHashC, blockHashA, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	pe, err = prepareGloasForkchoicePayload(rootC)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	emptyA := s.emptyNodeByRoot[rootA]
	fullA := s.fullNodeByRoot[rootA]
	emptyB := s.emptyNodeByRoot[rootB]
	emptyC := s.emptyNodeByRoot[rootC]
	fullC := s.fullNodeByRoot[rootC]

	// B wins via proposer boost. And no payload attestations for A's payload yet, so A(empty) wins over A(full).
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, headRoot)

	assert.Equal(t, uint64(0), emptyB.weight)
	assert.Equal(t, uint64(0), emptyB.node.weight) // no proposer boost: parent weight too low
	assert.Equal(t, uint64(0), emptyC.weight)
	assert.Equal(t, uint64(0), fullC.weight)
	assert.Equal(t, uint64(0), emptyC.node.weight)
	assert.Equal(t, uint64(0), emptyA.weight)
	assert.Equal(t, uint64(0), fullA.weight)
	assert.Equal(t, uint64(0), emptyA.node.weight) // no proposer boost: parent weight too low

	// Attestations shift head to C.
	// Validators 0,1 vote for B (payloadStatus=false → pending B).
	f.ProcessAttestation(ctx, []uint64{0, 1}, rootB, slotB, false)
	// Validators 2,3,4 vote for C (payloadStatus=false → pendingC).
	f.ProcessAttestation(ctx, []uint64{2, 3, 4}, rootC, slotB, false)

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootC, headRoot)

	assert.Equal(t, uint64(0), emptyB.weight)
	assert.Equal(t, uint64(20), emptyB.node.weight) // no boost: parent weight was 0 from prev Head
	assert.Equal(t, uint64(0), fullC.weight)
	assert.Equal(t, uint64(0), emptyC.weight)
	assert.Equal(t, uint64(30), emptyC.node.weight)
	assert.Equal(t, uint64(20), emptyA.weight)
	assert.Equal(t, uint64(30), fullA.weight)

	// Move to slot 34, boost clears.
	slot34 := slotB + 1
	driftGenesisTime(f, slot34, 0)
	require.NoError(t, f.NewSlot(ctx, slot34))

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootC, headRoot)

	assert.Equal(t, uint64(20), emptyB.node.weight)
	assert.Equal(t, uint64(30), emptyC.node.weight)
	assert.Equal(t, uint64(20), emptyA.weight)
	assert.Equal(t, uint64(30), fullA.weight)

	// More attestations for B overtake C.
	f.ProcessAttestation(ctx, []uint64{5, 6, 7, 8}, rootB, slot34, false)

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, headRoot)

	assert.Equal(t, uint64(60), emptyB.node.weight)
	assert.Equal(t, uint64(30), emptyC.node.weight)
	assert.Equal(t, uint64(60), emptyA.weight)
	assert.Equal(t, uint64(30), fullA.weight)
}

func TestGloasPTCOverridesProposerBoost(t *testing.T) {
	f := setupGloas(t, 1, 1)
	s := f.store
	ctx := t.Context()
	balances := make([]uint64, 64)
	for i := range balances {
		balances[i] = 10
	}
	f.justifiedBalances = balances
	s.committeeWeight = uint64(len(balances)*10) / uint64(params.BeaconConfig().SlotsPerEpoch)
	zeroHash := params.BeaconConfig().ZeroHash

	slotA := primitives.Slot(32)
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	driftGenesisTime(f, slotA, 0)
	st, blk, err := prepareGloasForkchoiceState(ctx, slotA, rootA, zeroHash, blockHashA, zeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	// PTC quorum for A's payload.
	emptyA := s.emptyNodeByRoot[rootA]
	for i := range uint64(fieldparams.PTCSize) {
		emptyA.node.setPayloadAvailabilityVote(i)
		emptyA.node.setPayloadDataAvailabilityVote(i)
	}

	slotB := slotA + 1
	driftGenesisTime(f, slotB, 0)
	require.NoError(t, f.NewSlot(ctx, slotB))

	rootB := indexToHash(2)
	blockHashB := indexToHash(200)
	nonMatchingHash := indexToHash(999)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotB, rootB, rootA, blockHashB, nonMatchingHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	rootC := indexToHash(3)
	blockHashC := indexToHash(300)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotB, rootC, rootA, blockHashC, blockHashA, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	fullA := s.fullNodeByRoot[rootA]
	emptyB := s.emptyNodeByRoot[rootB]
	emptyC := s.emptyNodeByRoot[rootC]

	// C wins despite B having proposer boost.
	// After boost removal weights tie at 0; PTC quorum on A makes
	// shouldExtendPayload return true, so choosePayloadContent picks fullA → C.
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootC, headRoot)

	assert.Equal(t, uint64(0), emptyB.node.weight) // no proposer boost: parent weight too low
	assert.Equal(t, uint64(0), emptyC.node.weight)
	assert.Equal(t, uint64(0), emptyA.weight)
	assert.Equal(t, uint64(0), fullA.weight)

	// Equal attestations on both sides, PTC still tips to C.
	f.ProcessAttestation(ctx, []uint64{0, 1}, rootB, slotB, false)
	f.ProcessAttestation(ctx, []uint64{2, 3}, rootC, slotB, false)

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootC, headRoot)

	// emptyA.weight = 20 (B votes), fullA.weight = 20 (C votes) → tied, PTC wins.
	assert.Equal(t, uint64(20), emptyB.node.weight) // no boost: parent weight was 0 from prev Head
	assert.Equal(t, uint64(20), emptyC.node.weight)
	assert.Equal(t, uint64(20), emptyA.weight)
	assert.Equal(t, uint64(20), fullA.weight)

	// One more attestation for B breaks the tie.
	f.ProcessAttestation(ctx, []uint64{4}, rootB, slotB, false)

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, headRoot)

	assert.Equal(t, uint64(38), emptyB.node.weight)
	assert.Equal(t, uint64(20), emptyC.node.weight)
	assert.Equal(t, uint64(30), emptyA.weight)
	assert.Equal(t, uint64(20), fullA.weight)
}

func TestSetPTCVote(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()

	root := indexToHash(1)
	blockHash := indexToHash(100)
	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, root, params.BeaconConfig().ZeroHash, blockHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	t.Run("unknown root is no-op", func(t *testing.T) {
		f.SetPTCVote(indexToHash(999), 0, true, true)
	})

	t.Run("payload present only", func(t *testing.T) {
		f.SetPTCVote(root, 5, true, false)
		en := f.store.emptyNodeByRoot[root]
		require.NotNil(t, en)
		assert.Equal(t, true, en.node.payloadAvailabilityVote.BitAt(5))
		assert.Equal(t, false, en.node.payloadDataAvailabilityVote.BitAt(5))
	})

	t.Run("blob data available only", func(t *testing.T) {
		f.SetPTCVote(root, 7, false, true)
		en := f.store.emptyNodeByRoot[root]
		require.NotNil(t, en)
		assert.Equal(t, false, en.node.payloadAvailabilityVote.BitAt(7))
		assert.Equal(t, true, en.node.payloadDataAvailabilityVote.BitAt(7))
	})

	t.Run("both flags", func(t *testing.T) {
		f.SetPTCVote(root, 10, true, true)
		en := f.store.emptyNodeByRoot[root]
		require.NotNil(t, en)
		assert.Equal(t, true, en.node.payloadAvailabilityVote.BitAt(10))
		assert.Equal(t, true, en.node.payloadDataAvailabilityVote.BitAt(10))
	})

	t.Run("neither flag", func(t *testing.T) {
		f.SetPTCVote(root, 15, false, false)
		en := f.store.emptyNodeByRoot[root]
		require.NotNil(t, en)
		assert.Equal(t, false, en.node.payloadAvailabilityVote.BitAt(15))
		assert.Equal(t, false, en.node.payloadDataAvailabilityVote.BitAt(15))
	})
}

func TestGloasDeepForkWeightPropagation(t *testing.T) {
	f := setupGloas(t, 1, 1)
	s := f.store
	ctx := t.Context()
	balances := make([]uint64, 64)
	for i := range balances {
		balances[i] = 10
	}
	f.justifiedBalances = balances
	s.committeeWeight = uint64(len(balances)*10) / uint64(params.BeaconConfig().SlotsPerEpoch)
	zeroHash := params.BeaconConfig().ZeroHash

	// Build:
	//   genesis(full)
	//       |
	//     A(pending) -- slot 32
	//       |
	//     A(full)
	//       |
	//     B(pending) -- slot 33
	//     /        \
	//   B(empty)  B(full)
	//     |          |
	//   C(pending) D(pending) -- slot 34
	//     |          |
	//   C(empty)  D(empty) + D(full)

	slotA := primitives.Slot(32)
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	driftGenesisTime(f, slotA, 0)
	st, blk, err := prepareGloasForkchoiceState(ctx, slotA, rootA, zeroHash, blockHashA, zeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	slotB := slotA + 1
	driftGenesisTime(f, slotB, 0)
	require.NoError(t, f.NewSlot(ctx, slotB))

	rootB := indexToHash(2)
	blockHashB := indexToHash(200)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotB, rootB, rootA, blockHashB, blockHashA, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	pe, err = prepareGloasForkchoicePayload(rootB)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	slotC := slotB + 1
	driftGenesisTime(f, slotC, 0)
	require.NoError(t, f.NewSlot(ctx, slotC))

	// C builds on B(empty) — non-matching hash. Gets proposer boost.
	rootC := indexToHash(3)
	blockHashC := indexToHash(300)
	nonMatchingHash := indexToHash(999)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotC, rootC, rootB, blockHashC, nonMatchingHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	// D builds on B(full) — matching blockHashB.
	rootD := indexToHash(4)
	blockHashD := indexToHash(400)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotC, rootD, rootB, blockHashD, blockHashB, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	pe, err = prepareGloasForkchoicePayload(rootD)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	emptyB := s.emptyNodeByRoot[rootB]
	fullB := s.fullNodeByRoot[rootB]
	emptyC := s.emptyNodeByRoot[rootC]
	emptyD := s.emptyNodeByRoot[rootD]
	fullD := s.fullNodeByRoot[rootD]

	// C wins via proposer boost.
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootC, headRoot)

	assert.Equal(t, uint64(0), emptyC.node.weight) // no proposer boost: parent weight too low
	assert.Equal(t, uint64(0), emptyD.node.weight)
	assert.Equal(t, uint64(0), emptyB.weight)
	assert.Equal(t, uint64(0), fullB.weight)
	assert.Equal(t, uint64(0), emptyB.node.weight) // no proposer boost: parent weight too low

	// Attestations at different levels.
	// Validators 0,1 vote for C (payloadStatus=false → pending C).
	f.ProcessAttestation(ctx, []uint64{0, 1}, rootC, slotC, false)
	// Validators 2,3,4 vote for D (payloadStatus=true → fullD).
	f.ProcessAttestation(ctx, []uint64{2, 3, 4}, rootD, slotC, true)
	// Validators 5,6 vote for B (payloadStatus=true → fullB).
	f.ProcessAttestation(ctx, []uint64{5, 6}, rootB, slotC, true)

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootD, headRoot)

	assert.Equal(t, uint64(20), emptyC.node.weight) // no boost: parent weight was 0 from prev Head
	assert.Equal(t, uint64(30), fullD.weight)
	assert.Equal(t, uint64(30), emptyD.node.weight)
	assert.Equal(t, uint64(20), emptyB.weight)
	assert.Equal(t, uint64(50), fullB.weight)
	assert.Equal(t, uint64(70), emptyB.node.weight) // no boost propagated from C

	// Heavy votes for C branch flip the head back.
	f.ProcessAttestation(ctx, []uint64{7, 8, 9, 10, 11}, rootC, slotC, false)

	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootC, headRoot)

	assert.Equal(t, uint64(78), emptyC.node.weight)
	assert.Equal(t, uint64(30), emptyD.node.weight)
	assert.Equal(t, uint64(70), emptyB.weight)
	assert.Equal(t, uint64(50), fullB.weight)
}

func TestCanonicalNodeAtSlot(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()
	zeroHash := params.BeaconConfig().ZeroHash

	// Insert block A at slot 1 building on genesis.
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	st, blk, err := prepareGloasForkchoiceState(ctx, 1, rootA, zeroHash, blockHashA, zeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	// Insert payload for A.
	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	// Insert block B at slot 2 building on A(full).
	rootB := indexToHash(2)
	blockHashB := indexToHash(101)
	driftGenesisTime(f, 2, 0)
	require.NoError(t, f.NewSlot(ctx, 2))
	st, blk, err = prepareGloasForkchoiceState(ctx, 2, rootB, rootA, blockHashB, blockHashA, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	// Compute head so headNode is set.
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, headRoot)

	// Slot 0 (genesis): should return the genesis root.
	root, full := f.CanonicalNodeAtSlot(0)
	assert.Equal(t, zeroHash, root)
	assert.Equal(t, true, full)

	// Slot 1: A has a payload, should return full=true.
	root, full = f.CanonicalNodeAtSlot(1)
	assert.Equal(t, rootA, root)
	assert.Equal(t, true, full)

	// Slot 2 is the current wall clock slot, so it returns the pending node (full=false).
	root, full = f.CanonicalNodeAtSlot(2)
	assert.Equal(t, rootB, root)
	assert.Equal(t, false, full)
}

func TestCanonicalNodeAtSlot_EmptyPayload(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()
	zeroHash := params.BeaconConfig().ZeroHash

	// Insert block A at slot 1 without a payload.
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	st, blk, err := prepareGloasForkchoiceState(ctx, 1, rootA, zeroHash, blockHashA, zeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	// Insert block B at slot 2 building on A(empty).
	rootB := indexToHash(2)
	blockHashB := indexToHash(101)
	driftGenesisTime(f, 2, 0)
	require.NoError(t, f.NewSlot(ctx, 2))
	st, blk, err = prepareGloasForkchoiceState(ctx, 2, rootB, rootA, blockHashB, zeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, headRoot)

	// Slot 1: A has no payload, so full should be false.
	root, full := f.CanonicalNodeAtSlot(1)
	assert.Equal(t, rootA, root)
	assert.Equal(t, false, full)
}

func TestCanonicalNodeAtSlot_NilHead(t *testing.T) {
	f := setupGloas(t, 0, 0)

	// headNode is nil before calling Head.
	f.store.headNode = nil
	root, full := f.CanonicalNodeAtSlot(0)
	assert.Equal(t, [32]byte{}, root)
	assert.Equal(t, false, full)
}

func TestCanonicalNodeAtSlot_NilParent(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()
	zeroHash := params.BeaconConfig().ZeroHash

	// Compute head so headNode is set to genesis.
	_, err := f.Head(ctx)
	require.NoError(t, err)

	// Simulate a checkpoint-synced tree where the root is at a nonzero slot.
	// The genesis node's parent is nil, so walking past it must not panic.
	genesisNode := f.store.emptyNodeByRoot[zeroHash].node
	genesisNode.slot = 5
	f.store.headNode = genesisNode
	root, full := f.CanonicalNodeAtSlot(3)
	assert.Equal(t, [32]byte{}, root)
	assert.Equal(t, false, full)
}

func TestFullHead_FullPayload(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()
	zeroHash := params.BeaconConfig().ZeroHash

	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	st, blk, err := prepareGloasForkchoiceState(ctx, 1, rootA, zeroHash, blockHashA, zeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	driftGenesisTime(f, 1, 0)
	require.NoError(t, f.NewSlot(ctx, 1))

	hr, bh, full, err := f.FullHead(ctx)
	require.NoError(t, err)
	assert.Equal(t, rootA, hr)
	assert.Equal(t, blockHashA, bh)
	assert.Equal(t, true, full)
}

func TestFullHead_EmptyPayload(t *testing.T) {
	f := setupGloas(t, 0, 0)
	ctx := t.Context()
	zeroHash := params.BeaconConfig().ZeroHash

	// Insert block A at slot 1 with payload.
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	st, blk, err := prepareGloasForkchoiceState(ctx, 1, rootA, zeroHash, blockHashA, zeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	// Insert block B at slot 2 without payload.
	rootB := indexToHash(2)
	blockHashB := indexToHash(101)
	driftGenesisTime(f, 2, 0)
	require.NoError(t, f.NewSlot(ctx, 2))
	st, blk, err = prepareGloasForkchoiceState(ctx, 2, rootB, rootA, blockHashB, blockHashA, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	// Head is B (empty), so blockHash should come from ancestor A.
	hr, bh, full, err := f.FullHead(ctx)
	require.NoError(t, err)
	assert.Equal(t, rootB, hr)
	assert.Equal(t, blockHashA, bh)
	assert.Equal(t, false, full)
}

// TestFullHead_PreGloasBlock_ReturnsFalse verifies that FullHead returns full=false
// for pre-Gloas (Fulu) blocks even though the fork choice store internally creates a
// fullNodeByRoot entry with full=true for them (for EL optimistic validation tracking).
// Without the epoch guard in FullHead, isNewHead would spuriously fire every ticker
// tick because FullHead returned full=true while saveHead stored head.full=false.
func TestFullHead_PreGloasBlock_ReturnsFalse(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.GloasForkEpoch = 100 // Gloas activates far in the future; slot 1 is pre-Gloas
	params.OverrideBeaconConfig(cfg)

	f := setup(0, 0)
	ctx := t.Context()
	zeroHash := params.BeaconConfig().ZeroHash

	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	st, blk, err := prepareForkchoiceState(ctx, 1, rootA, zeroHash, blockHashA, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	driftGenesisTime(f, 1, 0)
	require.NoError(t, f.NewSlot(ctx, 1))

	hr, _, full, err := f.FullHead(ctx)
	require.NoError(t, err)
	assert.Equal(t, rootA, hr)
	assert.Equal(t, false, full, "pre-Gloas block must return full=false from FullHead")
}
