package p2p

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/scorers"
	testp2p "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	noise "github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/multiformats/go-multiaddr"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

const testPingInterval = 100 * time.Millisecond

func createHost(t *testing.T, port uint) (host.Host, *ecdsa.PrivateKey, net.IP) {
	_, pkey := createAddrAndPrivKey(t)
	ipAddr := net.ParseIP("127.0.0.1")
	listen, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ipAddr, port))
	require.NoError(t, err, "Failed to p2p listen")
	h, err := libp2p.New([]libp2p.Option{privKeyOption(pkey), libp2p.ListenAddrs(listen), libp2p.Security(noise.ID, noise.New)}...)
	require.NoError(t, err)
	return h, pkey, ipAddr
}

func TestService_Stop_SetsStartedToFalse(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	s, err := NewService(t.Context(), &Config{StateNotifier: &mock.MockStateNotifier{}, DB: testDB.SetupDB(t)})
	require.NoError(t, err)
	s.started = true
	s.dv5Listener = testp2p.NewMockListener(nil, nil)
	assert.NoError(t, s.Stop())
	assert.Equal(t, false, s.started)
}

func TestService_Stop_DontPanicIfDv5ListenerIsNotInited(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	s, err := NewService(t.Context(), &Config{StateNotifier: &mock.MockStateNotifier{}, DB: testDB.SetupDB(t)})
	require.NoError(t, err)
	assert.NoError(t, s.Stop())
}

func TestService_Start_OnlyStartsOnce(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	hook := logTest.NewGlobal()

	cs := startup.NewClockSynchronizer()
	cfg := &Config{
		UDPPort:     2000,
		TCPPort:     3000,
		QUICPort:    3000,
		ClockWaiter: cs,
		DB:          testDB.SetupDB(t),
	}
	s, err := NewService(t.Context(), cfg)
	require.NoError(t, err)
	s.dv5Listener = testp2p.NewMockListener(nil, nil)
	s.custodyInfo = &custodyInfo{}
	exitRoutine := make(chan bool)
	go func() {
		s.Start()
		<-exitRoutine
	}()
	var vr [32]byte
	require.NoError(t, cs.SetClock(startup.NewClock(time.Now(), vr)))
	time.Sleep(time.Second * 2)
	assert.Equal(t, true, s.started, "Expected service to be started")
	s.Start()
	require.LogsContain(t, hook, "Attempted to start p2p service when it was already started")
	require.NoError(t, s.Stop())
	exitRoutine <- true
}

func TestService_Status_NotRunning(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	s := &Service{started: false}
	s.dv5Listener = testp2p.NewMockListener(nil, nil)
	assert.ErrorContains(t, "not running", s.Status(), "Status returned wrong error")
}

func TestService_Status_NoGenesisTimeSet(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	s := &Service{started: true}
	s.dv5Listener = testp2p.NewMockListener(nil, nil)
	assert.ErrorContains(t, "no genesis time set", s.Status(), "Status returned wrong error")

	s.genesisTime = time.Now()

	assert.NoError(t, s.Status(), "Status returned error")
}

func TestService_Start_NoDiscoverFlag(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	cs := startup.NewClockSynchronizer()
	cfg := &Config{
		UDPPort:       2000,
		TCPPort:       3000,
		QUICPort:      3000,
		StateNotifier: &mock.MockStateNotifier{},
		NoDiscovery:   true, // <-- no s.dv5Listener is created
		ClockWaiter:   cs,
		DB:            testDB.SetupDB(t),
	}
	s, err := NewService(t.Context(), cfg)
	require.NoError(t, err)

	// required params to addForkEntry in s.forkWatcher
	s.genesisTime = time.Now()
	beaconCfg := params.BeaconConfig().Copy()
	beaconCfg.AltairForkEpoch = 0
	beaconCfg.BellatrixForkEpoch = 0
	beaconCfg.CapellaForkEpoch = 0
	beaconCfg.SecondsPerSlot = 1
	params.OverrideBeaconConfig(beaconCfg)

	exitRoutine := make(chan bool)
	go func() {
		s.Start()
		<-exitRoutine
	}()

	var vr [32]byte
	require.NoError(t, cs.SetClock(startup.NewClock(time.Now(), vr)))

	time.Sleep(time.Second * 2)

	exitRoutine <- true
}

func TestListenForNewNodes(t *testing.T) {
	const (
		port              = uint(2000)
		testPollingPeriod = 1 * time.Second
		peerCount         = 5
	)

	params.SetupTestConfigCleanup(t)
	db := testDB.SetupDB(t)

	// Setup bootnode.
	cfg := &Config{
		StateNotifier:        &mock.MockStateNotifier{},
		PingInterval:         testPingInterval,
		DisableLivenessCheck: true,
		UDPPort:              port,
		DB:                   db,
	}

	_, pkey := createAddrAndPrivKey(t)
	ipAddr := net.ParseIP("127.0.0.1")
	genesisTime := prysmTime.Now()
	var gvr [fieldparams.RootLength]byte

	s := &Service{
		cfg:                   cfg,
		genesisTime:           genesisTime,
		genesisValidatorsRoot: gvr[:],
		custodyInfo:           &custodyInfo{},
	}

	bootListener, err := s.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer bootListener.Close()

	// Allow bootnode's table to have its initial refresh. This allows
	// inbound nodes to be added in.
	time.Sleep(5 * time.Second)

	// Use shorter period for testing.
	currentPeriod := pollingPeriod
	pollingPeriod = testPollingPeriod
	defer func() {
		pollingPeriod = currentPeriod
	}()

	bootNode := bootListener.Self()

	// Setup other nodes.
	cs := startup.NewClockSynchronizer()
	listeners := make([]*listenerWrapper, 0, peerCount)
	hosts := make([]host.Host, 0, peerCount)

	for i := uint(1); i <= peerCount; i++ {
		cfg = &Config{
			Discv5BootStrapAddrs: []string{bootNode.String()},
			PingInterval:         testPingInterval,
			DisableLivenessCheck: true,
			MaxPeers:             peerCount,
			ClockWaiter:          cs,
			UDPPort:              port + i,
			TCPPort:              port + i,
			DB:                   db,
		}

		h, pkey, ipAddr := createHost(t, port+i)

		s := &Service{
			cfg:                   cfg,
			genesisTime:           genesisTime,
			genesisValidatorsRoot: gvr[:],
			custodyInfo:           &custodyInfo{},
		}

		listener, err := s.startDiscoveryV5(ipAddr, pkey)
		require.NoError(t, err, "Could not start discovery for node")

		listeners = append(listeners, listener)
		hosts = append(hosts, h)
	}
	defer func() {
		// Close down all peers.
		for _, listener := range listeners {
			listener.Close()
		}
	}()

	// close peers upon exit of test
	defer func() {
		for _, h := range hosts {
			if err := h.Close(); err != nil {
				t.Log(err)
			}
		}
	}()

	cfg.UDPPort = 14000
	cfg.TCPPort = 14001

	s, err = NewService(t.Context(), cfg)
	require.NoError(t, err)
	s.custodyInfo = &custodyInfo{}

	go s.Start()

	err = cs.SetClock(startup.NewClock(genesisTime, gvr))
	require.NoError(t, err, "Could not set clock in service")

	actualPeerCount := len(s.host.Network().Peers())
	for range 40 {
		if actualPeerCount == peerCount {
			break
		}

		time.Sleep(100 * time.Millisecond)
		actualPeerCount = len(s.host.Network().Peers())
	}

	assert.Equal(t, peerCount, actualPeerCount, "Not all peers added to peerstore")

	err = s.Stop()
	require.NoError(t, err, "Failed to stop service")
}

func TestPeer_Disconnect(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	h1, _, _ := createHost(t, 5000)
	defer func() {
		if err := h1.Close(); err != nil {
			t.Log(err)
		}
	}()

	s := &Service{
		host: h1,
	}

	h2, _, ipaddr := createHost(t, 5001)
	defer func() {
		if err := h2.Close(); err != nil {
			t.Log(err)
		}
	}()

	h2Addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", ipaddr, 5001, h2.ID()))
	require.NoError(t, err)
	addrInfo, err := peer.AddrInfoFromP2pAddr(h2Addr)
	require.NoError(t, err)
	require.NoError(t, s.host.Connect(t.Context(), *addrInfo))
	assert.Equal(t, 1, len(s.host.Network().Peers()), "Invalid number of peers")
	assert.Equal(t, 1, len(s.host.Network().Conns()), "Invalid number of connections")
	require.NoError(t, s.Disconnect(h2.ID()))
	assert.Equal(t, 0, len(s.host.Network().Conns()), "Invalid number of connections")
}

func TestService_JoinLeaveTopic(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	gs := startup.NewClockSynchronizer()
	s, err := NewService(ctx, &Config{StateNotifier: &mock.MockStateNotifier{}, ClockWaiter: gs, DB: testDB.SetupDB(t)})
	require.NoError(t, err)

	fd := initializeStateWithForkDigest(ctx, t, gs)
	s.setAllForkDigests()
	s.awaitStateInitialized()

	assert.Equal(t, 0, len(s.joinedTopics))

	topic := fmt.Sprintf(AttestationSubnetTopicFormat, fd, 42) + "/" + encoder.ProtocolSuffixSSZSnappy
	topicHandle, err := s.JoinTopic(topic)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(s.joinedTopics))

	if topicHandle == nil {
		t.Fatal("topic is nil")
	}

	sub, err := topicHandle.Subscribe()
	assert.NoError(t, err)

	// Try leaving topic that has subscriptions.
	want := "cannot close topic: outstanding event handlers or subscriptions"
	assert.ErrorContains(t, want, s.LeaveTopic(topic))

	// After subscription is cancelled, leaving topic should not result in error.
	sub.Cancel()
	assert.NoError(t, s.LeaveTopic(topic))
}

// initializeStateWithForkDigest sets up the state feed initialized event and returns the fork
// digest associated with that genesis event.
func initializeStateWithForkDigest(_ context.Context, t *testing.T, gs startup.ClockSetter) [4]byte {
	gt := prysmTime.Now()
	gvr := params.BeaconConfig().GenesisValidatorsRoot
	clock := startup.NewClock(gt, gvr)
	require.NoError(t, gs.SetClock(clock))

	time.Sleep(50 * time.Millisecond) // wait for pubsub filter to initialize.

	return params.ForkDigest(clock.CurrentEpoch())
}

func TestService_connectWithPeer(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	tests := []struct {
		name    string
		peers   *peers.Status
		info    peer.AddrInfo
		wantErr string
	}{
		{
			name: "bad peer",
			peers: func() *peers.Status {
				ps := peers.NewStatus(t.Context(), &peers.StatusConfig{
					ScorerParams: &scorers.Config{},
				})
				for i := 0; i < 10; i++ {
					ps.Scorers().BadResponsesScorer().Increment("bad")
				}
				return ps
			}(),
			info:    peer.AddrInfo{ID: "bad"},
			wantErr: "bad peer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, _ := createHost(t, 34567)
			defer func() {
				if err := h.Close(); err != nil {
					t.Fatal(err)
				}
			}()
			ctx := t.Context()
			s := &Service{
				host:  h,
				peers: tt.peers,
			}
			err := s.connectWithPeer(ctx, tt.info)
			if len(tt.wantErr) > 0 {
				require.ErrorContains(t, tt.wantErr, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
