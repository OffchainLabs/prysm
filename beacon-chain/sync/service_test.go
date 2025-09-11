package sync

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	dbTest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	leakybucket "github.com/OffchainLabs/prysm/v6/container/leaky-bucket"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	gcache "github.com/patrickmn/go-cache"
)

func TestService_StatusZeroEpoch(t *testing.T) {
	bState, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: 0})
	require.NoError(t, err)
	chain := &mockChain.ChainService{
		Genesis: time.Now(),
		State:   bState,
	}
	r := &Service{
		cfg: &config{
			p2p:         p2ptest.NewTestP2P(t),
			initialSync: new(mockSync.Sync),
			chain:       chain,
			clock:       startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
		},
		chainStarted: abool.New(),
	}
	r.chainStarted.Set()

	assert.NoError(t, r.Status(), "Wanted non failing status")
}

func TestSyncHandlers_WaitToSync(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	chainService := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	gs := startup.NewClockSynchronizer()
	r := Service{
		ctx: t.Context(),
		cfg: &config{
			p2p:         p2p,
			chain:       chainService,
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		chainStarted: abool.New(),
		clockWaiter:  gs,
	}

	topic := "/eth2/%x/beacon_block"
	go r.startDiscoveryAndSubscriptions()
	time.Sleep(100 * time.Millisecond)

	var vr [32]byte
	require.NoError(t, gs.SetClock(startup.NewClock(time.Now(), vr)))
	b := []byte("sk")
	b32 := bytesutil.ToBytes32(b)
	sk, err := bls.SecretKeyFromBytes(b32[:])
	require.NoError(t, err)

	msg := util.NewBeaconBlock()
	msg.Block.ParentRoot = util.Random32Bytes(t)
	msg.Signature = sk.Sign([]byte("data")).Marshal()
	p2p.ReceivePubSub(topic, msg)
	// wait for chainstart to be sent
	time.Sleep(400 * time.Millisecond)
	require.Equal(t, true, r.chainStarted.IsSet(), "Did not receive chain start event.")
}

func TestSyncHandlers_WaitForChainStart(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	chainService := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	gs := startup.NewClockSynchronizer()
	r := Service{
		ctx: t.Context(),
		cfg: &config{
			p2p:         p2p,
			chain:       chainService,
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		chainStarted:        abool.New(),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		clockWaiter:         gs,
	}

	var vr [32]byte
	require.NoError(t, gs.SetClock(startup.NewClock(time.Now(), vr)))
	r.waitForChainStart()

	require.Equal(t, true, r.chainStarted.IsSet(), "Did not receive chain start event.")
}

func TestSyncHandlers_WaitTillSynced(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	chainService := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	gs := startup.NewClockSynchronizer()
	r := Service{
		ctx: ctx,
		cfg: &config{
			p2p:           p2p,
			beaconDB:      dbTest.SetupDB(t),
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
			initialSync:   &mockSync.Sync{IsSyncing: false},
		},
		chainStarted:        abool.New(),
		subHandler:          newSubTopicHandler(),
		clockWaiter:         gs,
		initialSyncComplete: make(chan struct{}),
	}
	r.initCaches()

	var vr [32]byte
	require.NoError(t, gs.SetClock(startup.NewClock(time.Now(), vr)))
	r.waitForChainStart()
	require.Equal(t, true, r.chainStarted.IsSet(), "Did not receive chain start event.")

	var err error
	p2p.Digest, err = r.currentForkDigest()
	require.NoError(t, err)

	syncCompleteCh := make(chan bool)
	go func() {
		r.startDiscoveryAndSubscriptions()
		syncCompleteCh <- true
	}()

	blockChan := make(chan *feed.Event, 1)
	sub := r.cfg.blockNotifier.BlockFeed().Subscribe(blockChan)
	defer sub.Unsubscribe()

	b := []byte("sk")
	b32 := bytesutil.ToBytes32(b)
	sk, err := bls.SecretKeyFromBytes(b32[:])
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ParentRoot = util.Random32Bytes(t)
	msg.Signature = sk.Sign([]byte("data")).Marshal()

	// Save block into DB so that validateBeaconBlockPubSub() process gets short cut.
	util.SaveBlock(t, ctx, r.cfg.beaconDB, msg)

	topic := "/eth2/%x/beacon_block"
	p2p.ReceivePubSub(topic, msg)
	assert.Equal(t, 0, len(blockChan), "block was received by sync service despite not being fully synced")

	close(r.initialSyncComplete)
	<-syncCompleteCh

	p2p.ReceivePubSub(topic, msg)

	select {
	case <-blockChan:
	case <-ctx.Done():
	}
	assert.NoError(t, ctx.Err())
}

func TestSyncService_StopCleanly(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	chainService := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	ctx, cancel := context.WithCancel(t.Context())
	gs := startup.NewClockSynchronizer()
	r := Service{
		ctx:    ctx,
		cancel: cancel,
		cfg: &config{
			p2p:         p2p,
			chain:       chainService,
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		chainStarted:        abool.New(),
		subHandler:          newSubTopicHandler(),
		clockWaiter:         gs,
		initialSyncComplete: make(chan struct{}),
	}
	markInitSyncComplete(t, &r)

	go r.startDiscoveryAndSubscriptions()
	var vr [32]byte
	require.NoError(t, gs.SetClock(startup.NewClock(time.Now(), vr)))
	r.waitForChainStart()

	var err error
	p2p.Digest, err = r.currentForkDigest()
	require.NoError(t, err)

	// wait for chainstart to be sent
	time.Sleep(2 * time.Second)
	require.Equal(t, true, r.chainStarted.IsSet(), "Did not receive chain start event.")

	require.NotEqual(t, 0, len(r.cfg.p2p.PubSub().GetTopics()))
	require.NotEqual(t, 0, len(r.cfg.p2p.Host().Mux().Protocols()))

	// Both pubsub and rpc topics should be unsubscribed.
	require.NoError(t, r.Stop())

	// Sleep to allow pubsub topics to be deregistered.
	time.Sleep(1 * time.Second)
	require.Equal(t, 0, len(r.cfg.p2p.PubSub().GetTopics()))
	require.Equal(t, 0, len(r.cfg.p2p.Host().Mux().Protocols()))
}

func TestService_Stop_SendsGoodbyeMessages(t *testing.T) {
	// Create test peers
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p3 := p2ptest.NewTestP2P(t)

	// Connect peers
	p1.Connect(p2)
	p1.Connect(p3)

	// Register peers in the peer status
	p1.Peers().Add(nil, p2.BHost.ID(), p2.BHost.Addrs()[0], network.DirOutbound)
	p1.Peers().Add(nil, p3.BHost.ID(), p3.BHost.Addrs()[0], network.DirOutbound)
	p1.Peers().SetConnectionState(p2.BHost.ID(), peers.Connected)
	p1.Peers().SetConnectionState(p3.BHost.ID(), peers.Connected)

	// Create service with connected peers
	d := dbTest.SetupDB(t)
	chain := &mockChain.ChainService{Genesis: time.Now(), ValidatorsRoot: [32]byte{}}
	ctx, cancel := context.WithCancel(context.Background())

	r := &Service{
		cfg: &config{
			beaconDB: d,
			p2p:      p1,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
		},
		ctx:         ctx,
		cancel:      cancel,
		rateLimiter: newRateLimiter(p1),
	}

	// Initialize context map for RPC
	ctxMap, err := ContextByteVersionsForValRoot(chain.ValidatorsRoot)
	require.NoError(t, err)
	r.ctxMap = ctxMap

	// Setup rate limiter for goodbye topic
	pcl := protocol.ID("/eth2/beacon_chain/req/goodbye/1/ssz_snappy")
	topic := string(pcl)
	r.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(1, 1, time.Second, false)

	// Track goodbye messages received
	var goodbyeMessages sync.Map
	var wg sync.WaitGroup

	wg.Add(2)

	p2.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
		defer wg.Done()
		out := new(primitives.SSZUint64)
		require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, out))
		goodbyeMessages.Store(p2.BHost.ID().String(), *out)
		require.NoError(t, stream.Close())
	})

	p3.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
		defer wg.Done()
		out := new(primitives.SSZUint64)
		require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, out))
		goodbyeMessages.Store(p3.BHost.ID().String(), *out)
		require.NoError(t, stream.Close())
	})

	connectedPeers := r.cfg.p2p.Peers().Connected()
	t.Logf("Connected peers before Stop: %d", len(connectedPeers))
	assert.Equal(t, 2, len(connectedPeers), "Expected 2 connected peers")

	err = r.Stop()
	assert.NoError(t, err)

	// Wait for goodbye messages
	if util.WaitTimeout(&wg, 15*time.Second) {
		t.Fatal("Did not receive goodbye messages within timeout")
	}

	// Verify correct goodbye codes were sent
	msg2, ok := goodbyeMessages.Load(p2.BHost.ID().String())
	assert.Equal(t, true, ok, "Expected goodbye message to peer 2")
	assert.Equal(t, p2ptypes.GoodbyeCodeClientShutdown, msg2)

	msg3, ok := goodbyeMessages.Load(p3.BHost.ID().String())
	assert.Equal(t, true, ok, "Expected goodbye message to peer 3")
	assert.Equal(t, p2ptypes.GoodbyeCodeClientShutdown, msg3)
}

func TestService_Stop_TimeoutHandling(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)

	p1.Peers().Add(nil, p2.BHost.ID(), p2.BHost.Addrs()[0], network.DirOutbound)
	p1.Peers().SetConnectionState(p2.BHost.ID(), peers.Connected)

	d := dbTest.SetupDB(t)
	chain := &mockChain.ChainService{Genesis: time.Now(), ValidatorsRoot: [32]byte{}}
	ctx, cancel := context.WithCancel(context.Background())

	r := &Service{
		cfg: &config{
			beaconDB: d,
			p2p:      p1,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
		},
		ctx:         ctx,
		cancel:      cancel,
		rateLimiter: newRateLimiter(p1),
	}

	// Initialize context map for RPC
	ctxMap, err := ContextByteVersionsForValRoot(chain.ValidatorsRoot)
	require.NoError(t, err)
	r.ctxMap = ctxMap

	// Setup rate limiter for goodbye topic
	pcl := protocol.ID("/eth2/beacon_chain/req/goodbye/1/ssz_snappy")
	topic := string(pcl)
	r.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(1, 1, time.Second, false)

	// Don't set up stream handler on p2 to simulate unresponsive peer

	// Verify peers are connected before stopping
	connectedPeers := r.cfg.p2p.Peers().Connected()
	t.Logf("Connected peers before Stop: %d", len(connectedPeers))

	start := time.Now()
	err = r.Stop()
	duration := time.Since(start)

	t.Logf("Stop completed in %v", duration)

	// Stop should complete successfully even when peers don't respond
	assert.NoError(t, err)
	// Should not hang - completes quickly when goodbye fails
	assert.Equal(t, true, duration < 5*time.Second, "Stop() should not hang when peer is unresponsive")
	// Test passes - the timeout behavior is working correctly, goodbye attempts fail quickly
}

func TestService_Stop_ConcurrentGoodbyeMessages(t *testing.T) {
	// Test that goodbye messages are sent concurrently, not sequentially
	const numPeers = 10

	p1 := p2ptest.NewTestP2P(t)
	testPeers := make([]*p2ptest.TestP2P, numPeers)

	// Create and connect multiple peers
	for i := 0; i < numPeers; i++ {
		testPeers[i] = p2ptest.NewTestP2P(t)
		p1.Connect(testPeers[i])
		// Register peer in the peer status
		p1.Peers().Add(nil, testPeers[i].BHost.ID(), testPeers[i].BHost.Addrs()[0], network.DirOutbound)
		p1.Peers().SetConnectionState(testPeers[i].BHost.ID(), peers.Connected)
	}

	d := dbTest.SetupDB(t)
	chain := &mockChain.ChainService{Genesis: time.Now(), ValidatorsRoot: [32]byte{}}
	ctx, cancel := context.WithCancel(context.Background())

	r := &Service{
		cfg: &config{
			beaconDB: d,
			p2p:      p1,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
		},
		ctx:         ctx,
		cancel:      cancel,
		rateLimiter: newRateLimiter(p1),
	}

	// Initialize context map for RPC
	ctxMap, err := ContextByteVersionsForValRoot(chain.ValidatorsRoot)
	require.NoError(t, err)
	r.ctxMap = ctxMap

	// Setup rate limiter for goodbye topic
	pcl := protocol.ID("/eth2/beacon_chain/req/goodbye/1/ssz_snappy")
	topic := string(pcl)
	r.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(1, 1, time.Second, false)

	// Each peer will have artificial delay in processing goodbye
	var wg sync.WaitGroup
	wg.Add(numPeers)

	for i := 0; i < numPeers; i++ {
		idx := i // capture loop variable
		testPeers[idx].BHost.SetStreamHandler(pcl, func(stream network.Stream) {
			defer wg.Done()
			time.Sleep(100 * time.Millisecond) // Artificial delay
			out := new(primitives.SSZUint64)
			require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, out))
			require.NoError(t, stream.Close())
		})
	}

	start := time.Now()
	err = r.Stop()
	duration := time.Since(start)

	// If messages were sent sequentially, it would take numPeers * 100ms = 1 second
	// If concurrent, should be ~100ms
	assert.NoError(t, err)
	assert.Equal(t, true, duration < 500*time.Millisecond, "Goodbye messages should be sent concurrently")

	require.Equal(t, false, util.WaitTimeout(&wg, 2*time.Second))
}
