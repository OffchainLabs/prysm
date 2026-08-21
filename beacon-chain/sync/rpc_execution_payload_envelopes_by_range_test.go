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
	envcoverage "github.com/OffchainLabs/prysm/v7/beacon-chain/coverage"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
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
				SlotNumber:    slot,
			},
			ExecutionRequests:     &engpb.ExecutionRequestsGloas{},
			BeaconBlockRoot:       root,
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}
}

func TestEnvelopeServeWindow(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 10
	params.OverrideBeaconConfig(cfg)
	gloasStart := util.SlotAtEpoch(t, params.BeaconConfig().GloasForkEpoch)

	t.Run("intersection preserves original bounds", func(t *testing.T) {
		begin, end, ok := envelopeServeWindow(gloasStart+10, gloasStart+29, gloasStart+100)
		require.Equal(t, true, ok)
		assert.Equal(t, gloasStart+10, begin)
		assert.Equal(t, gloasStart+29, end)
	})
	t.Run("pre-gloas start is intersected, not rewritten", func(t *testing.T) {
		begin, end, ok := envelopeServeWindow(gloasStart-10, gloasStart+39, gloasStart+200)
		require.Equal(t, true, ok)
		assert.Equal(t, gloasStart, begin)
		assert.Equal(t, gloasStart+39, end)
	})
	t.Run("end is clamped to current semantically", func(t *testing.T) {
		begin, end, ok := envelopeServeWindow(gloasStart, gloasStart+499, gloasStart+10)
		require.Equal(t, true, ok)
		assert.Equal(t, gloasStart, begin)
		assert.Equal(t, gloasStart+10, end)
	})
	t.Run("wholly pre-gloas is an empty domain", func(t *testing.T) {
		_, _, ok := envelopeServeWindow(gloasStart-100, gloasStart-1, gloasStart+200)
		require.Equal(t, false, ok)
	})
	t.Run("wholly in the future is an empty domain", func(t *testing.T) {
		_, _, ok := envelopeServeWindow(gloasStart+101, gloasStart+110, gloasStart+100)
		require.Equal(t, false, ok)
	})
	t.Run("unscheduled gloas is an empty domain", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		params.OverrideBeaconConfig(params.MainnetConfig().Copy())
		_, _, ok := envelopeServeWindow(0, 100, 200)
		require.Equal(t, false, ok)
	})
	t.Run("slot zero only is an empty domain when gloas is at genesis", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)
		_, _, ok := envelopeServeWindow(0, 0, 100)
		require.Equal(t, false, ok)
	})
}

func TestValidateExecutionPayloadEnvelopeByRangeResponseRejectsMalformedEnvelope(t *testing.T) {
	req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 1}

	t.Run("nil message", func(t *testing.T) {
		env := testSignedEnvelope(5, make([]byte, 32))
		env.Message = nil

		_, _, err := validateExecutionPayloadEnvelopeByRangeResponse(env, req, 0, nil, false)
		require.NotNil(t, err)
		assert.ErrorContains(t, "invalid execution payload envelope", err)
	})

	t.Run("nil payload", func(t *testing.T) {
		env := testSignedEnvelope(5, make([]byte, 32))
		env.Message.Payload = nil

		_, _, err := validateExecutionPayloadEnvelopeByRangeResponse(env, req, 0, nil, false)
		require.NotNil(t, err)
		assert.ErrorContains(t, "invalid execution payload envelope", err)
	})
}

// ---------------------------------------------------------------------------
// executionPayloadEnvelopesByRangeRPCHandler (server handler)
// ---------------------------------------------------------------------------

// fxEnvBlock is one fixture block with the linkage needed to derive child
// bids and envelopes.
type fxEnvBlock struct {
	slot              primitives.Slot
	root              [32]byte
	parentRoot        [32]byte
	bidHash           [32]byte
	parentPayloadHash [32]byte
}

// envSlotSpec describes one fixture block: whether its payload is revealed
// (committed by its child's bid) and whether its envelope is stored.
type envSlotSpec struct {
	slot     primitives.Slot
	revealed bool
	storeEnv bool
}

type envRangeFixture struct {
	t        *testing.T
	beaconDB db.Database
	chain    *chainMock.ChainService
	engine   *mockExecution.EngineClient
	coverage *envcoverage.Service
	head     *fxEnvBlock
	blocks   map[primitives.Slot]*fxEnvBlock
	seed     byte
}

func (f *envRangeFixture) addBlock(slot primitives.Slot, parent *fxEnvBlock, parentRevealed, canonical bool) *fxEnvBlock {
	f.t.Helper()
	f.seed++
	blk := util.NewBeaconBlockGloas()
	blk.Block.Slot = slot
	var parentRoot, pph [32]byte
	if parent != nil {
		parentRoot = parent.root
		if parentRevealed {
			pph = parent.bidHash
		} else {
			pph = parent.parentPayloadHash
		}
	}
	copy(blk.Block.ParentRoot, parentRoot[:])
	bid := blk.Block.Body.SignedExecutionPayloadBid.Message
	bidHash := bytesutil.ToBytes32([]byte{0xee, f.seed})
	copy(bid.BlockHash, bidHash[:])
	copy(bid.ParentBlockHash, pph[:])
	reqRoot, err := (&engpb.ExecutionRequestsGloas{}).HashTreeRoot()
	require.NoError(f.t, err)
	copy(bid.ExecutionRequestsRoot, reqRoot[:])
	bid.Slot = slot
	wsb := util.SaveBlock(f.t, context.Background(), f.beaconDB, blk)
	root, err := wsb.Block().HashTreeRoot()
	require.NoError(f.t, err)
	if canonical {
		f.chain.CanonicalRoots[root] = true
	}
	b := &fxEnvBlock{slot: slot, root: root, parentRoot: parentRoot, bidHash: bidHash, parentPayloadHash: pph}
	f.blocks[slot] = b
	return b
}

func (f *envRangeFixture) storeEnvelope(b *fxEnvBlock) {
	f.t.Helper()
	env := &pb.SignedExecutionPayloadEnvelope{
		Message: &pb.ExecutionPayloadEnvelope{
			Payload: &engpb.ExecutionPayloadGloas{
				ParentHash:    b.parentPayloadHash[:],
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     b.bidHash[:],
				SlotNumber:    b.slot,
			},
			ExecutionRequests:     &engpb.ExecutionRequestsGloas{},
			BeaconBlockRoot:       b.root[:],
			ParentBeaconBlockRoot: b.parentRoot[:],
		},
		Signature: make([]byte, 96),
	}
	require.NoError(f.t, f.beaconDB.SaveExecutionPayloadEnvelope(context.Background(), env))
	f.engine.ExecutionPayloadByBlockHash[b.bidHash] = &engpb.ExecutionPayload{
		ParentHash:    b.parentPayloadHash[:],
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     b.bidHash[:],
	}
	f.engine.SlotByBlockHash[b.bidHash] = b.slot
}

// newEnvRangeFixture builds a canonical Gloas chain per spec, starts the
// coverage runtime, and waits for it to settle at the expected interval.
func newEnvRangeFixture(t *testing.T, specs []envSlotSpec, current primitives.Slot, expLow primitives.Slot) *envRangeFixture {
	t.Helper()
	f := &envRangeFixture{
		t:        t,
		beaconDB: testDB.SetupDB(t),
		chain:    &chainMock.ChainService{CanonicalRoots: make(map[[32]byte]bool)},
		engine: &mockExecution.EngineClient{
			ExecutionPayloadByBlockHash: make(map[[32]byte]*engpb.ExecutionPayload),
			SlotByBlockHash:             make(map[[32]byte]primitives.Slot),
		},
		blocks: make(map[primitives.Slot]*fxEnvBlock),
	}
	var parent *fxEnvBlock
	parentRevealed := false
	for _, spec := range specs {
		b := f.addBlock(spec.slot, parent, parentRevealed, true)
		if spec.storeEnv {
			f.storeEnvelope(b)
		}
		parent = b
		parentRevealed = spec.revealed
	}
	require.NotNil(t, parent)
	f.head = parent
	f.chain.Root = f.head.root[:]

	cs := startup.NewClockSynchronizer()
	svc, err := envcoverage.New(context.Background(),
		envcoverage.WithDatabase(f.beaconDB),
		envcoverage.WithClockWaiter(cs),
		envcoverage.WithScanBudgets(4, 8, 1<<20),
		envcoverage.WithInterPageYield(0),
		envcoverage.WithTickInterval(time.Hour),
	)
	require.NoError(t, err)
	svc.SetChainView(f.chain)
	svc.Start()
	t.Cleanup(func() { require.NoError(t, svc.Stop()) })
	require.NoError(t, cs.SetClock(startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot, startup.WithSlotAsNow(current))))

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snap := svc.Snapshot()
		if snap.Initialized && snap.High == f.head.slot && snap.Low == expLow {
			break
		}
		svc.Notifier().Notify()
		time.Sleep(5 * time.Millisecond)
	}
	snap := svc.Snapshot()
	require.Equal(t, true, snap.Initialized, "coverage runtime did not settle")
	require.Equal(t, f.head.slot, snap.High, "coverage runtime did not settle at the head")
	require.Equal(t, expLow, snap.Low, "coverage runtime did not settle at the floor")
	// Quiesce the coordinator goroutine so tests may mutate the mock chain
	// afterwards; the serving read APIs remain functional after Stop and a
	// WakeReconcile on the stopped runtime is a harmless no-op.
	require.NoError(t, svc.Stop())
	f.coverage = svc
	return f
}

// newEnvRangeService wires a sync service around the fixture.
func newEnvRangeService(t *testing.T, f *envRangeFixture, current primitives.Slot, localP2P *p2ptest.TestP2P) *Service {
	t.Helper()
	clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot, startup.WithSlotAsNow(current))
	return &Service{
		cfg: &config{
			p2p:                    localP2P,
			beaconDB:               f.beaconDB,
			chain:                  f.chain,
			clock:                  clock,
			executionReconstructor: f.engine,
		},
		rateLimiter:      newRateLimiter(localP2P),
		envelopeCoverage: f.coverage,
	}
}

// collectRangeResponse runs the handler and returns the served slots and the
// handler error.
func collectRangeResponse(t *testing.T, svc *Service, req *pb.ExecutionPayloadEnvelopesByRangeRequest, topicFmt string) ([]primitives.Slot, error) {
	t.Helper()
	ctx := context.Background()
	localP2P, ok := svc.cfg.p2p.(*p2ptest.TestP2P)
	require.Equal(t, true, ok)
	remoteP2P := p2ptest.NewTestP2P(t)
	protocolID := protocol.ID(topicFmt)

	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)

	received := make([]primitives.Slot, 0)
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
			if readErr != nil {
				break
			}
			if env != nil {
				received = append(received, env.Message.Payload.SlotNumber)
			}
		}
	})

	localP2P.Connect(remoteP2P)
	stream, err := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), protocolID)
	require.NoError(t, err)

	handlerErr := svc.executionPayloadEnvelopesByRangeRPCHandler(ctx, req, stream)
	if util.WaitTimeout(&wg, 5*time.Second) {
		t.Fatal("timed out waiting for remote stream handler")
	}
	return received, handlerErr
}

// expectErrorResponseCode reads the response code the remote peer observes.
func expectErrorResponseCode(t *testing.T, svc *Service, req *pb.ExecutionPayloadEnvelopesByRangeRequest, topicFmt string, wantCode byte) {
	t.Helper()
	ctx := context.Background()
	localP2P, ok := svc.cfg.p2p.(*p2ptest.TestP2P)
	require.Equal(t, true, ok)
	remoteP2P := p2ptest.NewTestP2P(t)
	protocolID := protocol.ID(topicFmt)

	var wg sync.WaitGroup
	wg.Add(1)
	remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
		defer wg.Done()
		code, _, readErr := readStatusCodeNoDeadline(stream, localP2P.Encoding())
		assert.NoError(t, readErr)
		assert.Equal(t, wantCode, code)
	})

	localP2P.Connect(remoteP2P)
	stream, err := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), protocolID)
	require.NoError(t, err)
	_ = svc.executionPayloadEnvelopesByRangeRPCHandler(ctx, req, stream)
	if util.WaitTimeout(&wg, 5*time.Second) {
		t.Fatal("timed out waiting for remote stream handler")
	}
}

func TestExecutionPayloadEnvelopesByRangeRPCHandler(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.MaxRequestPayloads = 128
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	ctx := context.Background()
	topicFmt := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRangeTopicV1)
	currentSlot := primitives.Slot(40)

	allRevealed := []envSlotSpec{
		{slot: 10, revealed: true, storeEnv: true},
		{slot: 20, revealed: true, storeEnv: true},
		{slot: 30, revealed: true, storeEnv: true},
		{slot: 40, revealed: true, storeEnv: true},
	}

	t.Run("wrong message type", func(t *testing.T) {
		slot := primitives.Slot(100)
		localP2P, remoteP2P := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		pid := protocol.ID(topicFmt)
		clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot, startup.WithSlotAsNow(slot))
		svc := &Service{
			cfg:         &config{p2p: localP2P, chain: &chainMock.ChainService{Slot: &slot}, clock: clock},
			rateLimiter: newRateLimiter(localP2P),
		}
		remoteP2P.BHost.SetStreamHandler(pid, func(s network.Stream) { _ = s.Reset() })
		localP2P.Connect(remoteP2P)
		stream, sErr := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), pid)
		require.NoError(t, sErr)
		herr := svc.executionPayloadEnvelopesByRangeRPCHandler(ctx, "not-a-request", stream)
		require.ErrorContains(t, "message is not type", herr)
	})

	t.Run("invalid request count=0", func(t *testing.T) {
		f := newEnvRangeFixture(t, allRevealed, currentSlot, 1)
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		expectErrorResponseCode(t, svc, &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 10, Count: 0}, topicFmt, responseCodeInvalidRequest)
	})

	t.Run("overflow on the full count is an invalid request", func(t *testing.T) {
		f := newEnvRangeFixture(t, allRevealed, currentSlot, 1)
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: math.MaxUint64 - 5, Count: 10}
		expectErrorResponseCode(t, svc, req, topicFmt, responseCodeInvalidRequest)
	})

	t.Run("serves indexed envelopes below high with clean EOF", func(t *testing.T) {
		f := newEnvRangeFixture(t, allRevealed, currentSlot, 1)
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		slots, err := collectRangeResponse(t, svc, &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 31}, topicFmt)
		require.NoError(t, err)
		assert.DeepEqual(t, []primitives.Slot{10, 20, 30}, slots)
	})

	t.Run("stored-but-withheld slot is excluded while deeper history serves", func(t *testing.T) {
		specs := []envSlotSpec{
			{slot: 10, revealed: true, storeEnv: true},
			// Slot 20's payload is withheld by its child, but its envelope is
			// stored: it must never be served, and unlike the old backward
			// hash walk, slot 10 behind it still serves.
			{slot: 20, revealed: false, storeEnv: true},
			{slot: 30, revealed: true, storeEnv: true},
			{slot: 40, revealed: true, storeEnv: true},
		}
		f := newEnvRangeFixture(t, specs, currentSlot, 1)
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		slots, err := collectRangeResponse(t, svc, &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 31}, topicFmt)
		require.NoError(t, err)
		assert.DeepEqual(t, []primitives.Slot{10, 30}, slots)
	})

	t.Run("live frontier appends the validated stored head envelope", func(t *testing.T) {
		// Characterization: at this baseline develop's handler seeded its
		// walk from the head bid and served only through head-1, omitting the
		// stored head envelope; the coverage gate serves the proven prefix
		// plus the validated present-head item.
		f := newEnvRangeFixture(t, allRevealed, currentSlot, 1)
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		slots, err := collectRangeResponse(t, svc, &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 36}, topicFmt)
		require.NoError(t, err)
		assert.DeepEqual(t, []primitives.Slot{10, 20, 30, 40}, slots)
		// Serving a present head envelope never advances durable coverage.
		assert.Equal(t, primitives.Slot(40), f.coverage.Snapshot().High)
	})

	t.Run("live frontier without a stored head envelope is a legal short response", func(t *testing.T) {
		specs := []envSlotSpec{
			{slot: 10, revealed: true, storeEnv: true},
			{slot: 20, revealed: true, storeEnv: true},
			{slot: 30, revealed: true, storeEnv: true},
			{slot: 40, revealed: true, storeEnv: false},
		}
		f := newEnvRangeFixture(t, specs, currentSlot, 1)
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		slots, err := collectRangeResponse(t, svc, &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 36}, topicFmt)
		require.NoError(t, err)
		assert.DeepEqual(t, []primitives.Slot{10, 20, 30}, slots)
	})

	t.Run("head-only request serves the stored head envelope", func(t *testing.T) {
		f := newEnvRangeFixture(t, allRevealed, currentSlot, 1)
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		slots, err := collectRangeResponse(t, svc, &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 40, Count: 1}, topicFmt)
		require.NoError(t, err)
		assert.DeepEqual(t, []primitives.Slot{40}, slots)
	})

	t.Run("requestedEnd at MaxUint64 with a finite high serves the live frontier", func(t *testing.T) {
		f := newEnvRangeFixture(t, allRevealed, currentSlot, 1)
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 1, Count: math.MaxUint64}
		slots, err := collectRangeResponse(t, svc, req, topicFmt)
		require.NoError(t, err)
		assert.DeepEqual(t, []primitives.Slot{10, 20, 30, 40}, slots)
	})

	t.Run("wholly future request is a zero-chunk clean EOF", func(t *testing.T) {
		f := newEnvRangeFixture(t, allRevealed, currentSlot, 1)
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		slots, err := collectRangeResponse(t, svc, &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: currentSlot + 10, Count: 5}, topicFmt)
		require.NoError(t, err)
		assert.Equal(t, 0, len(slots))
	})

	t.Run("slot-zero-only request is a zero-chunk clean EOF", func(t *testing.T) {
		f := newEnvRangeFixture(t, allRevealed, currentSlot, 1)
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		slots, err := collectRangeResponse(t, svc, &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 0, Count: 1}, topicFmt)
		require.NoError(t, err)
		assert.Equal(t, 0, len(slots))
	})

	t.Run("coverage stopped below the canonical head refuses atomically", func(t *testing.T) {
		f := newEnvRangeFixture(t, allRevealed, currentSlot, 1)
		// A new canonical head arrives above the anchor without the runtime
		// having reconciled: the same crossing request now refuses.
		b45 := f.addBlock(45, f.head, true, true)
		f.storeEnvelope(b45)
		f.chain.Root = b45.root[:]
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 41}
		expectErrorResponseCode(t, svc, req, topicFmt, responseCodeResourceUnavailable)
	})
}

func TestExecutionPayloadEnvelopesByRangeQuotaSemantics(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.MaxRequestPayloads = 2
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	topicFmt := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRangeTopicV1)
	currentSlot := primitives.Slot(40)
	specs := []envSlotSpec{
		{slot: 10, revealed: true, storeEnv: true},
		{slot: 20, revealed: true, storeEnv: true},
		{slot: 30, revealed: true, storeEnv: true},
		{slot: 40, revealed: true, storeEnv: true},
	}

	t.Run("a quota filled strictly below high serves across a known gap with no head item", func(t *testing.T) {
		f := newEnvRangeFixture(t, specs, currentSlot, 1)
		// Move the canonical head above the anchor: a crossing request would
		// refuse, but a quota filled inside proven coverage never consults
		// the internal-gap or live-head branches.
		b45 := f.addBlock(45, f.head, true, true)
		f.storeEnvelope(b45)
		f.chain.Root = b45.root[:]
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		slots, err := collectRangeResponse(t, svc, &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 41}, topicFmt)
		require.NoError(t, err)
		assert.DeepEqual(t, []primitives.Slot{10, 20}, slots)
	})

	t.Run("quota truncation suppresses the head item", func(t *testing.T) {
		f := newEnvRangeFixture(t, specs, currentSlot, 1)
		localP2P := p2ptest.NewTestP2P(t)
		svc := newEnvRangeService(t, f, currentSlot, localP2P)
		slots, err := collectRangeResponse(t, svc, &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 36}, topicFmt)
		require.NoError(t, err)
		assert.DeepEqual(t, []primitives.Slot{10, 20}, slots)
	})
}

// stubEnvCoverage scripts coherent reads for gate-condition tests.
type stubEnvCoverage struct {
	reads     []*envcoverage.ServeRead
	epochNow  func(call int) uint64
	readCalls int
	epochCall int
	wakes     int
}

func (s *stubEnvCoverage) CoherentServeRead(_ context.Context, _, _ primitives.Slot, _ uint64) (*envcoverage.ServeRead, error) {
	read := s.reads[min(s.readCalls, len(s.reads)-1)]
	s.readCalls++
	return read, nil
}

func (s *stubEnvCoverage) ServeEpoch() uint64 {
	e := s.epochNow(s.epochCall)
	s.epochCall++
	return e
}

func (s *stubEnvCoverage) WakeReconcile() { s.wakes++ }

func TestExecutionPayloadEnvelopesByRangeGateConditions(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.MaxRequestPayloads = 128
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	topicFmt := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRangeTopicV1)
	currentSlot := primitives.Slot(20)
	anchor := bytesutil.ToBytes32([]byte("anchor"))

	newStubService := func(t *testing.T, stub *stubEnvCoverage, chain *chainMock.ChainService, localP2P *p2ptest.TestP2P) *Service {
		clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot, startup.WithSlotAsNow(currentSlot))
		return &Service{
			cfg: &config{
				p2p:      localP2P,
				beaconDB: testDB.SetupDB(t),
				chain:    chain,
				clock:    clock,
				executionReconstructor: &mockExecution.EngineClient{
					ExecutionPayloadByBlockHash: make(map[[32]byte]*engpb.ExecutionPayload),
				},
			},
			rateLimiter:      newRateLimiter(localP2P),
			envelopeCoverage: stub,
		}
	}

	t.Run("uninitialized coverage refuses", func(t *testing.T) {
		stub := &stubEnvCoverage{
			reads:    []*envcoverage.ServeRead{{}},
			epochNow: func(int) uint64 { return 0 },
		}
		svc := newStubService(t, stub, &chainMock.ChainService{}, p2ptest.NewTestP2P(t))
		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 5}
		expectErrorResponseCode(t, svc, req, topicFmt, responseCodeResourceUnavailable)
		assert.Equal(t, 1, stub.wakes)
	})

	t.Run("nil coverage runtime refuses", func(t *testing.T) {
		localP2P := p2ptest.NewTestP2P(t)
		clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot, startup.WithSlotAsNow(currentSlot))
		svc := &Service{
			cfg:         &config{p2p: localP2P, beaconDB: testDB.SetupDB(t), chain: &chainMock.ChainService{}, clock: clock},
			rateLimiter: newRateLimiter(localP2P),
		}
		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 5}
		expectErrorResponseCode(t, svc, req, topicFmt, responseCodeResourceUnavailable)
	})

	t.Run("request start below the covered lower bound refuses", func(t *testing.T) {
		stub := &stubEnvCoverage{
			reads: []*envcoverage.ServeRead{{
				Coverage: envcoverage.Snapshot{Initialized: true, FormatVersion: 1, Low: 8, High: 10, AnchorRoot: anchor},
				HeadRoot: anchor,
			}},
			epochNow: func(int) uint64 { return 0 },
		}
		chain := &chainMock.ChainService{CanonicalRoots: map[[32]byte]bool{anchor: true}}
		svc := newStubService(t, stub, chain, p2ptest.NewTestP2P(t))
		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 10}
		expectErrorResponseCode(t, svc, req, topicFmt, responseCodeResourceUnavailable)
	})

	t.Run("destructive serve-epoch change retries once then succeeds", func(t *testing.T) {
		// W.begin(15) > high(10) with anchor == head: an empty live-frontier
		// response. The first candidate is invalidated by an epoch bump; the
		// retry observes a stable epoch and serves zero chunks with clean EOF.
		read1 := &envcoverage.ServeRead{
			Coverage: envcoverage.Snapshot{Initialized: true, FormatVersion: 1, Low: 1, High: 10, AnchorRoot: anchor},
			Epoch:    1,
			HeadRoot: anchor,
		}
		read2 := &envcoverage.ServeRead{
			Coverage: envcoverage.Snapshot{Initialized: true, FormatVersion: 1, Low: 1, High: 10, AnchorRoot: anchor},
			Epoch:    2,
			HeadRoot: anchor,
		}
		stub := &stubEnvCoverage{
			reads:    []*envcoverage.ServeRead{read1, read2},
			epochNow: func(int) uint64 { return 2 },
		}
		chain := &chainMock.ChainService{CanonicalRoots: map[[32]byte]bool{anchor: true}}
		svc := newStubService(t, stub, chain, p2ptest.NewTestP2P(t))
		slots, err := collectRangeResponse(t, svc, &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 15, Count: 4}, topicFmt)
		require.NoError(t, err)
		assert.Equal(t, 0, len(slots))
		assert.Equal(t, 2, stub.readCalls)
	})

	t.Run("a second serve-epoch invalidation refuses", func(t *testing.T) {
		read := &envcoverage.ServeRead{
			Coverage: envcoverage.Snapshot{Initialized: true, FormatVersion: 1, Low: 1, High: 10, AnchorRoot: anchor},
			Epoch:    1,
			HeadRoot: anchor,
		}
		epoch := uint64(1)
		stub := &stubEnvCoverage{
			reads: []*envcoverage.ServeRead{read},
			epochNow: func(int) uint64 {
				epoch++
				return epoch
			},
		}
		chain := &chainMock.ChainService{CanonicalRoots: map[[32]byte]bool{anchor: true}}
		svc := newStubService(t, stub, chain, p2ptest.NewTestP2P(t))
		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 15, Count: 4}
		expectErrorResponseCode(t, svc, req, topicFmt, responseCodeResourceUnavailable)
		assert.Equal(t, 2, stub.readCalls)
	})

	t.Run("non-canonical anchor refuses", func(t *testing.T) {
		stub := &stubEnvCoverage{
			reads: []*envcoverage.ServeRead{{
				Coverage: envcoverage.Snapshot{Initialized: true, FormatVersion: 1, Low: 1, High: 10, AnchorRoot: anchor},
				HeadRoot: anchor,
			}},
			epochNow: func(int) uint64 { return 0 },
		}
		chain := &chainMock.ChainService{CanonicalRoots: map[[32]byte]bool{}}
		svc := newStubService(t, stub, chain, p2ptest.NewTestP2P(t))
		req := &pb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: 5, Count: 5}
		expectErrorResponseCode(t, svc, req, topicFmt, responseCodeResourceUnavailable)
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
		assert.Equal(t, startSlot, primitives.Slot(envelopes[0].Message.Payload.SlotNumber))
		assert.Equal(t, startSlot+1, primitives.Slot(envelopes[1].Message.Payload.SlotNumber))
		assert.Equal(t, startSlot+2, primitives.Slot(envelopes[2].Message.Payload.SlotNumber))
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
