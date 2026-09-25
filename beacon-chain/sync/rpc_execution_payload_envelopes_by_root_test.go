package sync

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
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

type envelopeBatchReconstructor struct {
	*mockExecution.EngineClient
	batches [][][32]byte
}

func (r *envelopeBatchReconstructor) ReconstructFullGloasExecutionPayloadsByHash(ctx context.Context, hashes [][32]byte) (map[[32]byte]*enginev1.ExecutionPayloadGloas, error) {
	r.batches = append(r.batches, append([][32]byte(nil), hashes...))
	return r.EngineClient.ReconstructFullGloasExecutionPayloadsByHash(ctx, hashes)
}

func TestExecutionPayloadEnvelopesByRootRPCHandler_AvailableEnvelopes(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()
	previousFlags := flags.Get()
	batchFlags := *previousFlags
	batchFlags.BlockBatchLimit = 2
	batchFlags.BlockBatchLimitBurstFactor = 10
	flags.Init(&batchFlags)
	t.Cleanup(func() { flags.Init(previousFlags) })

	ctxMap, err := ContextByteVersionsForValRoot(cfg.GenesisValidatorsRoot)
	require.NoError(t, err)
	topic := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1)
	oldRoot, olderRoot, recentRoot, unknownRoot := [32]byte{1}, [32]byte{2}, [32]byte{3}, [32]byte{4}
	envelopes := []*ethpb.SignedExecutionPayloadEnvelope{
		testSignedEnvelope(cfg.SlotsPerEpoch, oldRoot[:]),
		testSignedEnvelope(cfg.SlotsPerEpoch-1, olderRoot[:]),
		testSignedEnvelope(2*cfg.SlotsPerEpoch+1, recentRoot[:]),
	}
	tests := []struct {
		name        string
		roots       p2ptypes.ExecutionPayloadEnvelopesByRootReq
		wantRoots   [][32]byte
		wantBatches [][][32]byte
	}{
		{
			name: "historical envelope before finalized epoch", roots: p2ptypes.ExecutionPayloadEnvelopesByRootReq{oldRoot},
			wantRoots: [][32]byte{oldRoot}, wantBatches: [][][32]byte{{oldRoot}},
		},
		{
			name: "recent envelope", roots: p2ptypes.ExecutionPayloadEnvelopesByRootReq{recentRoot},
			wantRoots: [][32]byte{recentRoot}, wantBatches: [][][32]byte{{recentRoot}},
		},
		{
			name: "unknown root is omitted", roots: p2ptypes.ExecutionPayloadEnvelopesByRootReq{unknownRoot},
		},
		{
			name:      "mixed ages and unknown root across bounded batches",
			roots:     p2ptypes.ExecutionPayloadEnvelopesByRootReq{oldRoot, olderRoot, unknownRoot, recentRoot},
			wantRoots: [][32]byte{oldRoot, olderRoot, recentRoot}, wantBatches: [][][32]byte{{oldRoot, olderRoot}, {recentRoot}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			beaconDB := testDB.SetupDB(t)
			reconstructor := &envelopeBatchReconstructor{EngineClient: &mockExecution.EngineClient{
				ExecutionPayloadByBlockHash: make(map[[32]byte]*enginev1.ExecutionPayload),
				SlotByBlockHash:             make(map[[32]byte]primitives.Slot),
			}}
			for _, envelope := range envelopes {
				require.NoError(t, beaconDB.SaveExecutionPayloadEnvelope(ctx, envelope))
				payload := envelope.Message.Payload
				hash := bytesutil.ToBytes32(payload.BlockHash)
				reconstructor.SlotByBlockHash[hash] = payload.SlotNumber
				reconstructor.ExecutionPayloadByBlockHash[hash] = &enginev1.ExecutionPayload{
					ParentHash: payload.ParentHash, FeeRecipient: payload.FeeRecipient, StateRoot: payload.StateRoot,
					ReceiptsRoot: payload.ReceiptsRoot, LogsBloom: payload.LogsBloom, PrevRandao: payload.PrevRandao,
					BaseFeePerGas: payload.BaseFeePerGas, BlockHash: payload.BlockHash,
				}
			}
			local, remote := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
			t.Cleanup(func() { assert.NoError(t, local.BHost.Close()) })
			t.Cleanup(func() { assert.NoError(t, remote.BHost.Close()) })
			local.Connect(remote)
			clock := startup.NewClock(time.Now(), cfg.GenesisValidatorsRoot, startup.WithSlotAsNow(3*cfg.SlotsPerEpoch))
			svc := &Service{
				cfg: &config{
					p2p: remote, beaconDB: beaconDB, clock: clock, executionReconstructor: reconstructor,
					chain: &chainMock.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Epoch: 2}},
				},
				rateLimiter: newRateLimiter(remote),
			}
			t.Cleanup(svc.rateLimiter.free)
			handlerDone := make(chan error, 1)
			remote.SetStreamHandler(topic, func(stream network.Stream) {
				defer func() { _ = stream.Close() }()
				req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
				if err := remote.Encoding().DecodeWithMaxLength(stream, req); err != nil {
					handlerDone <- err
					return
				}
				handlerDone <- svc.executionPayloadEnvelopesByRootRPCHandler(ctx, req, stream)
			})

			received, err := SendExecutionPayloadEnvelopesByRootRequest(ctx, clock, local, remote.PeerID(), ctxMap, &tt.roots)
			require.NoError(t, err)
			select {
			case err := <-handlerDone:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for payload envelope handler")
			}
			var receivedRoots [][32]byte
			for _, envelope := range received {
				root := bytesutil.ToBytes32(envelope.Message.BeaconBlockRoot)
				receivedRoots = append(receivedRoots, root)
				assert.Equal(t, root, bytesutil.ToBytes32(envelope.Message.Payload.BlockHash))
				assert.Equal(t, reconstructor.SlotByBlockHash[root], envelope.Message.Payload.SlotNumber)
			}
			assert.Equal(t, true, slices.Equal(tt.wantRoots, receivedRoots), "unexpected roots: got %v, want %v", receivedRoots, tt.wantRoots)
			require.Equal(t, len(tt.wantBatches), len(reconstructor.batches))
			for i, batch := range reconstructor.batches {
				assert.Equal(t, true, slices.Equal(tt.wantBatches[i], batch), "unexpected reconstruction batch %d: got %v, want %v", i, batch, tt.wantBatches[i])
			}
		})
	}
}

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
				Payload: &enginev1.ExecutionPayloadGloas{
					ParentHash:    make([]byte, fieldparams.RootLength),
					FeeRecipient:  make([]byte, 20),
					StateRoot:     make([]byte, fieldparams.RootLength),
					ReceiptsRoot:  make([]byte, fieldparams.RootLength),
					LogsBloom:     make([]byte, 256),
					PrevRandao:    make([]byte, fieldparams.RootLength),
					BaseFeePerGas: make([]byte, fieldparams.RootLength),
					BlockHash:     make([]byte, fieldparams.RootLength),
					SlotNumber:    slot,
				},
				BeaconBlockRoot:       root[:],
				ParentBeaconBlockRoot: make([]byte, fieldparams.RootLength),
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
			err := WriteExecutionPayloadEnvelopeChunk(stream, p2p2.Encoding(), envelopeA)
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
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2p2.Encoding(), envelopeA))
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2p2.Encoding(), envelopeA))

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
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2p2.Encoding(), envelopeC))

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
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2p2.Encoding(), envelopeA))
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2p2.Encoding(), envelopeB))
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2p2.Encoding(), envelopeA))

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

			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2p2.Encoding(), envelopeA))
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2p2.Encoding(), envelopeB))

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
