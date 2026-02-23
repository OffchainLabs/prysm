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
	f := setup(0, 0)
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
	f := setup(0, 0)
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
	f := setup(0, 0)
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
	f := setup(0, 0)

	root := indexToHash(99)
	pe, err := prepareGloasForkchoicePayload(root)
	require.NoError(t, err)

	err = f.InsertPayload(pe)
	require.ErrorContains(t, ErrNilNode.Error(), err)
}

func TestGloasBlock_ChildBuildsOnEmpty(t *testing.T) {
	f := setup(0, 0)
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
	f := setup(0, 0)
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
	f := setup(0, 0)
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
	f := setup(0, 0)

	unknownRoot := indexToHash(999)
	_, err := f.BlockHash(unknownRoot)
	require.ErrorContains(t, ErrNilNode.Error(), err)
}

func TestBlockHash_GenesisRoot(t *testing.T) {
	f := setup(0, 0)

	got, err := f.BlockHash(params.BeaconConfig().ZeroHash)
	require.NoError(t, err)
	assert.Equal(t, [32]byte{}, got)
}

func TestGloasBlock_ChildBuildsOnFull(t *testing.T) {
	f := setup(0, 0)
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
	f := setup(1, 1)
	s := f.store
	ctx := t.Context()
	balances := make([]uint64, 64)
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
	hn := s.choosePayloadContent(s.headNode)
	require.NotNil(t, hn)
	require.Equal(t, false, hn.full)
	assert.Equal(t, uint64(8), s.headNode.weight)
	assert.Equal(t, uint64(8), s.headNode.balance)
	assert.Equal(t, uint64(0), hn.balance)
	assert.Equal(t, uint64(0), hn.weight)

	// Insert payload for A, head is still A.
	//   genesis(full)
	//       |
	//      A(empty)
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
	hn = s.choosePayloadContent(s.headNode)
	require.NotNil(t, hn)
	require.Equal(t, true, hn.full)

	// We move to the next slot. full remains head
	slotB := slotA + 1
	driftGenesisTime(f, slotB, 0)
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootA, headRoot)
	hn = s.choosePayloadContent(s.headNode)
	require.NotNil(t, hn)
	require.Equal(t, true, hn.full)

	// Insert block B at slotB, building on full A.
	//   genesis(full)
	//       |
	//      A(empty)
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
	hn = s.choosePayloadContent(s.headNode)
	require.NotNil(t, hn)
	require.Equal(t, false, hn.full)

	// Process an attestation for rootA at slotB, voting empty (payloadStatus=false).
	attesters := []uint64{0}
	f.ProcessAttestation(ctx, attesters, rootA, slotB, false)
	_, err = f.Head(ctx)
	require.NoError(t, err)

	emptyA := s.emptyNodeByRoot[rootA]
	require.NotNil(t, emptyA)
	fullA := s.fullNodeByRoot[rootA]
	require.NotNil(t, fullA)
	assert.Equal(t, uint64(10), emptyA.balance)
	assert.NotEqual(t, uint64(0), emptyA.weight)
	assert.Equal(t, uint64(0), fullA.balance)
	assert.Equal(t, uint64(0), fullA.weight)
}

func TestShouldExtendPayload(t *testing.T) {
	f := setup(0, 0)
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
	f := setup(0, 0)
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

	t.Run("not previous slot returns full", func(t *testing.T) {
		driftGenesisTime(f, 3, 0)
		assert.Equal(t, fullA, f.store.choosePayloadContent(n))
	})

	t.Run("previous slot with extend returns full", func(t *testing.T) {
		driftGenesisTime(f, 2, 0)
		f.store.proposerBoostRoot = [32]byte{}
		assert.Equal(t, fullA, f.store.choosePayloadContent(n))
	})

	t.Run("previous slot without extend returns empty", func(t *testing.T) {
		driftGenesisTime(f, 2, 0)
		// Build a child on empty A so shouldExtendPayload returns false.
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
