package sync

import (
	"context"
	"fmt"
	"io"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	engpb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// testSignedEnvelope creates a SignedExecutionPayloadEnvelope that is valid for SSZ
// encoding. All fixed-size byte fields are zero-filled to the required length.
func testSignedEnvelope(slot primitives.Slot, beaconBlockRoot []byte) *pb.SignedExecutionPayloadEnvelope {
	root := make([]byte, 32)
	copy(root, beaconBlockRoot)
	return &pb.SignedExecutionPayloadEnvelope{
		Message: &pb.ExecutionPayloadEnvelope{
			Payload: &engpb.ExecutionPayloadGloas{
				ParentHash:    make([]byte, 32),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     root,
			},
			ExecutionRequests: &engpb.ExecutionRequests{},
			BeaconBlockRoot:   root,
			StateRoot:         make([]byte, 32),
			Slot:              slot,
		},
		Signature: make([]byte, 96),
	}
}

func TestValidateEnvelopesByRange(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 10
	cfg.MaxRequestPayloads = 128
	params.OverrideBeaconConfig(cfg)

	gloasStart := util.SlotAtEpoch(t, params.BeaconConfig().GloasForkEpoch)

	maxUint := primitives.Slot(math.MaxUint64)

	tests := []struct {
		name       string
		req        *pb.ExecutionPayloadEnvelopesByRangeRequest
		current    primitives.Slot
		expectErr  bool
		expectedRP *rangeParams
	}{
		{
			name:      "zero count returns error",
			req:       &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 100, Count: 0},
			current:   200,
			expectErr: true,
		},
		{
			name:      "overflow start plus count minus one",
			req:       &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: maxUint - 5, Count: 10},
			current:   maxUint,
			expectErr: true,
		},
		{
			name:       "start after current returns empty rangeParams",
			req:        &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 500, Count: 10},
			current:    400,
			expectedRP: &rangeParams{start: 400, end: 400, size: 0},
		},
		{
			name:       "normal range within limits",
			req:        &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: gloasStart + 10, Count: 20},
			current:    gloasStart + 100,
			expectedRP: &rangeParams{start: gloasStart + 10, end: gloasStart + 29, size: 20},
		},
		{
			// StartSlot is 10 before gloasStart; after clamping start to gloasStart the
			// original end (gloasStart-10+50-1 = gloasStart+39) is still valid.
			name:       "start partially before Gloas clamped to Gloas fork start",
			req:        &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: gloasStart - 10, Count: 50},
			current:    gloasStart + 200,
			expectedRP: &rangeParams{start: gloasStart, end: gloasStart + 39, size: 50},
		},
		{
			name:       "end clamped to current slot",
			req:        &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: gloasStart, Count: 500},
			current:    gloasStart + 10,
			expectedRP: &rangeParams{start: gloasStart, end: gloasStart + 10, size: 128},
		},
		{
			name:       "count capped at MaxRequestPayloads",
			req:        &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: gloasStart, Count: 1000},
			current:    gloasStart + 2000,
			expectedRP: &rangeParams{start: gloasStart, end: gloasStart + 999, size: 128},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rp, err := validateEnvelopesByRange(tc.req, tc.current)
			if tc.expectErr {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, tc.expectedRP)
			assert.Equal(t, tc.expectedRP.start, rp.start)
			assert.Equal(t, tc.expectedRP.end, rp.end)
			assert.Equal(t, tc.expectedRP.size, rp.size)
		})
	}
}

// ---------------------------------------------------------------------------
// executionPayloadEnvelopesByRangeRPCHandler (server handler)
// ---------------------------------------------------------------------------

func TestExecutionPayloadEnvelopesByRangeRPCHandler(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.MaxRequestPayloads = 128
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	ctx := context.Background()
	topicFmt := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRangeTopicV1)

	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)

	t.Run("wrong message type", func(t *testing.T) {
		slot := primitives.Slot(100)
		localP2P, remoteP2P := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		pid := protocol.ID(topicFmt)
		clock2 := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot, startup.WithSlotAsNow(slot))
		svc2 := &Service{
			cfg:         &config{p2p: localP2P, chain: &chainMock.ChainService{Slot: &slot}, clock: clock2},
			rateLimiter: newRateLimiter(localP2P),
		}
		// Install a no-op handler so the stream can be opened.
		remoteP2P.BHost.SetStreamHandler(pid, func(s network.Stream) { _ = s.Reset() })
		localP2P.Connect(remoteP2P)
		stream, sErr := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), pid)
		require.NoError(t, sErr)
		herr := svc2.executionPayloadEnvelopesByRangeRPCHandler(ctx, "not-a-request", stream)
		require.ErrorContains(t, "message is not type", herr)
	})

	t.Run("invalid request count=0", func(t *testing.T) {
		slot := primitives.Slot(100)
		localP2P, remoteP2P := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		protocolID := protocol.ID(topicFmt)

		clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot, startup.WithSlotAsNow(slot))
		svc := &Service{
			cfg: &config{
				p2p:   localP2P,
				chain: &chainMock.ChainService{Slot: &slot},
				clock: clock,
			},
			rateLimiter: newRateLimiter(localP2P),
		}

		var wg sync.WaitGroup
		wg.Add(1)
		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()
			code, _, readErr := readStatusCodeNoDeadline(stream, localP2P.Encoding())
			assert.NoError(t, readErr)
			assert.Equal(t, responseCodeInvalidRequest, code)
		})

		localP2P.Connect(remoteP2P)
		stream, streamErr := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, streamErr)

		msg := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 10, Count: 0}
		handlerErr := svc.executionPayloadEnvelopesByRangeRPCHandler(ctx, msg, stream)
		require.NotNil(t, handlerErr)

		if util.WaitTimeout(&wg, 2*time.Second) {
			t.Fatal("timed out waiting for remote stream handler")
		}
	})

	t.Run("nominal sends chunks for saved envelopes", func(t *testing.T) {
		beaconDB := testDB.SetupDB(t)
		localP2P, remoteP2P := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		protocolID := protocol.ID(topicFmt)

		currentSlot := primitives.Slot(50)
		clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot, startup.WithSlotAsNow(currentSlot))

		// Build blocks at slots 10, 20, 30 (in range) and slot 40 (successor).
		// Each child's bid.ParentBlockHash = parent's envelope BlockHash (= parent's root).
		blockSlots := []primitives.Slot{10, 20, 30, 40}
		roots := make([][32]byte, len(blockSlots))
		var prevRoot [32]byte

		for i, sl := range blockSlots {
			parentRoot := prevRoot
			blk := util.NewBeaconBlockGloas()
			blk.Block.Slot = sl
			copy(blk.Block.ParentRoot, parentRoot[:])
			copy(blk.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockHash, parentRoot[:])
			wsb := util.SaveBlock(t, ctx, beaconDB, blk)
			htr, hErr := wsb.Block().HashTreeRoot()
			require.NoError(t, hErr)
			roots[i] = htr
			prevRoot = htr

			// Save envelopes for the in-range blocks only.
			if sl <= 30 {
				env := testSignedEnvelope(sl, htr[:])
				copy(env.Message.Payload.ParentHash, parentRoot[:])
				require.NoError(t, beaconDB.SaveExecutionPayloadEnvelope(ctx, env))
			}
		}

		mockEngine := &mockExecution.EngineClient{
			ExecutionPayloadByBlockHash: make(map[[32]byte]*engpb.ExecutionPayload, len(roots)),
		}
		for _, root := range roots[:3] {
			mockEngine.ExecutionPayloadByBlockHash[root] = &engpb.ExecutionPayload{
				ParentHash:    make([]byte, 32),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     root[:],
			}
		}

		svc := &Service{
			cfg: &config{
				p2p:                    localP2P,
				beaconDB:               beaconDB,
				chain:                  &chainMock.ChainService{},
				clock:                  clock,
				executionReconstructor: mockEngine,
			},
			availableBlocker: mockBlocker{avail: true},
			rateLimiter:      newRateLimiter(localP2P),
		}

		receivedSlots := make([]primitives.Slot, 0)
		var wg sync.WaitGroup
		wg.Add(1)
		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()
			for {
				env, readErr := readChunkedExecutionPayloadEnvelope(stream, remoteP2P.Encoding(), ctxMap)
				if errors.Is(readErr, io.EOF) {
					break
				}
				assert.NoError(t, readErr)
				if env != nil {
					receivedSlots = append(receivedSlots, env.Message.Slot)
				}
			}
		})

		localP2P.Connect(remoteP2P)
		stream, streamErr := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, streamErr)

		// Request slots 5–35; blocks at 10, 20, 30 are in range, successor at 40.
		msg := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 31}
		handlerErr := svc.executionPayloadEnvelopesByRangeRPCHandler(ctx, msg, stream)
		require.NoError(t, handlerErr)

		if util.WaitTimeout(&wg, 2*time.Second) {
			t.Fatal("timed out waiting for remote stream handler")
		}

		assert.Equal(t, 3, len(receivedSlots))
		assert.Equal(t, primitives.Slot(10), receivedSlots[0])
		assert.Equal(t, primitives.Slot(20), receivedSlots[1])
		assert.Equal(t, primitives.Slot(30), receivedSlots[2])
	})

	t.Run("skips envelopes where payload was empty", func(t *testing.T) {
		beaconDB := testDB.SetupDB(t)
		localP2P, remoteP2P := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		protocolID := protocol.ID(topicFmt)

		currentSlot := primitives.Slot(50)
		clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot, startup.WithSlotAsNow(currentSlot))

		// Build blocks at slots 10, 20, 30 (in range) and slot 40 (successor).
		// Block at slot 20 builds on EMPTY parent (bid.ParentBlockHash != slot 10's envelope BlockHash).
		// This means the backward walk from slot 40 will find slot 30 → slot 20 but NOT slot 10.
		blockSlots := []primitives.Slot{10, 20, 30, 40}
		roots := make([][32]byte, len(blockSlots))
		var prevRoot [32]byte

		for i, sl := range blockSlots {
			parentRoot := prevRoot
			blk := util.NewBeaconBlockGloas()
			blk.Block.Slot = sl
			copy(blk.Block.ParentRoot, parentRoot[:])
			if i == 1 {
				// Slot 20's bid.ParentBlockHash is zero → does NOT match slot 10's envelope BlockHash.
				// The walk will stop here (no envelope found for the zero hash).
			} else {
				copy(blk.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockHash, parentRoot[:])
			}
			wsb := util.SaveBlock(t, ctx, beaconDB, blk)
			htr, hErr := wsb.Block().HashTreeRoot()
			require.NoError(t, hErr)
			roots[i] = htr
			prevRoot = htr

			if sl <= 30 {
				env := testSignedEnvelope(sl, htr[:])
				if i != 1 {
					copy(env.Message.Payload.ParentHash, parentRoot[:])
				}
				require.NoError(t, beaconDB.SaveExecutionPayloadEnvelope(ctx, env))
			}
		}

		mockEngine := &mockExecution.EngineClient{
			ExecutionPayloadByBlockHash: make(map[[32]byte]*engpb.ExecutionPayload, len(roots)),
		}
		for _, root := range roots[:3] {
			mockEngine.ExecutionPayloadByBlockHash[root] = &engpb.ExecutionPayload{
				ParentHash:    make([]byte, 32),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     root[:],
			}
		}

		svc := &Service{
			cfg: &config{
				p2p:                    localP2P,
				beaconDB:               beaconDB,
				chain:                  &chainMock.ChainService{},
				clock:                  clock,
				executionReconstructor: mockEngine,
			},
			availableBlocker: mockBlocker{avail: true},
			rateLimiter:      newRateLimiter(localP2P),
		}

		receivedSlots := make([]primitives.Slot, 0)
		var wg sync.WaitGroup
		wg.Add(1)
		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()
			for {
				env, readErr := readChunkedExecutionPayloadEnvelope(stream, remoteP2P.Encoding(), ctxMap)
				if errors.Is(readErr, io.EOF) {
					break
				}
				assert.NoError(t, readErr)
				if env != nil {
					receivedSlots = append(receivedSlots, env.Message.Slot)
				}
			}
		})

		localP2P.Connect(remoteP2P)
		stream, streamErr := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, streamErr)

		msg := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 31}
		handlerErr := svc.executionPayloadEnvelopesByRangeRPCHandler(ctx, msg, stream)
		require.NoError(t, handlerErr)

		if util.WaitTimeout(&wg, 2*time.Second) {
			t.Fatal("timed out waiting for remote stream handler")
		}

		// Slot 10's envelope is NOT reachable via the backward walk (slot 20 built on empty).
		// Only slots 20 and 30 should be served.
		assert.Equal(t, 2, len(receivedSlots))
		assert.Equal(t, primitives.Slot(20), receivedSlots[0])
		assert.Equal(t, primitives.Slot(30), receivedSlots[1])
	})

}

// ---------------------------------------------------------------------------
// SendExecutionPayloadEnvelopesByRangeRequest (client)
// ---------------------------------------------------------------------------

func TestSendExecutionPayloadEnvelopesByRangeRequest(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.MaxRequestPayloads = 128
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	topic := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRangeTopicV1)
	ctx := t.Context()

	// Set the clock such that the current slot is in the Gloas fork.
	s := uint64(10) * params.BeaconConfig().SecondsPerSlot
	clock := startup.NewClock(time.Now().Add(-time.Second*time.Duration(s)), params.BeaconConfig().GenesisValidatorsRoot)
	ctxMap, err := ContextByteVersionsForValRoot(clock.GenesisValidatorsRoot())
	require.NoError(t, err)

	t.Run("receives envelopes from remote peer", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)

		startSlot := primitives.Slot(5)
		count := uint64(3)

		p2.SetStreamHandler(topic, func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()
			// Read and discard the request.
			req := &pb.ExecutionPayloadEnvelopesByRangeRequest{}
			assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))

			// Write one envelope per slot.
			for i := range count {
				sl := startSlot + primitives.Slot(i)
				env := testSignedEnvelope(sl, make([]byte, 32))
				assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2.Encoding(), env))
			}
		})

		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: startSlot, Count: count}
		envelopes, recvErr := SendExecutionPayloadEnvelopesByRangeRequest(ctx, clock, p1, p2.PeerID(), ctxMap, req)
		require.NoError(t, recvErr)
		require.Equal(t, int(count), len(envelopes))
		assert.Equal(t, startSlot, envelopes[0].Message.Slot)
		assert.Equal(t, startSlot+1, envelopes[1].Message.Slot)
		assert.Equal(t, startSlot+2, envelopes[2].Message.Slot)
	})

	t.Run("empty response from remote peer", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)

		p2.SetStreamHandler(topic, func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()
			// Read and discard request; send nothing.
			req := &pb.ExecutionPayloadEnvelopesByRangeRequest{}
			assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))
		})

		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 10}
		envelopes, recvErr := SendExecutionPayloadEnvelopesByRangeRequest(ctx, clock, p1, p2.PeerID(), ctxMap, req)
		require.NoError(t, recvErr)
		assert.Equal(t, 0, len(envelopes))
	})

	t.Run("slot out of requested range returns error", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)

		startSlot := primitives.Slot(5)
		count := uint64(3)

		p2.SetStreamHandler(topic, func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()
			req := &pb.ExecutionPayloadEnvelopesByRangeRequest{}
			assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))
			// Send an envelope with a slot BEFORE the requested range.
			env := testSignedEnvelope(startSlot-1, make([]byte, 32))
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2.Encoding(), env))
		})

		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: startSlot, Count: count}
		_, recvErr := SendExecutionPayloadEnvelopesByRangeRequest(ctx, clock, p1, p2.PeerID(), ctxMap, req)
		require.NotNil(t, recvErr)
		assert.ErrorContains(t, "outside requested range", recvErr)
	})

	t.Run("slots not monotonically increasing returns error", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)

		startSlot := primitives.Slot(5)

		p2.SetStreamHandler(topic, func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()
			req := &pb.ExecutionPayloadEnvelopesByRangeRequest{}
			assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))
			// Send slot 6 first, then slot 5 (going backwards).
			env1 := testSignedEnvelope(startSlot+1, make([]byte, 32))
			env2 := testSignedEnvelope(startSlot, make([]byte, 32))
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2.Encoding(), env1))
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2.Encoding(), env2))
		})

		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: startSlot, Count: 10}
		_, recvErr := SendExecutionPayloadEnvelopesByRangeRequest(ctx, clock, p1, p2.PeerID(), ctxMap, req)
		require.NotNil(t, recvErr)
		assert.ErrorContains(t, "not greater than previous slot", recvErr)
	})

	t.Run("peer exceeds requested count returns error", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)

		startSlot := primitives.Slot(5)
		count := uint64(2)

		p2.SetStreamHandler(topic, func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()
			req := &pb.ExecutionPayloadEnvelopesByRangeRequest{}
			assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))
			// Send count+1 envelopes (one more than requested).
			for i := uint64(0); i <= count; i++ {
				env := testSignedEnvelope(startSlot+primitives.Slot(i), make([]byte, 32))
				assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2.Encoding(), env))
			}
		})

		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: startSlot, Count: count}
		_, recvErr := SendExecutionPayloadEnvelopesByRangeRequest(ctx, clock, p1, p2.PeerID(), ctxMap, req)
		require.NotNil(t, recvErr)
		assert.ErrorContains(t, "more execution payload envelopes than requested", recvErr)
	})
}
