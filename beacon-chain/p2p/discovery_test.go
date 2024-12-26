package p2p

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	mathRand "math/rand"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prysmaticlabs/go-bitfield"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers/peerdata"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers/scorers"
	testp2p "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/wrapper"
	leakybucket "github.com/prysmaticlabs/prysm/v5/container/leaky-bucket"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v5/crypto/ecdsa"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	prysmNetwork "github.com/prysmaticlabs/prysm/v5/network"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

var discoveryWaitTime = 1 * time.Second

func createAddrAndPrivKey(t *testing.T) (net.IP, *ecdsa.PrivateKey) {
	ip, err := prysmNetwork.ExternalIPv4()
	require.NoError(t, err, "Could not get ip")
	ipAddr := net.ParseIP(ip)
	temp := t.TempDir()
	randNum := mathRand.Int()
	tempPath := path.Join(temp, strconv.Itoa(randNum))
	require.NoError(t, os.Mkdir(tempPath, 0700))
	pkey, err := privKey(&Config{DataDir: tempPath})
	require.NoError(t, err, "Could not get private key")
	return ipAddr, pkey
}

func TestCreateListener(t *testing.T) {
	port := 1024
	ipAddr, pkey := createAddrAndPrivKey(t)
	s := &Service{
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		cfg:                   &Config{UDPPort: uint(port)},
	}
	listener, err := s.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer listener.Close()

	assert.Equal(t, true, listener.Self().IP().Equal(ipAddr), "IP address is not the expected type")
	assert.Equal(t, port, listener.Self().UDP(), "Incorrect port number")

	pubkey := listener.Self().Pubkey()
	XisSame := pkey.PublicKey.X.Cmp(pubkey.X) == 0
	YisSame := pkey.PublicKey.Y.Cmp(pubkey.Y) == 0

	if !(XisSame && YisSame) {
		t.Error("Pubkey is different from what was used to create the listener")
	}
}

func TestStartDiscV5_DiscoverAllPeers(t *testing.T) {
	port := 2000
	ipAddr, pkey := createAddrAndPrivKey(t)
	genesisTime := time.Now()
	genesisValidatorsRoot := make([]byte, 32)
	s := &Service{
		cfg:                   &Config{UDPPort: uint(port)},
		genesisTime:           genesisTime,
		genesisValidatorsRoot: genesisValidatorsRoot,
	}
	bootListener, err := s.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer bootListener.Close()

	bootNode := bootListener.Self()

	var listeners []*listenerWrapper
	for i := 1; i <= 5; i++ {
		port = 3000 + i
		cfg := &Config{
			Discv5BootStrapAddrs: []string{bootNode.String()},
			UDPPort:              uint(port),
		}
		ipAddr, pkey := createAddrAndPrivKey(t)
		s = &Service{
			cfg:                   cfg,
			genesisTime:           genesisTime,
			genesisValidatorsRoot: genesisValidatorsRoot,
		}
		listener, err := s.startDiscoveryV5(ipAddr, pkey)
		assert.NoError(t, err, "Could not start discovery for node")
		listeners = append(listeners, listener)
	}
	defer func() {
		// Close down all peers.
		for _, listener := range listeners {
			listener.Close()
		}
	}()

	// Wait for the nodes to have their local routing tables to be populated with the other nodes
	time.Sleep(discoveryWaitTime)

	lastListener := listeners[len(listeners)-1]
	nodes := lastListener.Lookup(bootNode.ID())
	if len(nodes) < 4 {
		t.Errorf("The node's local table doesn't have the expected number of nodes. "+
			"Expected more than or equal to %d but got %d", 4, len(nodes))
	}
}

func TestCreateLocalNode(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.Eip7594ForkEpoch = 1
	params.OverrideBeaconConfig(cfg)
	testCases := []struct {
		name          string
		cfg           *Config
		expectedError bool
	}{
		{
			name:          "valid config",
			cfg:           nil,
			expectedError: false,
		},
		{
			name:          "invalid host address",
			cfg:           &Config{HostAddress: "invalid"},
			expectedError: true,
		},
		{
			name:          "valid host address",
			cfg:           &Config{HostAddress: "192.168.0.1"},
			expectedError: false,
		},
		{
			name:          "invalid host DNS",
			cfg:           &Config{HostDNS: "invalid"},
			expectedError: true,
		},
		{
			name:          "valid host DNS",
			cfg:           &Config{HostDNS: "www.google.com"},
			expectedError: false,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Define ports.
			const (
				udpPort  = 2000
				tcpPort  = 3000
				quicPort = 3000
			)

			// Create a private key.
			address, privKey := createAddrAndPrivKey(t)

			// Create a service.
			service := &Service{
				genesisTime:           time.Now(),
				genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
				cfg:                   tt.cfg,
			}

			localNode, err := service.createLocalNode(privKey, address, udpPort, tcpPort, quicPort)
			if tt.expectedError {
				require.NotNil(t, err)
				return
			}

			require.NoError(t, err)

			expectedAddress := address
			if tt.cfg != nil && tt.cfg.HostAddress != "" {
				expectedAddress = net.ParseIP(tt.cfg.HostAddress)
			}

			// Check IP.
			// IP is not checked int case of DNS, since it can be resolved to different IPs.
			if tt.cfg == nil || tt.cfg.HostDNS == "" {
				ip := new(net.IP)
				require.NoError(t, localNode.Node().Record().Load(enr.WithEntry("ip", ip)))
				require.Equal(t, true, ip.Equal(expectedAddress))
				require.Equal(t, true, localNode.Node().IP().Equal(expectedAddress))
			}

			// Check UDP.
			udp := new(uint16)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry("udp", udp)))
			require.Equal(t, udpPort, localNode.Node().UDP())

			// Check TCP.
			tcp := new(uint16)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry("tcp", tcp)))
			require.Equal(t, tcpPort, localNode.Node().TCP())

			// Check fork is set.
			fork := new([]byte)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry(eth2ENRKey, fork)))
			require.NotEmpty(t, *fork)

			// Check att subnets.
			attSubnets := new([]byte)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry(attSubnetEnrKey, attSubnets)))
			require.DeepSSZEqual(t, []byte{0, 0, 0, 0, 0, 0, 0, 0}, *attSubnets)

			// Check sync committees subnets.
			syncSubnets := new([]byte)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry(syncCommsSubnetEnrKey, syncSubnets)))
			require.DeepSSZEqual(t, []byte{0}, *syncSubnets)
		})
	}
}

func TestRebootDiscoveryListener(t *testing.T) {
	port := 1024
	ipAddr, pkey := createAddrAndPrivKey(t)
	s := &Service{
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		cfg:                   &Config{UDPPort: uint(port)},
	}
	createListener := func() (*discover.UDPv5, error) {
		return s.createListener(ipAddr, pkey)
	}
	listener, err := newListener(createListener)
	require.NoError(t, err)
	currentPubkey := listener.Self().Pubkey()
	currentID := listener.Self().ID()
	currentPort := listener.Self().UDP()
	currentAddr := listener.Self().IP()

	assert.NoError(t, listener.RebootListener())

	newPubkey := listener.Self().Pubkey()
	newID := listener.Self().ID()
	newPort := listener.Self().UDP()
	newAddr := listener.Self().IP()

	assert.Equal(t, true, currentPubkey.Equal(newPubkey))
	assert.Equal(t, currentID, newID)
	assert.Equal(t, currentPort, newPort)
	assert.Equal(t, currentAddr.String(), newAddr.String())
}

func TestMultiAddrsConversion_InvalidIPAddr(t *testing.T) {
	addr := net.ParseIP("invalidIP")
	_, pkey := createAddrAndPrivKey(t)
	s := &Service{
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
	}
	node, err := s.createLocalNode(pkey, addr, 0, 0, 0)
	require.NoError(t, err)
	multiAddr := convertToMultiAddr([]*enode.Node{node.Node()})
	assert.Equal(t, 0, len(multiAddr), "Invalid ip address converted successfully")
}

func TestMultiAddrConversion_OK(t *testing.T) {
	hook := logTest.NewGlobal()
	ipAddr, pkey := createAddrAndPrivKey(t)
	s := &Service{
		cfg: &Config{
			UDPPort:  2000,
			TCPPort:  3000,
			QUICPort: 3000,
		},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
	}
	listener, err := s.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer listener.Close()

	_ = convertToMultiAddr([]*enode.Node{listener.Self()})
	require.LogsDoNotContain(t, hook, "Node doesn't have an ip4 address")
	require.LogsDoNotContain(t, hook, "Invalid port, the tcp port of the node is a reserved port")
	require.LogsDoNotContain(t, hook, "Could not get multiaddr")
}

func TestStaticPeering_PeersAreAdded(t *testing.T) {
	cs := startup.NewClockSynchronizer()
	cfg := &Config{
		MaxPeers:    30,
		ClockWaiter: cs,
	}
	port := 6000
	var staticPeers []string
	var hosts []host.Host
	// setup other nodes
	for i := 1; i <= 5; i++ {
		h, _, ipaddr := createHost(t, port+i)
		staticPeers = append(staticPeers, fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", ipaddr, port+i, h.ID()))
		hosts = append(hosts, h)
	}

	defer func() {
		for _, h := range hosts {
			if err := h.Close(); err != nil {
				t.Log(err)
			}
		}
	}()

	cfg.TCPPort = 14500
	cfg.UDPPort = 14501
	cfg.StaticPeers = staticPeers
	cfg.StateNotifier = &mock.MockStateNotifier{}
	cfg.NoDiscovery = true
	s, err := NewService(context.Background(), cfg)
	require.NoError(t, err)

	exitRoutine := make(chan bool)
	go func() {
		s.Start()
		<-exitRoutine
	}()
	time.Sleep(50 * time.Millisecond)
	var vr [32]byte
	require.NoError(t, cs.SetClock(startup.NewClock(time.Now(), vr)))
	time.Sleep(4 * time.Second)
	ps := s.host.Network().Peers()
	assert.Equal(t, 5, len(ps), "Not all peers added to peerstore")
	require.NoError(t, s.Stop())
	exitRoutine <- true
}

func TestHostIsResolved(t *testing.T) {
	// As defined in RFC 2606 , example.org is a
	// reserved example domain name.
	exampleHost := "example.org"
	exampleIP := "93.184.215.14"

	s := &Service{
		cfg: &Config{
			HostDNS: exampleHost,
		},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
	}
	ip, key := createAddrAndPrivKey(t)
	list, err := s.createListener(ip, key)
	require.NoError(t, err)

	newIP := list.Self().IP()
	assert.Equal(t, exampleIP, newIP.String(), "Did not resolve to expected IP")
}

func TestInboundPeerLimit(t *testing.T) {
	fakePeer := testp2p.NewTestP2P(t)
	s := &Service{
		cfg:       &Config{MaxPeers: 30},
		ipLimiter: leakybucket.NewCollector(ipLimit, ipBurst, 1*time.Second, false),
		peers: peers.NewStatus(context.Background(), &peers.StatusConfig{
			PeerLimit:    30,
			ScorerParams: &scorers.Config{},
		}),
		host: fakePeer.BHost,
	}

	for i := 0; i < 30; i++ {
		_ = addPeer(t, s.peers, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED), false)
	}

	require.Equal(t, true, s.isPeerAtLimit(false), "not at limit for outbound peers")
	require.Equal(t, false, s.isPeerAtLimit(true), "at limit for inbound peers")

	for i := 0; i < highWatermarkBuffer; i++ {
		_ = addPeer(t, s.peers, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED), false)
	}

	require.Equal(t, true, s.isPeerAtLimit(true), "not at limit for inbound peers")
}

func TestOutboundPeerThreshold(t *testing.T) {
	fakePeer := testp2p.NewTestP2P(t)
	s := &Service{
		cfg:       &Config{MaxPeers: 30},
		ipLimiter: leakybucket.NewCollector(ipLimit, ipBurst, 1*time.Second, false),
		peers: peers.NewStatus(context.Background(), &peers.StatusConfig{
			PeerLimit:    30,
			ScorerParams: &scorers.Config{},
		}),
		host: fakePeer.BHost,
	}

	for i := 0; i < 2; i++ {
		_ = addPeer(t, s.peers, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED), true)
	}

	require.Equal(t, true, s.isBelowOutboundPeerThreshold(), "not at outbound peer threshold")

	for i := 0; i < 3; i++ {
		_ = addPeer(t, s.peers, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED), true)
	}

	require.Equal(t, false, s.isBelowOutboundPeerThreshold(), "still at outbound peer threshold")
}

func TestUDPMultiAddress(t *testing.T) {
	port := 6500
	ipAddr, pkey := createAddrAndPrivKey(t)
	genesisTime := time.Now()
	genesisValidatorsRoot := make([]byte, 32)
	s := &Service{
		cfg:                   &Config{UDPPort: uint(port)},
		genesisTime:           genesisTime,
		genesisValidatorsRoot: genesisValidatorsRoot,
	}

	createListener := func() (*discover.UDPv5, error) {
		return s.createListener(ipAddr, pkey)
	}
	listener, err := newListener(createListener)
	require.NoError(t, err)
	defer listener.Close()
	s.dv5Listener = listener

	multiAddresses, err := s.DiscoveryAddresses()
	require.NoError(t, err)
	require.Equal(t, true, len(multiAddresses) > 0)
	assert.Equal(t, true, strings.Contains(multiAddresses[0].String(), fmt.Sprintf("%d", port)))
	assert.Equal(t, true, strings.Contains(multiAddresses[0].String(), "udp"))
}

func TestMultipleDiscoveryAddresses(t *testing.T) {
	db, err := enode.OpenDB(t.TempDir())
	require.NoError(t, err)
	_, key := createAddrAndPrivKey(t)
	node := enode.NewLocalNode(db, key)
	node.Set(enr.IPv4{127, 0, 0, 1})
	node.Set(enr.IPv6{0x20, 0x01, 0x48, 0x60, 0, 0, 0x20, 0x01, 0, 0, 0, 0, 0, 0, 0x00, 0x68})
	s := &Service{dv5Listener: mockListener{localNode: node}}

	multiAddresses, err := s.DiscoveryAddresses()
	require.NoError(t, err)
	require.Equal(t, 2, len(multiAddresses))
	ipv4Found, ipv6Found := false, false
	for _, address := range multiAddresses {
		s := address.String()
		if strings.Contains(s, "ip4") {
			ipv4Found = true
		} else if strings.Contains(s, "ip6") {
			ipv6Found = true
		}
	}
	assert.Equal(t, true, ipv4Found, "IPv4 discovery address not found")
	assert.Equal(t, true, ipv6Found, "IPv6 discovery address not found")
}

func TestCorrectUDPVersion(t *testing.T) {
	assert.Equal(t, udp4, udpVersionFromIP(net.IPv4zero), "incorrect network version")
	assert.Equal(t, udp6, udpVersionFromIP(net.IPv6zero), "incorrect network version")
	assert.Equal(t, udp4, udpVersionFromIP(net.IP{200, 20, 12, 255}), "incorrect network version")
	assert.Equal(t, udp6, udpVersionFromIP(net.IP{22, 23, 24, 251, 17, 18, 0, 0, 0, 0, 12, 14, 212, 213, 16, 22}), "incorrect network version")
	// v4 in v6
	assert.Equal(t, udp4, udpVersionFromIP(net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 212, 213, 16, 22}), "incorrect network version")
}

// addPeer is a helper to add a peer with a given connection state)
func addPeer(t *testing.T, p *peers.Status, state peerdata.ConnectionState, outbound bool) peer.ID {
	// Set up some peers with different states
	mhBytes := []byte{0x11, 0x04}
	idBytes := make([]byte, 4)
	_, err := rand.Read(idBytes)
	require.NoError(t, err)
	mhBytes = append(mhBytes, idBytes...)
	id, err := peer.IDFromBytes(mhBytes)
	require.NoError(t, err)
	dir := network.DirInbound
	if outbound {
		dir = network.DirOutbound
	}
	p.Add(new(enr.Record), id, nil, dir)
	p.SetConnectionState(id, state)
	p.SetMetadata(id, wrapper.WrappedMetadataV0(&ethpb.MetaDataV0{
		SeqNumber: 0,
		Attnets:   bitfield.NewBitvector64(),
	}))
	return id
}

func createAndConnectPeer(t *testing.T, p2pService *testp2p.TestP2P, offset int) {
	// Create the private key.
	privateKeyBytes := make([]byte, 32)
	for i := 0; i < 32; i++ {
		privateKeyBytes[i] = byte(offset + i)
	}

	privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes)
	require.NoError(t, err)

	// Create the peer.
	peer := testp2p.NewTestP2P(t, libp2p.Identity(privateKey))

	// Add the peer and connect it.
	p2pService.Peers().Add(&enr.Record{}, peer.PeerID(), nil, network.DirOutbound)
	p2pService.Peers().SetConnectionState(peer.PeerID(), peers.Connected)
	p2pService.Connect(peer)
}

// Define the ping count.
var actualPingCount int

type check struct {
	pingCount              int
	metadataSequenceNumber uint64
	attestationSubnets     []uint64
	syncSubnets            []uint64
	custodySubnetCount     *uint64
}

func checkPingCountCacheMetadataRecord(
	t *testing.T,
	service *Service,
	expected check,
) {
	// Check the ping count.
	require.Equal(t, expected.pingCount, actualPingCount)

	// Check the attestation subnets in the cache.
	actualAttestationSubnets := cache.SubnetIDs.GetAllSubnets()
	require.DeepSSZEqual(t, expected.attestationSubnets, actualAttestationSubnets)

	// Check the metadata sequence number.
	actualMetadataSequenceNumber := service.metaData.SequenceNumber()
	require.Equal(t, expected.metadataSequenceNumber, actualMetadataSequenceNumber)

	// Compute expected attestation subnets bits.
	expectedBitV := bitfield.NewBitvector64()
	exists := false

	for _, idx := range expected.attestationSubnets {
		exists = true
		expectedBitV.SetBitAt(idx, true)
	}

	// Check attnets in ENR.
	var actualBitVENR bitfield.Bitvector64
	err := service.dv5Listener.LocalNode().Node().Record().Load(enr.WithEntry(attSubnetEnrKey, &actualBitVENR))
	require.NoError(t, err)
	require.DeepSSZEqual(t, expectedBitV, actualBitVENR)

	// Check attnets in metadata.
	if !exists {
		expectedBitV = nil
	}

	actualBitVMetadata := service.metaData.AttnetsBitfield()
	require.DeepSSZEqual(t, expectedBitV, actualBitVMetadata)

	if expected.syncSubnets != nil {
		// Compute expected sync subnets bits.
		expectedBitS := bitfield.NewBitvector4()
		exists = false

		for _, idx := range expected.syncSubnets {
			exists = true
			expectedBitS.SetBitAt(idx, true)
		}

		// Check syncnets in ENR.
		var actualBitSENR bitfield.Bitvector4
		err := service.dv5Listener.LocalNode().Node().Record().Load(enr.WithEntry(syncCommsSubnetEnrKey, &actualBitSENR))
		require.NoError(t, err)
		require.DeepSSZEqual(t, expectedBitS, actualBitSENR)

		// Check syncnets in metadata.
		if !exists {
			expectedBitS = nil
		}

		actualBitSMetadata := service.metaData.SyncnetsBitfield()
		require.DeepSSZEqual(t, expectedBitS, actualBitSMetadata)
	}
}

func TestRefreshPersistentSubnets(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	// Clean up caches after usage.
	defer cache.SubnetIDs.EmptyAllCaches()
	defer cache.SyncSubnetIDs.EmptyAllCaches()

	const (
		altairForkEpoch  = 5
		eip7594ForkEpoch = 10
	)

	// Set up epochs.
	defaultCfg := params.BeaconConfig()
	cfg := defaultCfg.Copy()
	cfg.AltairForkEpoch = altairForkEpoch
	cfg.Eip7594ForkEpoch = eip7594ForkEpoch
	params.OverrideBeaconConfig(cfg)

	// Compute the number of seconds per epoch.
	secondsPerSlot := params.BeaconConfig().SecondsPerSlot
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	secondsPerEpoch := secondsPerSlot * uint64(slotsPerEpoch)

	testCases := []struct {
		name              string
		epochSinceGenesis uint64
		checks            []check
	}{
		{
			name:              "Phase0",
			epochSinceGenesis: 0,
			checks: []check{
				{
					pingCount:              0,
					metadataSequenceNumber: 0,
					attestationSubnets:     []uint64{},
				},
				{
					pingCount:              1,
					metadataSequenceNumber: 1,
					attestationSubnets:     []uint64{40, 41},
				},
				{
					pingCount:              1,
					metadataSequenceNumber: 1,
					attestationSubnets:     []uint64{40, 41},
				},
				{
					pingCount:              1,
					metadataSequenceNumber: 1,
					attestationSubnets:     []uint64{40, 41},
				},
			},
		},
		{
			name:              "Altair",
			epochSinceGenesis: altairForkEpoch,
			checks: []check{
				{
					pingCount:              0,
					metadataSequenceNumber: 0,
					attestationSubnets:     []uint64{},
					syncSubnets:            nil,
				},
				{
					pingCount:              1,
					metadataSequenceNumber: 1,
					attestationSubnets:     []uint64{40, 41},
					syncSubnets:            nil,
				},
				{
					pingCount:              2,
					metadataSequenceNumber: 2,
					attestationSubnets:     []uint64{40, 41},
					syncSubnets:            []uint64{1, 2},
				},
				{
					pingCount:              2,
					metadataSequenceNumber: 2,
					attestationSubnets:     []uint64{40, 41},
					syncSubnets:            []uint64{1, 2},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const peerOffset = 1

			// Initialize the ping count.
			actualPingCount = 0

			// Create the private key.
			privateKeyBytes := make([]byte, 32)
			for i := 0; i < 32; i++ {
				privateKeyBytes[i] = byte(i)
			}

			unmarshalledPrivateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes)
			require.NoError(t, err)

			privateKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(unmarshalledPrivateKey)
			require.NoError(t, err)

			// Create a p2p service.
			p2p := testp2p.NewTestP2P(t)

			// Create and connect a peer.
			createAndConnectPeer(t, p2p, peerOffset)

			// Create a service.
			service := &Service{
				pingMethod: func(_ context.Context, _ peer.ID) error {
					actualPingCount++
					return nil
				},
				cfg:                   &Config{UDPPort: 2000},
				peers:                 p2p.Peers(),
				genesisTime:           time.Now().Add(-time.Duration(tc.epochSinceGenesis*secondsPerEpoch) * time.Second),
				genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
			}

			// Set the listener and the metadata.
			createListener := func() (*discover.UDPv5, error) {
				return service.createListener(nil, privateKey)
			}

			listener, err := newListener(createListener)
			require.NoError(t, err)

			service.dv5Listener = listener
			service.metaData = wrapper.WrappedMetadataV0(new(ethpb.MetaDataV0))

			// Run a check.
			checkPingCountCacheMetadataRecord(t, service, tc.checks[0])

			// Refresh the persistent subnets.
			service.RefreshPersistentSubnets()
			time.Sleep(10 * time.Millisecond)

			// Run a check.
			checkPingCountCacheMetadataRecord(t, service, tc.checks[1])

			// Add a sync committee subnet.
			cache.SyncSubnetIDs.AddSyncCommitteeSubnets([]byte{'a'}, altairForkEpoch, []uint64{1, 2}, 1*time.Hour)

			// Refresh the persistent subnets.
			service.RefreshPersistentSubnets()
			time.Sleep(10 * time.Millisecond)

			// Run a check.
			checkPingCountCacheMetadataRecord(t, service, tc.checks[2])

			// Refresh the persistent subnets.
			service.RefreshPersistentSubnets()
			time.Sleep(10 * time.Millisecond)

			// Run a check.
			checkPingCountCacheMetadataRecord(t, service, tc.checks[3])

			// Clean the test.
			service.dv5Listener.Close()
			cache.SubnetIDs.EmptyAllCaches()
			cache.SyncSubnetIDs.EmptyAllCaches()
		})
	}

	// Reset the config.
	params.OverrideBeaconConfig(defaultCfg)
}
