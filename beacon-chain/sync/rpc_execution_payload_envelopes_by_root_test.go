package sync

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/libp2p/go-libp2p/core/network"
)

func TestSendExecutionPayloadEnvelopesByRootRequest(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)

	protocol := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1)
	clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})

	// Helper: create a SignedExecutionPayloadEnvelope with the given beacon block root and slot.
	makeEnvelope := func(root [32]byte, slot primitives.Slot) *ethpb.SignedExecutionPayloadEnvelope {
		return &ethpb.SignedExecutionPayloadEnvelope{
			Message: &ethpb.ExecutionPayloadEnvelope{
				Payload: &enginev1.ExecutionPayloadDeneb{
					ParentHash:    make([]byte, fieldparams.RootLength),
					FeeRecipient:  make([]byte, 20),
					StateRoot:     make([]byte, fieldparams.RootLength),
					ReceiptsRoot:  make([]byte, fieldparams.RootLength),
					LogsBloom:     make([]byte, 256),
					PrevRandao:    make([]byte, fieldparams.RootLength),
					BaseFeePerGas: make([]byte, fieldparams.RootLength),
					BlockHash:     make([]byte, fieldparams.RootLength),
				},
				BeaconBlockRoot: root[:],
				Slot:            slot,
				StateRoot:       make([]byte, fieldparams.RootLength),
			},
			Signature: make([]byte, fieldparams.BLSSignatureLength),
		}
	}

	rootA := [32]byte{0xAA}
	rootB := [32]byte{0xBB}
	rootC := [32]byte{0xCC}

	t.Run("short valid subset response", func(t *testing.T) {
		// Request [A, B], server responds with only [A] — should accept.
		p1, p2p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		p1.Connect(p2p2)

		envelopeA := makeEnvelope(rootA, 1)

		var wg sync.WaitGroup
		wg.Add(1)
		p2p2.SetStreamHandler(protocol, func(stream network.Stream) {
			defer wg.Done()

			req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
			assert.NoError(t, p2p2.Encoding().DecodeWithMaxLength(stream, req))

			// Only respond with envelope A (skip B).
			err := WriteExecutionPayloadEnvelopeChunk(stream, clock, p2p2.Encoding(), envelopeA)
			assert.NoError(t, err)

			assert.NoError(t, stream.CloseWrite())
		})

		reqRoots := p2ptypes.ExecutionPayloadEnvelopesByRootReq{rootA, rootB}
		envelopes, err := SendExecutionPayloadEnvelopesByRootRequest(t.Context(), clock, p1, p2p2.PeerID(), ctxMap, &reqRoots)
		require.NoError(t, err)
		require.Equal(t, 1, len(envelopes))
		assert.Equal(t, rootA, bytesutil.ToBytes32(envelopes[0].Message.BeaconBlockRoot))

		if util.WaitTimeout(&wg, time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("duplicate response for same root rejected", func(t *testing.T) {
		// Request [A, B], server responds with [A, A] — should reject (duplicate).
		p1, p2p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		p1.Connect(p2p2)

		envelopeA := makeEnvelope(rootA, 1)

		var wg sync.WaitGroup
		wg.Add(1)
		p2p2.SetStreamHandler(protocol, func(stream network.Stream) {
			defer wg.Done()

			req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
			assert.NoError(t, p2p2.Encoding().DecodeWithMaxLength(stream, req))

			// Respond with A twice.
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, clock, p2p2.Encoding(), envelopeA))
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, clock, p2p2.Encoding(), envelopeA))

			assert.NoError(t, stream.CloseWrite())
		})

		reqRoots := p2ptypes.ExecutionPayloadEnvelopesByRootReq{rootA, rootB}
		_, err := SendExecutionPayloadEnvelopesByRootRequest(t.Context(), clock, p1, p2p2.PeerID(), ctxMap, &reqRoots)
		require.ErrorContains(t, "unrequested or duplicate", err)

		if util.WaitTimeout(&wg, time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("unrequested root rejected", func(t *testing.T) {
		// Request [A, B], server responds with [C] — should reject.
		p1, p2p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		p1.Connect(p2p2)

		envelopeC := makeEnvelope(rootC, 1)

		var wg sync.WaitGroup
		wg.Add(1)
		p2p2.SetStreamHandler(protocol, func(stream network.Stream) {
			defer wg.Done()

			req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
			assert.NoError(t, p2p2.Encoding().DecodeWithMaxLength(stream, req))

			// Respond with envelope for rootC, which was not requested.
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, clock, p2p2.Encoding(), envelopeC))

			assert.NoError(t, stream.CloseWrite())
		})

		reqRoots := p2ptypes.ExecutionPayloadEnvelopesByRootReq{rootA, rootB}
		_, err := SendExecutionPayloadEnvelopesByRootRequest(t.Context(), clock, p1, p2p2.PeerID(), ctxMap, &reqRoots)
		require.ErrorContains(t, "unrequested or duplicate", err)

		if util.WaitTimeout(&wg, time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("more responses than requested rejected", func(t *testing.T) {
		// Request [A, B], server responds with [A, B, A] — should reject (too many).
		p1, p2p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		p1.Connect(p2p2)

		envelopeA := makeEnvelope(rootA, 1)
		envelopeB := makeEnvelope(rootB, 1)

		var wg sync.WaitGroup
		wg.Add(1)
		p2p2.SetStreamHandler(protocol, func(stream network.Stream) {
			defer wg.Done()

			req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
			assert.NoError(t, p2p2.Encoding().DecodeWithMaxLength(stream, req))

			// Respond with 3 envelopes for a request of 2.
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, clock, p2p2.Encoding(), envelopeA))
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, clock, p2p2.Encoding(), envelopeB))
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, clock, p2p2.Encoding(), envelopeA))

			assert.NoError(t, stream.CloseWrite())
		})

		reqRoots := p2ptypes.ExecutionPayloadEnvelopesByRootReq{rootA, rootB}
		_, err := SendExecutionPayloadEnvelopesByRootRequest(t.Context(), clock, p1, p2p2.PeerID(), ctxMap, &reqRoots)
		require.ErrorContains(t, "more execution payload envelopes than requested", err)

		if util.WaitTimeout(&wg, time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("perfect match response accepted", func(t *testing.T) {
		// Request [A, B], server responds with [A, B] — should accept.
		p1, p2p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		p1.Connect(p2p2)

		envelopeA := makeEnvelope(rootA, 1)
		envelopeB := makeEnvelope(rootB, 1)

		var wg sync.WaitGroup
		wg.Add(1)
		p2p2.SetStreamHandler(protocol, func(stream network.Stream) {
			defer wg.Done()

			req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
			assert.NoError(t, p2p2.Encoding().DecodeWithMaxLength(stream, req))

			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, clock, p2p2.Encoding(), envelopeA))
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, clock, p2p2.Encoding(), envelopeB))

			assert.NoError(t, stream.CloseWrite())
		})

		reqRoots := p2ptypes.ExecutionPayloadEnvelopesByRootReq{rootA, rootB}
		envelopes, err := SendExecutionPayloadEnvelopesByRootRequest(t.Context(), clock, p1, p2p2.PeerID(), ctxMap, &reqRoots)
		require.NoError(t, err)
		require.Equal(t, 2, len(envelopes))
		assert.Equal(t, rootA, bytesutil.ToBytes32(envelopes[0].Message.BeaconBlockRoot))
		assert.Equal(t, rootB, bytesutil.ToBytes32(envelopes[1].Message.BeaconBlockRoot))

		if util.WaitTimeout(&wg, time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("exceeds max request payloads", func(t *testing.T) {
		// Request more than MaxRequestPayloads — should error immediately.
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		cfg.MaxRequestPayloads = 2
		params.OverrideBeaconConfig(cfg)
		params.BeaconConfig().InitializeForkSchedule()

		reqRoots := p2ptypes.ExecutionPayloadEnvelopesByRootReq{rootA, rootB, rootC}
		_, err := SendExecutionPayloadEnvelopesByRootRequest(t.Context(), clock, nil, "", ctxMap, &reqRoots)
		require.ErrorContains(t, "requested more than MAX_REQUEST_PAYLOADS", err)
	})

	t.Run("empty response accepted", func(t *testing.T) {
		// Request [A], server responds with nothing — should accept with empty result.
		p1, p2p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		p1.Connect(p2p2)

		var wg sync.WaitGroup
		wg.Add(1)
		p2p2.SetStreamHandler(protocol, func(stream network.Stream) {
			defer wg.Done()

			req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
			assert.NoError(t, p2p2.Encoding().DecodeWithMaxLength(stream, req))

			// Close immediately — no envelopes.
			assert.NoError(t, stream.CloseWrite())
		})

		reqRoots := p2ptypes.ExecutionPayloadEnvelopesByRootReq{rootA}
		envelopes, err := SendExecutionPayloadEnvelopesByRootRequest(t.Context(), clock, p1, p2p2.PeerID(), ctxMap, &reqRoots)
		require.NoError(t, err)
		require.Equal(t, 0, len(envelopes))

		if util.WaitTimeout(&wg, time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})
}
