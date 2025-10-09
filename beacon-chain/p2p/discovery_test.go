package p2p

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	mathRand "math/rand"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/peerdata"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/scorers"
	testp2p "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/wrapper"
	leakybucket "github.com/OffchainLabs/prysm/v6/container/leaky-bucket"
	ecdsaprysm "github.com/OffchainLabs/prysm/v6/crypto/ecdsa"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	prysmNetwork "github.com/OffchainLabs/prysm/v6/network"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prysmaticlabs/go-bitfield"
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

// createTestNodeWithID creates a LocalNode for testing with deterministic private key
// This is needed for deduplication tests where we need the same node ID across different sequence numbers
func createTestNodeWithID(t *testing.T, id string) *enode.LocalNode {
	// Create a deterministic reader based on the ID for consistent key generation
	h := sha256.New()
	h.Write([]byte(id))
	seedBytes := h.Sum(nil)

	// Create a deterministic reader using the seed
	deterministicReader := bytes.NewReader(seedBytes)

	// Generate the private key using the same approach as the production code
	privKey, _, err := crypto.GenerateSecp256k1Key(deterministicReader)
	require.NoError(t, err)

	// Convert to ECDSA private key for enode usage
	ecdsaPrivKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(privKey)
	require.NoError(t, err)

	db, err := enode.OpenDB("")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	localNode := enode.NewLocalNode(db, ecdsaPrivKey)

	// Set basic properties
	localNode.SetStaticIP(net.ParseIP("127.0.0.1"))
	localNode.Set(enr.TCP(3000))
	localNode.Set(enr.UDP(3000))
	localNode.Set(enr.WithEntry(eth2EnrKey, make([]byte, 16)))

	return localNode
}

// createTestNodeRandom creates a LocalNode for testing using the existing createAddrAndPrivKey function
func createTestNodeRandom(t *testing.T) *enode.LocalNode {
	_, privKey := createAddrAndPrivKey(t)

	db, err := enode.OpenDB("")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	localNode := enode.NewLocalNode(db, privKey)

	// Set basic properties
	localNode.SetStaticIP(net.ParseIP("127.0.0.1"))
	localNode.Set(enr.TCP(3000))
	localNode.Set(enr.UDP(3000))
	localNode.Set(enr.WithEntry(eth2EnrKey, make([]byte, 16)))

	return localNode
}

// setNodeSeq updates a LocalNode to have the specified sequence number
func setNodeSeq(localNode *enode.LocalNode, seq uint64) {
	// Force set the sequence number - we need to update the record seq-1 times
	// because it starts at 1
	currentSeq := localNode.Node().Seq()
	for currentSeq < seq {
		localNode.Set(enr.WithEntry("dummy", currentSeq))
		currentSeq++
	}
}

// setNodeSubnets sets the attestation subnets for a LocalNode
func setNodeSubnets(localNode *enode.LocalNode, attSubnets []uint64) {
	if len(attSubnets) > 0 {
		bitV := bitfield.NewBitvector64()
		for _, subnet := range attSubnets {
			bitV.SetBitAt(subnet, true)
		}
		localNode.Set(enr.WithEntry(attSubnetEnrKey, &bitV))
	}
}

func TestCreateListener(t *testing.T) {
	port := 1024
	ipAddr, pkey := createAddrAndPrivKey(t)
	s := &Service{
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		cfg:                   &Config{UDPPort: uint(port)},
		custodyInfo:           &custodyInfo{},
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
		cfg:                   &Config{UDPPort: uint(port), PingInterval: testPingInterval, DisableLivenessCheck: true},
		genesisTime:           genesisTime,
		genesisValidatorsRoot: genesisValidatorsRoot,
		custodyInfo:           &custodyInfo{},
	}
	bootListener, err := s.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer bootListener.Close()

	// Allow bootnode's table to have its initial refresh. This allows
	// inbound nodes to be added in.
	time.Sleep(5 * time.Second)

	bootNode := bootListener.Self()

	var listeners []*listenerWrapper
	for i := 1; i <= 5; i++ {
		port = 3000 + i
		cfg := &Config{
			Discv5BootStrapAddrs: []string{bootNode.String()},
			UDPPort:              uint(port),
			PingInterval:         testPingInterval,
			DisableLivenessCheck: true,
		}
		ipAddr, pkey := createAddrAndPrivKey(t)
		s = &Service{
			cfg:                   cfg,
			genesisTime:           genesisTime,
			genesisValidatorsRoot: genesisValidatorsRoot,
			custodyInfo:           &custodyInfo{},
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

	// Set the fulu fork epoch to something other than the far future epoch.
	initFuluForkEpoch := params.BeaconConfig().FuluForkEpoch
	params.BeaconConfig().FuluForkEpoch = 42

	defer func() {
		params.BeaconConfig().FuluForkEpoch = initFuluForkEpoch
	}()

	testCases := []struct {
		name          string
		cfg           *Config
		expectedError bool
	}{
		{
			name:          "valid config",
			cfg:           &Config{},
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

			custodyRequirement := params.BeaconConfig().CustodyRequirement

			// Create a private key.
			address, privKey := createAddrAndPrivKey(t)

			// Create a service.
			service := &Service{
				genesisTime:           time.Now(),
				genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
				cfg:                   tt.cfg,
				ctx:                   t.Context(),
				custodyInfo:           &custodyInfo{groupCount: custodyRequirement},
				custodyInfoSet:        make(chan struct{}),
			}

			close(service.custodyInfoSet)

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
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry(eth2EnrKey, fork)))
			require.NotEmpty(t, *fork)

			// Check att subnets.
			attSubnets := new([]byte)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry(attSubnetEnrKey, attSubnets)))
			require.DeepSSZEqual(t, []byte{0, 0, 0, 0, 0, 0, 0, 0}, *attSubnets)

			// Check sync committees subnets.
			syncSubnets := new([]byte)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry(syncCommsSubnetEnrKey, syncSubnets)))
			require.DeepSSZEqual(t, []byte{0}, *syncSubnets)

			// Check cgc config.
			custodyGroupCount := new(uint64)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry(params.BeaconNetworkConfig().CustodyGroupCountKey, custodyGroupCount)))
			require.Equal(t, custodyRequirement, *custodyGroupCount)
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
		custodyInfo:           &custodyInfo{},
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
		cfg:                   &Config{},
		custodyInfo:           &custodyInfo{},
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
		custodyInfo:           &custodyInfo{},
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
	const port = uint(6000)
	cs := startup.NewClockSynchronizer()
	cfg := &Config{
		MaxPeers:    30,
		ClockWaiter: cs,
	}
	var staticPeers []string
	var hosts []host.Host
	// setup other nodes
	for i := uint(1); i <= 5; i++ {
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
	cfg.DB = testDB.SetupDB(t)

	s, err := NewService(t.Context(), cfg)
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
	host := "dns.google"
	ips := map[string]bool{
		"8.8.8.8":              true,
		"8.8.4.4":              true,
		"2001:4860:4860::8888": true,
		"2001:4860:4860::8844": true,
	}

	s := &Service{
		cfg: &Config{
			HostDNS: host,
		},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		custodyInfo:           &custodyInfo{},
	}
	ip, key := createAddrAndPrivKey(t)
	list, err := s.createListener(ip, key)
	require.NoError(t, err)

	newIP := list.Self().IP()
	assert.Equal(t, true, ips[newIP.String()], "Did not resolve to expected IP")
}

func TestInboundPeerLimit(t *testing.T) {
	fakePeer := testp2p.NewTestP2P(t)
	s := &Service{
		cfg:       &Config{MaxPeers: 30},
		ipLimiter: leakybucket.NewCollector(ipLimit, ipBurst, 1*time.Second, false),
		peers: peers.NewStatus(t.Context(), &peers.StatusConfig{
			PeerLimit:    30,
			ScorerParams: &scorers.Config{},
		}),
		host: fakePeer.BHost,
	}

	for i := 0; i < 30; i++ {
		_ = addPeer(t, s.peers, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED), false)
	}

	require.Equal(t, true, s.isPeerAtLimit(all), "not at limit for outbound peers")
	require.Equal(t, false, s.isPeerAtLimit(inbound), "at limit for inbound peers")

	for i := 0; i < highWatermarkBuffer; i++ {
		_ = addPeer(t, s.peers, peerdata.ConnectionState(ethpb.ConnectionState_CONNECTED), false)
	}

	require.Equal(t, true, s.isPeerAtLimit(inbound), "not at limit for inbound peers")
}

func TestOutboundPeerThreshold(t *testing.T) {
	fakePeer := testp2p.NewTestP2P(t)
	s := &Service{
		cfg:       &Config{MaxPeers: 30},
		ipLimiter: leakybucket.NewCollector(ipLimit, ipBurst, 1*time.Second, false),
		peers: peers.NewStatus(t.Context(), &peers.StatusConfig{
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
		custodyInfo:           &custodyInfo{},
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
	s := &Service{dv5Listener: testp2p.NewMockListener(node, nil)}

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

func TestDiscoveryV5_SeqNumber(t *testing.T) {
	db, err := enode.OpenDB(t.TempDir())
	require.NoError(t, err)
	_, key := createAddrAndPrivKey(t)
	node := enode.NewLocalNode(db, key)
	node.Set(enr.IPv4{127, 0, 0, 1})
	currentSeq := node.Seq()
	s := &Service{dv5Listener: testp2p.NewMockListener(node, nil)}
	_, err = s.DiscoveryAddresses()
	require.NoError(t, err)
	newSeq := node.Seq()
	require.Equal(t, currentSeq+1, newSeq) // node seq should increase when discovery starts

	// see that the keys changing, will change the node seq
	_, keyTwo := createAddrAndPrivKey(t)
	nodeTwo := enode.NewLocalNode(db, keyTwo) // use the same db with different key
	nodeTwo.Set(enr.IPv6{0x20, 0x01, 0x48, 0x60, 0, 0, 0x20, 0x01, 0, 0, 0, 0, 0, 0, 0x00, 0x68})
	seqTwo := nodeTwo.Seq()
	assert.NotEqual(t, seqTwo, newSeq)
	sTwo := &Service{dv5Listener: testp2p.NewMockListener(nodeTwo, nil)}
	_, err = sTwo.DiscoveryAddresses()
	require.NoError(t, err)
	assert.Equal(t, seqTwo+1, nodeTwo.Seq())

	// see that reloading the same node with same key and db results in same seq number
	nodeThree := enode.NewLocalNode(db, key)
	assert.Equal(t, node.Seq(), nodeThree.Seq())
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
	custodyGroupCount      *uint64
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

	if expected.custodyGroupCount != nil {
		// Check custody subnet count in ENR.
		var actualCustodyGroupCount uint64
		err := service.dv5Listener.LocalNode().Node().Record().Load(enr.WithEntry(params.BeaconNetworkConfig().CustodyGroupCountKey, &actualCustodyGroupCount))
		require.NoError(t, err)
		require.Equal(t, *expected.custodyGroupCount, actualCustodyGroupCount)

		// Check custody subnet count in metadata.
		actualGroupCountMetadata := service.metaData.CustodyGroupCount()
		require.Equal(t, *expected.custodyGroupCount, actualGroupCountMetadata)
	}
}

func TestRefreshPersistentSubnets(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	// Clean up caches after usage.
	defer cache.SubnetIDs.EmptyAllCaches()
	defer cache.SyncSubnetIDs.EmptyAllCaches()

	const (
		altairForkEpoch = 5
		fuluForkEpoch   = 10
	)

	custodyGroupCount := params.BeaconConfig().CustodyRequirement

	// Set up epochs.
	defaultCfg := params.BeaconConfig()
	cfg := defaultCfg.Copy()
	cfg.AltairForkEpoch = altairForkEpoch
	cfg.FuluForkEpoch = fuluForkEpoch
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
		{
			name:              "Fulu",
			epochSinceGenesis: fuluForkEpoch,
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
					custodyGroupCount:      &custodyGroupCount,
				},
				{
					pingCount:              2,
					metadataSequenceNumber: 2,
					attestationSubnets:     []uint64{40, 41},
					syncSubnets:            []uint64{1, 2},
					custodyGroupCount:      &custodyGroupCount,
				},
				{
					pingCount:              2,
					metadataSequenceNumber: 2,
					attestationSubnets:     []uint64{40, 41},
					syncSubnets:            []uint64{1, 2},
					custodyGroupCount:      &custodyGroupCount,
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
				cfg:                   &Config{UDPPort: 2000, DB: testDB.SetupDB(t)},
				peers:                 p2p.Peers(),
				genesisTime:           time.Now().Add(-time.Duration(tc.epochSinceGenesis*secondsPerEpoch) * time.Second),
				genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
				ctx:                   t.Context(),
				custodyInfoSet:        make(chan struct{}),
				custodyInfo:           &custodyInfo{groupCount: custodyGroupCount},
			}

			close(service.custodyInfoSet)

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

func TestFindPeers_NodeDeduplication(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cache.SubnetIDs.EmptyAllCaches()
	defer cache.SubnetIDs.EmptyAllCaches()

	ctx := t.Context()

	// Create LocalNodes and manipulate sequence numbers
	localNode1 := createTestNodeWithID(t, "node1")
	localNode2 := createTestNodeWithID(t, "node2")
	localNode3 := createTestNodeWithID(t, "node3")

	// Create different sequence versions of node1
	setNodeSeq(localNode1, 1)
	node1_seq1 := localNode1.Node()
	setNodeSeq(localNode1, 2)
	node1_seq2 := localNode1.Node() // Same ID, higher seq
	setNodeSeq(localNode1, 3)
	node1_seq3 := localNode1.Node() // Same ID, even higher seq

	// Other nodes with seq 1
	node2_seq1 := localNode2.Node()
	node3_seq1 := localNode3.Node()

	tests := []struct {
		name          string
		nodes         []*enode.Node
		missingPeers  uint
		expectedCount int
		description   string
		eval          func(t *testing.T, result []*enode.Node)
	}{
		{
			name: "No duplicates - all unique nodes",
			nodes: []*enode.Node{
				node2_seq1,
				node3_seq1,
			},
			missingPeers:  2,
			expectedCount: 2,
			description:   "Should return all unique nodes without deduplication",
			eval:          nil, // No special validation needed
		},
		{
			name: "Duplicate with lower seq comes first - should replace",
			nodes: []*enode.Node{
				node1_seq1,
				node1_seq2, // Higher seq, should replace
				node2_seq1, // Different node added after duplicates are processed
			},
			missingPeers:  2, // Need 2 peers so we process all nodes
			expectedCount: 2, // Should get node1 (with higher seq) and node2
			description:   "Should keep node with higher sequence number when duplicate found",
			eval: func(t *testing.T, result []*enode.Node) {
				// Should have node2 and node1 with higher seq (node1_seq2)
				foundNode1WithHigherSeq := false
				for _, node := range result {
					if node.ID() == node1_seq2.ID() {
						require.Equal(t, node1_seq2.Seq(), node.Seq(), "Node1 should have higher seq")
						foundNode1WithHigherSeq = true
					}
				}
				require.Equal(t, true, foundNode1WithHigherSeq, "Should have node1 with higher seq")
			},
		},
		{
			name: "Duplicate with higher seq comes first - should keep existing",
			nodes: []*enode.Node{
				node1_seq3, // Higher seq
				node1_seq2, // Lower seq, should be skipped (continue branch)
				node1_seq1, // Even lower seq, should also be skipped (continue branch)
				node2_seq1, // Different node added after duplicates are processed
			},
			missingPeers:  2,
			expectedCount: 2,
			description:   "Should keep existing node when it has higher sequence number and skip all lower seq duplicates",
			eval: func(t *testing.T, result []*enode.Node) {
				// Should have kept the node with highest seq (node1_seq3)
				foundNode1WithHigherSeq := false
				for _, node := range result {
					if node.ID() == node1_seq3.ID() {
						require.Equal(t, node1_seq3.Seq(), node.Seq(), "Node1 should have highest seq")
						foundNode1WithHigherSeq = true
					}
				}
				require.Equal(t, true, foundNode1WithHigherSeq, "Should have node1 with highest seq")
			},
		},
		{
			name: "Multiple duplicates with increasing seq",
			nodes: []*enode.Node{
				node1_seq1,
				node1_seq2, // Should replace seq1
				node1_seq3, // Should replace seq2
				node2_seq1, // Different node added after duplicates are processed
			},
			missingPeers:  2,
			expectedCount: 2,
			description:   "Should keep updating to highest sequence number",
			eval: func(t *testing.T, result []*enode.Node) {
				// Should have the node with highest seq (node1_seq3)
				foundNode1WithHigherSeq := false
				for _, node := range result {
					if node.ID() == node1_seq3.ID() {
						require.Equal(t, node1_seq3.Seq(), node.Seq(), "Node1 should have highest seq")
						foundNode1WithHigherSeq = true
					}
				}
				require.Equal(t, true, foundNode1WithHigherSeq, "Should have node1 with highest seq")
			},
		},
		{
			name: "Duplicate with equal seq comes after - should skip",
			nodes: []*enode.Node{
				node1_seq2, // First occurrence
				node1_seq2, // Same exact node instance, should be skipped (continue branch for >= case)
				node2_seq1, // Different node
			},
			missingPeers:  2,
			expectedCount: 2,
			description:   "Should skip duplicate with equal sequence number",
			eval: func(t *testing.T, result []*enode.Node) {
				// Should have exactly one instance of node1_seq2 and one instance of node2_seq1
				foundNode1 := false
				foundNode2 := false
				for _, node := range result {
					if node.ID() == node1_seq2.ID() {
						require.Equal(t, node1_seq2.Seq(), node.Seq(), "Node1 should have the expected seq")
						require.Equal(t, false, foundNode1, "Should have only one instance of node1") // Ensure no duplicates
						foundNode1 = true
					}
					if node.ID() == node2_seq1.ID() {
						foundNode2 = true
					}
				}
				require.Equal(t, true, foundNode1, "Should have node1")
				require.Equal(t, true, foundNode2, "Should have node2")
			},
		},
		{
			name: "Mix of unique and duplicate nodes",
			nodes: []*enode.Node{
				node1_seq1,
				node2_seq1,
				node1_seq2, // Should replace node1_seq1
				node3_seq1,
				node1_seq3, // Should replace node1_seq2
			},
			missingPeers:  3,
			expectedCount: 3,
			description:   "Should handle mix of unique nodes and duplicates correctly",
			eval:          nil, // Basic count validation is sufficient
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakePeer := testp2p.NewTestP2P(t)

			s := &Service{
				cfg: &Config{
					MaxPeers: 30,
				},
				genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
				peers: peers.NewStatus(ctx, &peers.StatusConfig{
					PeerLimit:    30,
					ScorerParams: &scorers.Config{},
				}),
				host: fakePeer.BHost,
			}

			localNode := createTestNodeRandom(t)
			mockIter := testp2p.NewMockIterator(tt.nodes)
			s.dv5Listener = testp2p.NewMockListener(localNode, mockIter)

			ctxWithTimeout, cancel := context.WithTimeout(ctx, 1*time.Second)
			defer cancel()

			result, err := s.findPeers(ctxWithTimeout, tt.missingPeers)

			require.NoError(t, err, tt.description)
			require.Equal(t, tt.expectedCount, len(result), tt.description)

			if tt.eval != nil {
				tt.eval(t, result)
			}
		})
	}
}

// callbackIterator allows us to execute callbacks at specific points during iteration
type callbackIterator struct {
	nodes     []*enode.Node
	index     int
	callbacks map[int]func() // map from index to callback function
}

func (c *callbackIterator) Next() bool {
	// Execute callback before checking if we can continue (if one exists)
	if callback, exists := c.callbacks[c.index]; exists {
		callback()
	}

	return c.index < len(c.nodes)
}

func (c *callbackIterator) Node() *enode.Node {
	if c.index >= len(c.nodes) {
		return nil
	}

	node := c.nodes[c.index]
	c.index++
	return node
}

func (c *callbackIterator) Close() {
	// Nothing to clean up for this simple implementation
}

func TestFindPeers_received_bad_existing_node(t *testing.T) {
	// This test successfully triggers delete(nodeByNodeID, node.ID()) in subnets.go by:
	// 1. Processing node1_seq1 first (passes filterPeer, gets added to map
	// 2. Callback marks peer as bad before processing node1_seq2"
	// 3. Processing node1_seq2 (fails filterPeer, triggers delete since ok=true
	params.SetupTestConfigCleanup(t)
	cache.SubnetIDs.EmptyAllCaches()
	defer cache.SubnetIDs.EmptyAllCaches()

	// Create LocalNode with same ID but different sequences
	localNode1 := createTestNodeWithID(t, "testnode")
	node1_seq1 := localNode1.Node() // Get current node
	currentSeq := node1_seq1.Seq()
	setNodeSeq(localNode1, currentSeq+1) // Increment sequence by 1
	node1_seq2 := localNode1.Node()      // This should have higher seq

	// Additional node to ensure we have enough peers to process
	localNode2 := createTestNodeWithID(t, "othernode")
	node2 := localNode2.Node()

	fakePeer := testp2p.NewTestP2P(t)

	service := &Service{
		cfg: &Config{
			MaxPeers: 30,
		},
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		peers: peers.NewStatus(t.Context(), &peers.StatusConfig{
			PeerLimit:    30,
			ScorerParams: &scorers.Config{},
		}),
		host: fakePeer.BHost,
	}

	// Create iterator with callback that marks peer as bad before processing node1_seq2
	iter := &callbackIterator{
		nodes: []*enode.Node{node1_seq1, node1_seq2, node2},
		index: 0,
		callbacks: map[int]func(){
			1: func() { // Before processing node1_seq2 (index 1)
				// Mark peer as bad before processing node1_seq2
				peerData, _, _ := convertToAddrInfo(node1_seq2)
				if peerData != nil {
					service.peers.Add(node1_seq2.Record(), peerData.ID, nil, network.DirUnknown)
					// Mark as bad peer - need enough increments to exceed threshold (6)
					for i := 0; i < 10; i++ {
						service.peers.Scorers().BadResponsesScorer().Increment(peerData.ID)
					}
				}
			},
		},
	}

	localNode := createTestNodeRandom(t)
	service.dv5Listener = testp2p.NewMockListener(localNode, iter)

	// Run findPeers - node1_seq1 gets processed first, then callback marks peer bad, then node1_seq2 fails
	ctxWithTimeout, cancel := context.WithTimeout(t.Context(), 1*time.Second)
	defer cancel()

	result, err := service.findPeers(ctxWithTimeout, 3)

	require.NoError(t, err)
	require.Equal(t, 1, len(result))
}
