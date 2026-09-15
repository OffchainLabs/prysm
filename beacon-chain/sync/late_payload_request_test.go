package sync

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestLatePayload_Guards(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 1
	params.OverrideBeaconConfig(cfg)
	gloasSlot := primitives.Slot(cfg.SlotsPerEpoch)
	var headRoot [32]byte
	headRoot[0] = 0x42

	tests := []struct {
		name     string
		headSlot primitives.Slot
		slot     primitives.Slot
		started  bool
		syncing  bool
	}{
		{name: "chain not started", headSlot: gloasSlot, slot: gloasSlot},
		{name: "still syncing", headSlot: gloasSlot, slot: gloasSlot, started: true, syncing: true},
		{name: "head is in the future", headSlot: gloasSlot + 1, slot: gloasSlot, started: true},
		{name: "head is before Gloas", headSlot: gloasSlot - 1, slot: gloasSlot, started: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// p2p is nil — if a guard fails to short-circuit and we reach
			// requestPayloadEnvelope's peer lookup, the test would panic.
			s := &Service{
				cfg: &config{
					chain: &mock.ChainService{
						Root:         headRoot[:],
						MockHeadSlot: &tt.headSlot,
					},
					initialSync: &mockSync.Sync{IsSyncing: tt.syncing},
				},
				chainStarted:    &atomic.Bool{},
				badPayloadCache: lruwrpr.New(10),
			}
			if tt.started {
				s.chainStarted.Store(true)
			}
			require.NotPanics(t, func() { s.requestLatePayload(tt.slot) })
		})
	}
}

type latePayloadSyncChecker struct {
	mockSync.Sync
	syncing atomic.Bool
	ticks   atomic.Int32
}

// Count after loading so a counted tick saw the pre-flip value.
func (c *latePayloadSyncChecker) Syncing() bool {
	syncing := c.syncing.Load()
	c.ticks.Add(1)
	return syncing
}

type latePayloadChain struct {
	*mock.ChainService
	onEnvelope func(context.Context, interfaces.ROSignedExecutionPayloadEnvelope) error
	full       atomic.Bool
}

func (c *latePayloadChain) HasFullNode([32]byte) bool {
	return c.full.Load()
}

func (c *latePayloadChain) ReceiveExecutionPayloadEnvelope(ctx context.Context, envelope interfaces.ROSignedExecutionPayloadEnvelope) error {
	return c.onEnvelope(ctx, envelope)
}

func TestRunLatePayloadRequest_InitialSyncHandoff(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotDurationMilliseconds = 200
	params.SetGenesisFork(t, cfg, version.Gloas)
	ctxMap, err := ContextByteVersionsForValRoot(cfg.GenesisValidatorsRoot)
	require.NoError(t, err)

	p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	p1.Peers().SetConnectionState(p2.PeerID(), peers.Connected)
	p1.Peers().SetChainState(p2.PeerID(), &ethpb.StatusV2{})

	headSlot := primitives.Slot(1)
	root := [32]byte{0x42}
	envelope := testSignedEnvelope(headSlot, root[:])
	var requests atomic.Int32
	// Callbacks run off the test goroutine, where require's FailNow is not allowed.
	p2.SetStreamHandler(fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1), func(stream network.Stream) {
		defer func() { _ = stream.Close() }()
		req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
		if !assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req)) {
			return
		}
		assert.Equal(t, p2ptypes.ExecutionPayloadEnvelopesByRootReq{root}, *req)
		// Withhold the payload on the first tick, then make it available.
		if requests.Add(1) > 1 {
			assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2.Encoding(), envelope))
		}
		assert.NoError(t, stream.CloseWrite())
	})

	chain := &latePayloadChain{ChainService: &mock.ChainService{
		Root: root[:], MockHeadSlot: &headSlot, FinalizedCheckPoint: &ethpb.Checkpoint{},
	}}
	var received atomic.Int32
	chain.onEnvelope = func(ctx context.Context, signed interfaces.ROSignedExecutionPayloadEnvelope) error {
		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline)
		env, err := signed.Envelope()
		if !assert.NoError(t, err) {
			return err
		}
		assert.Equal(t, root, env.BeaconBlockRoot())
		if received.Add(1) == 1 {
			return errors.New("transient payload processing failure")
		}
		chain.full.Store(true)
		return nil
	}

	checker := &latePayloadSyncChecker{}
	checker.syncing.Store(true)
	// The head's own payload deadline has already passed before the handoff.
	clock := startup.NewClock(time.Now().Add(-3*cfg.SlotDuration()), cfg.GenesisValidatorsRoot)
	clockWaiter := startup.NewClockSynchronizer()
	require.NoError(t, clockWaiter.SetClock(clock))
	ctx, cancel := context.WithCancel(t.Context())
	s := &Service{
		ctx:         ctx,
		cfg:         &config{chain: chain, p2p: p1, clock: clock, initialSync: checker},
		clockWaiter: clockWaiter, ctxMap: ctxMap,
		chainStarted: &atomic.Bool{}, badPayloadCache: lruwrpr.New(10),
	}
	s.chainStarted.Store(true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runLatePayloadRequest()
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("late payload routine did not stop")
		}
	})
	waitForTicks := func(n int32) {
		t.Helper()
		require.Eventually(t, func() bool { return checker.ticks.Load() >= n }, 5*time.Second, 10*time.Millisecond, "late payload routine did not tick")
	}
	waitForTicks(2)
	require.Zero(t, requests.Load(), "initial sync still owns recovery")
	checker.syncing.Store(false)
	require.Eventually(t, chain.full.Load, 5*time.Second, 10*time.Millisecond, "missing terminal head payload was not retried after handoff")
	require.Equal(t, int32(3), requests.Load())
	require.Equal(t, int32(2), received.Load())
	require.False(t, s.hasBadPayload(root))
	waitForTicks(checker.ticks.Load() + 2)
	require.Equal(t, int32(3), requests.Load(), "a full head needs no further requests")
}
