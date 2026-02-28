package sync

import (
	"bytes"
	"context"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

func TestProcessPendingPayloadEnvelope_NoPendingEnvelope(t *testing.T) {
	s := &Service{
		pendingPayloadEnvelopes:  make(map[[32]byte]*ethpb.SignedExecutionPayloadEnvelope),
		seenPayloadEnvelopeCache: lruwrpr.New(10),
		cfg:                      &config{chain: &mock.ChainService{}},
	}
	root := [32]byte{0x01}
	sb := util.NewBeaconBlockGloas()
	signedBlock, err := blocks.NewSignedBeaconBlock(sb)
	require.NoError(t, err)
	// Should return immediately without error when no envelope is queued.
	s.processPendingPayloadEnvelope(context.Background(), signedBlock, root)
}

func TestProcessPendingPayloadEnvelope_AlreadySeen(t *testing.T) {
	ctx := context.Background()
	db := dbtest.SetupDB(t)
	chainService := &mock.ChainService{
		Genesis:             time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &ethpb.Checkpoint{},
		DB:                  db,
	}
	s := &Service{
		pendingPayloadEnvelopes:  make(map[[32]byte]*ethpb.SignedExecutionPayloadEnvelope),
		seenPayloadEnvelopeCache: lruwrpr.New(10),
		cfg:                      &config{chain: chainService, beaconDB: db},
	}

	bid := util.GenerateTestSignedExecutionPayloadBid(1)
	sb := util.NewBeaconBlockGloas()
	sb.Block.Slot = 1
	sb.Block.Body.SignedExecutionPayloadBid = bid
	signedBlock, err := blocks.NewSignedBeaconBlock(sb)
	require.NoError(t, err)
	root, err := signedBlock.Block().HashTreeRoot()
	require.NoError(t, err)

	blockHash := bytesutil.ToBytes32(bid.Message.BlockHash)
	env := testSignedExecutionPayloadEnvelope(t, 1, primitives.BuilderIndex(bid.Message.BuilderIndex), root, blockHash)
	s.pendingPayloadEnvelopes[root] = env
	s.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(mockExecutionPayloadEnvelopeVerifier{})

	// Mark it as already seen, the function should remove the envelope from
	// the queue but not process it further.
	s.setSeenPayloadEnvelope(root, primitives.BuilderIndex(bid.Message.BuilderIndex))
	s.processPendingPayloadEnvelope(ctx, signedBlock, root)
	// Queue should be drained.
	require.Equal(t, 0, len(s.pendingPayloadEnvelopes))
}

func TestProcessPendingPayloadEnvelope_ValidationFailure(t *testing.T) {
	ctx := context.Background()
	db := dbtest.SetupDB(t)
	chainService := &mock.ChainService{
		Genesis:             time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &ethpb.Checkpoint{},
		DB:                  db,
	}
	s := &Service{
		pendingPayloadEnvelopes:  make(map[[32]byte]*ethpb.SignedExecutionPayloadEnvelope),
		seenPayloadEnvelopeCache: lruwrpr.New(10),
		cfg:                      &config{chain: chainService, beaconDB: db},
	}

	bid := util.GenerateTestSignedExecutionPayloadBid(1)
	sb := util.NewBeaconBlockGloas()
	sb.Block.Slot = 1
	sb.Block.Body.SignedExecutionPayloadBid = bid
	signedBlock, err := blocks.NewSignedBeaconBlock(sb)
	require.NoError(t, err)
	root, err := signedBlock.Block().HashTreeRoot()
	require.NoError(t, err)

	blockHash := bytesutil.ToBytes32(bid.Message.BlockHash)
	env := testSignedExecutionPayloadEnvelope(t, 1, primitives.BuilderIndex(bid.Message.BuilderIndex), root, blockHash)
	s.pendingPayloadEnvelopes[root] = env

	// Inject a verifier that fails on builder validation.
	s.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(
		mockExecutionPayloadEnvelopeVerifier{errBuilderValid: errors.New("bad builder")},
	)
	s.processPendingPayloadEnvelope(ctx, signedBlock, root)
	// Queue should be drained even though validation failed.
	require.Equal(t, 0, len(s.pendingPayloadEnvelopes))
	// Should NOT be marked as seen since validation failed.
	require.Equal(t, false, s.hasSeenPayloadEnvelope(root, primitives.BuilderIndex(bid.Message.BuilderIndex)))
}

func TestProcessPendingPayloadEnvelope_HappyPath(t *testing.T) {
	ctx := context.Background()
	db := dbtest.SetupDB(t)
	chainService := &mock.ChainService{
		Genesis:             time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &ethpb.Checkpoint{},
		DB:                  db,
	}
	stateGen := stategen.New(db, doublylinkedtree.New())
	s := &Service{
		pendingPayloadEnvelopes:  make(map[[32]byte]*ethpb.SignedExecutionPayloadEnvelope),
		seenPayloadEnvelopeCache: lruwrpr.New(10),
		cfg: &config{
			chain:    chainService,
			beaconDB: db,
			stateGen: stateGen,
			clock:    startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
		},
	}

	bid := util.GenerateTestSignedExecutionPayloadBid(1)
	sb := util.NewBeaconBlockGloas()
	sb.Block.Slot = 1
	sb.Block.Body.SignedExecutionPayloadBid = bid
	signedBlock, err := blocks.NewSignedBeaconBlock(sb)
	require.NoError(t, err)
	root, err := signedBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, signedBlock))

	st, err := util.NewBeaconStateFulu()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, st, root))

	blockHash := bytesutil.ToBytes32(bid.Message.BlockHash)
	env := testSignedExecutionPayloadEnvelope(t, 1, primitives.BuilderIndex(bid.Message.BuilderIndex), root, blockHash)
	s.pendingPayloadEnvelopes[root] = env
	s.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(mockExecutionPayloadEnvelopeVerifier{})

	builderIdx := primitives.BuilderIndex(bid.Message.BuilderIndex)
	require.Equal(t, false, s.hasSeenPayloadEnvelope(root, builderIdx))
	s.processPendingPayloadEnvelope(ctx, signedBlock, root)
	// Queue drained.
	require.Equal(t, 0, len(s.pendingPayloadEnvelopes))
	// Marked as seen after successful processing.
	require.Equal(t, true, s.hasSeenPayloadEnvelope(root, builderIdx))
}

func TestPrunePendingPayloadEnvelopes(t *testing.T) {
	finalizedEpoch := primitives.Epoch(3)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	s := &Service{
		pendingPayloadEnvelopes: make(map[[32]byte]*ethpb.SignedExecutionPayloadEnvelope),
		cfg: &config{
			chain: &mock.ChainService{
				FinalizedCheckPoint: &ethpb.Checkpoint{Epoch: finalizedEpoch},
			},
		},
	}

	oldRoot := [32]byte{0x01}
	oldEnv := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Slot:            primitives.Slot(finalizedEpoch) * slotsPerEpoch, // exactly at finalized epoch
			BeaconBlockRoot: oldRoot[:],
		},
		Signature: bytes.Repeat([]byte{0xAA}, 96),
	}

	freshRoot := [32]byte{0x02}
	freshEnv := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Slot:            primitives.Slot(finalizedEpoch+1) * slotsPerEpoch, // above finalized epoch
			BeaconBlockRoot: freshRoot[:],
		},
		Signature: bytes.Repeat([]byte{0xBB}, 96),
	}

	s.pendingPayloadEnvelopes[oldRoot] = oldEnv
	s.pendingPayloadEnvelopes[freshRoot] = freshEnv
	require.Equal(t, 2, len(s.pendingPayloadEnvelopes))

	s.prunePendingPayloadEnvelopes()

	require.Equal(t, 1, len(s.pendingPayloadEnvelopes))
	_, ok := s.pendingPayloadEnvelopes[oldRoot]
	require.Equal(t, false, ok)
	_, ok = s.pendingPayloadEnvelopes[freshRoot]
	require.Equal(t, true, ok)
}

func TestQueuePendingPayloadEnvelope_RejectBadSignature(t *testing.T) {
	ctx := context.Background()
	s, _, _, root := setupExecutionPayloadEnvelopeService(t, 1, 1)

	blockHash := [32]byte{0x02}
	signedEnv := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
	e, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedEnv)
	require.NoError(t, err)
	env, err := e.Envelope()
	require.NoError(t, err)

	v := &mockExecutionPayloadEnvelopeVerifier{errSignature: errors.New("bad signature")}
	result, err := s.queuePendingPayloadEnvelope(ctx, v, env, signedEnv)
	require.NotNil(t, err)
	require.Equal(t, pubsub.ValidationReject, result)
	require.Equal(t, 0, len(s.pendingPayloadEnvelopes))
}

func TestQueuePendingPayloadEnvelope_QueuesNewRoot(t *testing.T) {
	ctx := context.Background()
	s, _, _, root := setupExecutionPayloadEnvelopeService(t, 1, 1)

	blockHash := [32]byte{0x02}
	signedEnv := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
	e, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedEnv)
	require.NoError(t, err)
	env, err := e.Envelope()
	require.NoError(t, err)

	v := &mockExecutionPayloadEnvelopeVerifier{}
	result, err := s.queuePendingPayloadEnvelope(ctx, v, env, signedEnv)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
	require.Equal(t, 1, len(s.pendingPayloadEnvelopes))
	_, ok := s.pendingPayloadEnvelopes[root]
	require.Equal(t, true, ok)
}

func TestQueuePendingPayloadEnvelope_DoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	s, _, _, root := setupExecutionPayloadEnvelopeService(t, 1, 1)

	blockHash := [32]byte{0x02}
	first := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
	s.pendingPayloadEnvelopes[root] = first

	second := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
	e, err := blocks.WrappedROSignedExecutionPayloadEnvelope(second)
	require.NoError(t, err)
	env, err := e.Envelope()
	require.NoError(t, err)

	v := &mockExecutionPayloadEnvelopeVerifier{}
	result, err := s.queuePendingPayloadEnvelope(ctx, v, env, second)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
	require.Equal(t, 1, len(s.pendingPayloadEnvelopes))
	require.Equal(t, first, s.pendingPayloadEnvelopes[root])
}

func TestValidateExecutionPayloadEnvelope_RejectBadSignatureBeforeQueue(t *testing.T) {
	ctx := context.Background()
	s, msg, _, _ := setupExecutionPayloadEnvelopeService(t, 1, 1)
	s.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(
		mockExecutionPayloadEnvelopeVerifier{
			errBlockRootSeen: errors.New("not seen"),
			errSignature:     errors.New("bad signature"),
		},
	)

	result, err := s.validateExecutionPayloadEnvelope(ctx, "", msg)
	require.NotNil(t, err)
	require.Equal(t, result, pubsub.ValidationReject)
	// Envelope should NOT be queued when signature is invalid.
	require.Equal(t, 0, len(s.pendingPayloadEnvelopes))
}

func TestValidateExecutionPayloadEnvelope_QueueOnUnknownBlock(t *testing.T) {
	ctx := context.Background()
	s, msg, _, root := setupExecutionPayloadEnvelopeService(t, 1, 1)
	s.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(
		mockExecutionPayloadEnvelopeVerifier{errBlockRootSeen: errors.New("not seen")},
	)

	require.Equal(t, 0, len(s.pendingPayloadEnvelopes))
	result, err := s.validateExecutionPayloadEnvelope(ctx, "", msg)
	require.NotNil(t, err)
	require.Equal(t, result, pubsub.ValidationIgnore)
	// Envelope should be queued.
	require.Equal(t, 1, len(s.pendingPayloadEnvelopes))
	_, ok := s.pendingPayloadEnvelopes[root]
	require.Equal(t, true, ok)
}

func TestValidateExecutionPayloadEnvelope_QueueKeepsFirst(t *testing.T) {
	ctx := context.Background()
	s, msg, _, root := setupExecutionPayloadEnvelopeService(t, 1, 1)
	s.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(
		mockExecutionPayloadEnvelopeVerifier{errBlockRootSeen: errors.New("not seen")},
	)

	// First envelope gets queued.
	_, _ = s.validateExecutionPayloadEnvelope(ctx, "", msg)
	first := s.pendingPayloadEnvelopes[root]

	// Second envelope for the same root should be ignored (keep first).
	_, _ = s.validateExecutionPayloadEnvelope(ctx, "", msg)
	require.Equal(t, 1, len(s.pendingPayloadEnvelopes))
	require.Equal(t, first, s.pendingPayloadEnvelopes[root])
}
