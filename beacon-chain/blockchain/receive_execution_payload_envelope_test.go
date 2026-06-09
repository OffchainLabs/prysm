package blockchain

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func gloasEnvelopeFixture(t *testing.T, blockRoot [32]byte) (*ethpb.BeaconStateGloas, *ethpb.SignedBeaconBlockGloas, *ethpb.SignedExecutionPayloadEnvelope) {
	t.Helper()

	cfg := params.BeaconConfig()
	slot := primitives.Slot(5)
	parentBeaconRoot := bytes.Repeat([]byte{0x11}, 32)
	blockHash := bytesutil.ToBytes32([]byte("payload-hash"))

	sk, err := bls.RandKey()
	require.NoError(t, err)
	pk := sk.PublicKey().Marshal()

	// Get base state and patch the state to be consistent with the payload we will build and sign.
	base, blk := testGloasState(t, slot, bytesutil.ToBytes32(parentBeaconRoot), blockHash)
	base.Fork = &ethpb.Fork{
		CurrentVersion:  bytes.Repeat([]byte{0x01}, 4),
		PreviousVersion: bytes.Repeat([]byte{0x01}, 4),
		Epoch:           0,
	}
	base.GenesisValidatorsRoot = make([]byte, 32)
	base.Builders = []*ethpb.Builder{{
		Pubkey:           pk,
		Version:          []byte{0},
		ExecutionAddress: make([]byte, 20),
	}}

	emptyRequestsRoot, err := enginev1.EmptyExecutionRequestsHashTreeRoot()
	require.NoError(t, err)

	base.LatestExecutionPayloadBid.ExecutionRequestsRoot = emptyRequestsRoot[:]
	base.LatestExecutionPayloadBid.BlobKzgCommitments = nil

	// Build a payload that is consistent with the committed bid and the state.
	bid := base.LatestExecutionPayloadBid
	payload := &enginev1.ExecutionPayloadGloas{
		ParentHash:    base.LatestBlockHash,
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    bid.PrevRandao,
		BlockNumber:   1,
		GasLimit:      bid.GasLimit,
		Timestamp:     uint64(slot) * cfg.SecondsPerSlot,
		ExtraData:     []byte{},
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     bid.BlockHash,
		Transactions:  [][]byte{},
		Withdrawals:   []*enginev1.Withdrawal{},
		SlotNumber:    slot,
	}

	// Build and sign the envelope.
	envelope := &ethpb.ExecutionPayloadEnvelope{
		BuilderIndex:          0,
		BeaconBlockRoot:       blockRoot[:],
		ParentBeaconBlockRoot: parentBeaconRoot,
		Payload:               payload,
		ExecutionRequests:     &enginev1.ExecutionRequests{},
	}

	domain, err := signing.Domain(base.Fork, slots.ToEpoch(slot), cfg.DomainBeaconBuilder, base.GenesisValidatorsRoot)
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(envelope, domain)
	require.NoError(t, err)
	signedProto := &ethpb.SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: sk.Sign(signingRoot[:]).Marshal(),
	}

	return base, blk, signedProto
}

// TestReceiveExecutionPayloadEnvelope_EmitEvents verifies the event(`execution_payload`
// and `execution_payload_available`) emission behavior of receiver.
// Key regression: Independent of EL validation, `execution_payload_available`
// must be emitted as soon as the payload data is available,
// while `execution_payload` must only be emitted if the payload is imported successfully.
func TestReceiveExecutionPayloadEnvelope_EmitEvents(t *testing.T) {
	tests := []struct {
		name          string
		engine        *mockExecution.EngineClient
		wantErr       bool
		wantAvailable int
		wantProcessed int
	}{
		{
			name:          "valid payload emits available and processed",
			engine:        &mockExecution.EngineClient{},
			wantErr:       false,
			wantAvailable: 1,
			wantProcessed: 1,
		},
		{
			name:          "EL-invalid still emits available but not processed",
			engine:        &mockExecution.EngineClient{ErrNewPayload: execution.ErrInvalidPayloadStatus},
			wantErr:       true,
			wantAvailable: 1,
			wantProcessed: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := setupGloasService(t, tt.engine)
			ctx := t.Context()

			blockRoot := bytesutil.ToBytes32([]byte("envelope-root"))
			base, blk, signedProto := gloasEnvelopeFixture(t, blockRoot)
			insertGloasBlock(t, s, base, blk, blockRoot)

			events := make(chan *feed.Event, 10)
			sub := s.cfg.StateNotifier.StateFeed().Subscribe(events)
			defer sub.Unsubscribe()

			signed, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedProto)
			require.NoError(t, err)

			err = s.ReceiveExecutionPayloadEnvelope(ctx, signed)
			if tt.wantErr {
				require.NotNil(t, err)
				require.Equal(t, true, IsInvalidBlock(err))
			} else {
				require.NoError(t, err)
			}

			got := countStateEventsByType(events)
			require.Equal(t, tt.wantAvailable, got[statefeed.ExecutionPayloadAvailable])
			require.Equal(t, tt.wantProcessed, got[statefeed.ExecutionPayloadProcessed])
		})
	}
}

// TestReceiveExecutionPayloadEnvelope_EmitsHeadV2Event verifies the second
// head_v2 emission with payload status updated from empty to full.
func TestReceiveExecutionPayloadEnvelope_EmitsHeadV2Event(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.InitializeForkSchedule()
	params.OverrideBeaconConfig(cfg)

	s, _ := setupGloasService(t, &mockExecution.EngineClient{})
	ctx := t.Context()

	blockRoot := bytesutil.ToBytes32([]byte("envelope-root"))
	base, blk, signedProto := gloasEnvelopeFixture(t, blockRoot)

	parentRoot := bytesutil.ToBytes32(bytes.Repeat([]byte{0x11}, 32))
	parentBlockHash := [32]byte{0xaa}
	zeroHash := params.BeaconConfig().ZeroHash
	pst, parentROBlock, err := prepareGloasForkchoiceState(ctx, 4, parentRoot, zeroHash, parentBlockHash, zeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, s.cfg.ForkChoiceStore.InsertNode(ctx, pst, parentROBlock))

	insertGloasBlock(t, s, base, blk, blockRoot)

	// Make the envelope's block the current head while it is still payload-empty, so the
	// import drives the empty->full transition for the head root.
	headBlock, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	headState, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)
	s.head = &head{
		root:  blockRoot,
		block: headBlock,
		state: headState,
		slot:  blk.Block.Slot,
		full:  false, // head is not full until the payload is imported
	}

	events := make(chan *feed.Event, 10)
	sub := s.cfg.StateNotifier.StateFeed().Subscribe(events)
	defer sub.Unsubscribe()

	signed, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedProto)
	require.NoError(t, err)
	require.NoError(t, s.ReceiveExecutionPayloadEnvelope(ctx, signed))

	var headV2 *statefeed.HeadV2Data
	var headV2Count int
	for {
		select {
		case e := <-events:
			if e.Type == statefeed.NewHeadV2 {
				headV2Count++
				d, ok := e.Data.(*statefeed.HeadV2Data)
				require.Equal(t, true, ok)
				headV2 = d
			}
			continue
		default:
		}
		break
	}

	// Assertion: `head_v2` must be emitted once,
	// with the payload status updated to "full"
	// and other fields consistent with the imported block and state.
	require.Equal(t, 1, headV2Count)
	require.NotNil(t, headV2)
	require.Equal(t, blockRoot, headV2.Block)
	require.Equal(t, blk.Block.Slot, headV2.Slot)
	require.Equal(t, version.Gloas, headV2.Version)
	require.Equal(t, "full", headV2.PayloadStatus.String())
}

// TestReceiveExecutionPayloadEnvelope_EmitsHeadV2OnceForDuplicateEnvelope verifies
// that only the empty->full transition for the head root emits a head_v2 update.
func TestReceiveExecutionPayloadEnvelope_EmitsHeadV2OnceForDuplicateEnvelope(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.InitializeForkSchedule()
	params.OverrideBeaconConfig(cfg)

	s, _ := setupGloasService(t, &mockExecution.EngineClient{})
	ctx := t.Context()

	blockRoot := bytesutil.ToBytes32([]byte("envelope-root"))
	base, blk, signedProto := gloasEnvelopeFixture(t, blockRoot)

	parentRoot := bytesutil.ToBytes32(bytes.Repeat([]byte{0x11}, 32))
	parentBlockHash := [32]byte{0xaa}
	zeroHash := params.BeaconConfig().ZeroHash
	pst, parentROBlock, err := prepareGloasForkchoiceState(ctx, 4, parentRoot, zeroHash, parentBlockHash, zeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, s.cfg.ForkChoiceStore.InsertNode(ctx, pst, parentROBlock))

	insertGloasBlock(t, s, base, blk, blockRoot)

	headBlock, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	headState, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)
	s.head = &head{
		root:  blockRoot,
		block: headBlock,
		state: headState,
		slot:  blk.Block.Slot,
		full:  false,
	}

	events := make(chan *feed.Event, 10)
	sub := s.cfg.StateNotifier.StateFeed().Subscribe(events)
	defer sub.Unsubscribe()

	signed, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedProto)
	require.NoError(t, err)

	// Deliberately import the same envelope twice to verify that
	// only the first import triggers a head_v2 event for the empty->full transition.
	require.NoError(t, s.ReceiveExecutionPayloadEnvelope(ctx, signed))
	require.NoError(t, s.ReceiveExecutionPayloadEnvelope(ctx, signed))

	var headV2Count int
	for {
		select {
		case e := <-events:
			if e.Type == statefeed.NewHeadV2 {
				headV2Count++
				d, ok := e.Data.(*statefeed.HeadV2Data)
				require.Equal(t, true, ok)
				require.Equal(t, blockRoot, d.Block)
				require.Equal(t, "full", d.PayloadStatus.String())
			}
			continue
		default:
		}
		break
	}

	require.Equal(t, 1, headV2Count)
}

// countStateEventsByType is a helper function for counting the number of events
// of each type received on a channel.
func countStateEventsByType(ch chan *feed.Event) map[feed.EventType]int {
	got := make(map[feed.EventType]int)
	for {
		select {
		case e := <-ch:
			got[e.Type]++
		default:
			return got
		}
	}
}
