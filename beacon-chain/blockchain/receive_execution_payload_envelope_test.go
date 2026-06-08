package blockchain

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
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
