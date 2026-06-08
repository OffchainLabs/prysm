package sync

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/async/abool"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/pkg/errors"
)

func TestValidateExecutionPayloadEnvelope_InvalidTopic(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	s := &Service{cfg: &config{p2p: p, initialSync: &mockSync.Sync{}}}

	result, err := s.validateExecutionPayloadEnvelope(ctx, "", &pubsub.Message{
		Message: &pb.Message{},
	})
	require.ErrorIs(t, p2p.ErrInvalidTopic, err)
	require.Equal(t, result, pubsub.ValidationReject)
}

func TestValidateExecutionPayloadEnvelope_AlreadySeen(t *testing.T) {
	ctx := context.Background()
	s, msg, builderIdx, root := setupExecutionPayloadEnvelopeService(t, 1, 1)
	s.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(mockExecutionPayloadEnvelopeVerifier{})

	s.setSeenPayloadEnvelope(root, builderIdx)
	result, err := s.validateExecutionPayloadEnvelope(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, result, pubsub.ValidationIgnore)
}

func TestValidateExecutionPayloadEnvelope_ErrorPathsWithMock(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		verifier  mockExecutionPayloadEnvelopeVerifier
		result    pubsub.ValidationResult
		wantError bool
	}{
		{
			name:     "block root not seen queues envelope",
			verifier: mockExecutionPayloadEnvelopeVerifier{errBlockRootSeen: errors.New("not seen")},
			result:   pubsub.ValidationIgnore,
		},
		{
			name:      "slot below finalized",
			verifier:  mockExecutionPayloadEnvelopeVerifier{errSlotAboveFinalized: errors.New("below finalized")},
			result:    pubsub.ValidationIgnore,
			wantError: true,
		},
		{
			name:      "block root invalid",
			verifier:  mockExecutionPayloadEnvelopeVerifier{errBlockRootValid: errors.New("invalid block")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "slot mismatch",
			verifier:  mockExecutionPayloadEnvelopeVerifier{errSlotMatchesBlock: errors.New("slot mismatch")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "builder mismatch",
			verifier:  mockExecutionPayloadEnvelopeVerifier{errBuilderValid: errors.New("builder mismatch")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "payload hash mismatch",
			verifier:  mockExecutionPayloadEnvelopeVerifier{errPayloadHash: errors.New("payload hash mismatch")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "signature invalid",
			verifier:  mockExecutionPayloadEnvelopeVerifier{errSignature: errors.New("signature invalid")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, msg, _, _ := setupExecutionPayloadEnvelopeService(t, 1, 1)
			s.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(tc.verifier)

			result, err := s.validateExecutionPayloadEnvelope(ctx, "", msg)
			if tc.wantError {
				require.NotNil(t, err)
			}
			require.Equal(t, result, tc.result)
		})
	}
}

func TestValidateExecutionPayloadEnvelope_HappyPath(t *testing.T) {
	ctx := context.Background()
	s, msg, builderIdx, root := setupExecutionPayloadEnvelopeService(t, 1, 1)
	s.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(mockExecutionPayloadEnvelopeVerifier{})

	require.Equal(t, false, s.hasSeenPayloadEnvelope(root, builderIdx))
	result, err := s.validateExecutionPayloadEnvelope(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, result, pubsub.ValidationAccept)
	require.Equal(t, true, s.hasSeenPayloadEnvelope(root, builderIdx))
}

func TestValidateExecutionPayloadEnvelope_GossipEvent(t *testing.T) {
	ctx := context.Background()
	s, msg, builderIdx, root := setupExecutionPayloadEnvelopeService(t, 1, 1)
	s.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(mockExecutionPayloadEnvelopeVerifier{})

	opChannel := make(chan *feed.Event, 1)
	opSub := s.cfg.operationNotifier.OperationFeed().Subscribe(opChannel)
	defer opSub.Unsubscribe()

	t.Run("gossip event emitted on valid envelope", func(t *testing.T) {
		result, err := s.validateExecutionPayloadEnvelope(ctx, "", msg)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)

		signed, ok := msg.ValidatorData.(*ethpb.SignedExecutionPayloadEnvelope)
		require.Equal(t, true, ok)

		select {
		case event := <-opChannel:
			require.Equal(t, feed.EventType(opfeed.ExecutionPayloadGossipReceived), event.Type)
			data, ok := event.Data.(*opfeed.ExecutionPayloadGossipReceivedData)
			require.Equal(t, true, ok)
			require.Equal(t, primitives.Slot(1), data.Slot)
			require.Equal(t, builderIdx, data.BuilderIndex)
			require.Equal(t, root, data.BlockRoot)
			require.Equal(t, bytesutil.ToBytes32(signed.Message.Payload.BlockHash), data.BlockHash)
		case <-time.After(time.Second):
			t.Fatal("expected execution_payload_gossip event was not received")
		}
	})

	t.Run("self-origin envelope does not emit gossip event", func(t *testing.T) {
		result, err := s.validateExecutionPayloadEnvelope(ctx, s.cfg.p2p.PeerID(), msg)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)

		select {
		case event := <-opChannel:
			t.Fatalf("did not expect a gossip event for a self-origin envelope, got type %v", event.Type)
		case <-time.After(50 * time.Millisecond):
			// No event received, as expected.
		}
	})
}

func TestExecutionPayloadEnvelopeSubscriber_WrongMessage(t *testing.T) {
	s := &Service{cfg: &config{}}
	err := s.executionPayloadEnvelopeSubscriber(context.Background(), &ethpb.BeaconBlock{})
	require.ErrorIs(t, errWrongMessage, err)
}

func TestExecutionPayloadEnvelopeSubscriber_HappyPath(t *testing.T) {
	s := &Service{
		cfg:          &config{chain: &mock.ChainService{}},
		chainStarted: abool.New(),
	}
	root := [32]byte{0x01}
	blockHash := [32]byte{0x02}
	env := testSignedExecutionPayloadEnvelope(t, 1, 2, root, blockHash)

	err := s.executionPayloadEnvelopeSubscriber(context.Background(), env)
	require.NoError(t, err)
}

type mockExecutionPayloadEnvelopeVerifier struct {
	errBlockRootSeen      error
	errBlockRootValid     error
	errSlotAboveFinalized error
	errSlotMatchesBlock   error
	errBuilderValid       error
	errPayloadHash        error
	errSignature          error
}

var _ verification.ExecutionPayloadEnvelopeVerifier = &mockExecutionPayloadEnvelopeVerifier{}

func (m *mockExecutionPayloadEnvelopeVerifier) VerifyBlockRootSeen(_ func([32]byte) bool) error {
	return m.errBlockRootSeen
}

func (m *mockExecutionPayloadEnvelopeVerifier) VerifyBlockRootValid(_ func([32]byte) bool) error {
	return m.errBlockRootValid
}

func (m *mockExecutionPayloadEnvelopeVerifier) VerifySlotAboveFinalized(_ primitives.Epoch) error {
	return m.errSlotAboveFinalized
}

func (m *mockExecutionPayloadEnvelopeVerifier) VerifySlotMatchesBlock(_ primitives.Slot) error {
	return m.errSlotMatchesBlock
}

func (m *mockExecutionPayloadEnvelopeVerifier) VerifyBuilderValid(_ interfaces.ROExecutionPayloadBid) error {
	return m.errBuilderValid
}

func (m *mockExecutionPayloadEnvelopeVerifier) VerifyPayloadHash(_ interfaces.ROExecutionPayloadBid) error {
	return m.errPayloadHash
}

func (m *mockExecutionPayloadEnvelopeVerifier) VerifyExecutionRequestsRoot(_ interfaces.ROExecutionPayloadBid) error {
	return nil
}

func (m *mockExecutionPayloadEnvelopeVerifier) VerifySignature(_ state.ReadOnlyBeaconState) error {
	return m.errSignature
}

func (*mockExecutionPayloadEnvelopeVerifier) SatisfyRequirement(_ verification.Requirement) {}

func testNewExecutionPayloadEnvelopeVerifier(m mockExecutionPayloadEnvelopeVerifier) verification.NewExecutionPayloadEnvelopeVerifier {
	return func(_ interfaces.ROSignedExecutionPayloadEnvelope, _ []verification.Requirement) verification.ExecutionPayloadEnvelopeVerifier {
		clone := m
		return &clone
	}
}

func setupExecutionPayloadEnvelopeService(t *testing.T, envelopeSlot, blockSlot primitives.Slot) (*Service, *pubsub.Message, primitives.BuilderIndex, [32]byte) {
	t.Helper()

	ctx := context.Background()
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	chainService := &mock.ChainService{
		Genesis:             time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &ethpb.Checkpoint{},
		DB:                  db,
	}
	stateGen := stategen.New(db, doublylinkedtree.New())
	s := &Service{
		seenPayloadEnvelopeCache: lruwrpr.New(10),
		pendingPayloadEnvelopes:  make(map[[32]byte]map[uint64]*ethpb.SignedExecutionPayloadEnvelope),
		cfg: &config{
			p2p:               p,
			initialSync:       &mockSync.Sync{},
			chain:             chainService,
			beaconDB:          db,
			stateGen:          stateGen,
			clock:             startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			operationNotifier: chainService.OperationNotifier(),
		},
	}

	bid := util.GenerateTestSignedExecutionPayloadBid(blockSlot)
	sb := util.NewBeaconBlockGloas()
	sb.Block.Slot = blockSlot
	sb.Block.Body.SignedExecutionPayloadBid = bid
	signedBlock, err := blocks.NewSignedBeaconBlock(sb)
	require.NoError(t, err)
	root, err := signedBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, signedBlock))

	state, err := util.NewBeaconStateFulu()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, state, root))
	chainService.State = state

	blockHash := bytesutil.ToBytes32(bid.Message.BlockHash)
	env := testSignedExecutionPayloadEnvelope(t, envelopeSlot, primitives.BuilderIndex(bid.Message.BuilderIndex), root, blockHash)
	msg := envelopeToPubsub(t, s, p, env)

	return s, msg, primitives.BuilderIndex(bid.Message.BuilderIndex), root
}

func envelopeToPubsub(t *testing.T, s *Service, p p2p.P2P, env *ethpb.SignedExecutionPayloadEnvelope) *pubsub.Message {
	t.Helper()

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, env)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*ethpb.SignedExecutionPayloadEnvelope]()]
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic = s.addDigestToTopic(topic, digest)

	return &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
}

func TestQueuePendingPayloadEnvelope_SelfBuildInvalidSignature(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name       string
		builderIdx primitives.BuilderIndex
		result     pubsub.ValidationResult
		wantError  bool
	}{
		{
			name:       "self-build with invalid signature is ignored",
			builderIdx: params.BeaconConfig().BuilderIndexSelfBuild,
			result:     pubsub.ValidationIgnore,
		},
		{
			name:       "non-self-build with invalid signature is rejected",
			builderIdx: 42,
			result:     pubsub.ValidationReject,
			wantError:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := p2ptest.NewTestP2P(t)
			chainService := &mock.ChainService{
				Genesis:             time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
				FinalizedCheckPoint: &ethpb.Checkpoint{},
			}
			st, err := util.NewBeaconStateFulu()
			require.NoError(t, err)
			chainService.State = st

			s := &Service{
				seenPayloadEnvelopeCache: lruwrpr.New(10),
				pendingPayloadEnvelopes:  make(map[[32]byte]map[uint64]*ethpb.SignedExecutionPayloadEnvelope),
				cfg: &config{
					p2p:         p,
					initialSync: &mockSync.Sync{},
					chain:       chainService,
					clock:       startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
				},
			}
			s.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(mockExecutionPayloadEnvelopeVerifier{
				errBlockRootSeen: errors.New("not seen"),
				errSignature:     errors.New("bad signature"),
			})

			root := [32]byte{0x01}
			blockHash := [32]byte{0x02}
			env := testSignedExecutionPayloadEnvelope(t, 1, tc.builderIdx, root, blockHash)
			msg := envelopeToPubsub(t, s, p, env)

			result, err := s.validateExecutionPayloadEnvelope(ctx, "", msg)
			if tc.wantError {
				require.NotNil(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.result, result)
		})
	}
}

func testSignedExecutionPayloadEnvelope(t *testing.T, slot primitives.Slot, builderIdx primitives.BuilderIndex, root, blockHash [32]byte) *ethpb.SignedExecutionPayloadEnvelope {
	t.Helper()

	payload := &enginev1.ExecutionPayloadGloas{
		ParentHash:    bytes.Repeat([]byte{0x01}, 32),
		FeeRecipient:  bytes.Repeat([]byte{0x02}, 20),
		StateRoot:     bytes.Repeat([]byte{0x03}, 32),
		ReceiptsRoot:  bytes.Repeat([]byte{0x04}, 32),
		LogsBloom:     bytes.Repeat([]byte{0x05}, 256),
		PrevRandao:    bytes.Repeat([]byte{0x06}, 32),
		BlockNumber:   1,
		GasLimit:      2,
		GasUsed:       3,
		Timestamp:     4,
		BaseFeePerGas: bytes.Repeat([]byte{0x07}, 32),
		BlockHash:     blockHash[:],
		Transactions:  [][]byte{},
		Withdrawals:   []*enginev1.Withdrawal{},
		BlobGasUsed:   0,
		ExcessBlobGas: 0,
		SlotNumber:    slot,
	}

	return &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: payload,
			ExecutionRequests: &enginev1.ExecutionRequests{
				Deposits: []*enginev1.DepositRequest{},
			},
			BuilderIndex:          builderIdx,
			BeaconBlockRoot:       root[:],
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: bytes.Repeat([]byte{0xAA}, 96),
	}
}
