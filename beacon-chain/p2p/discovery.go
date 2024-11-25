package p2p

import (
	"bytes"
	"crypto/ecdsa"
	"net"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v5/crypto/ecdsa"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

type ListenerRebooter interface {
	Listener
	RebootListener() error
}

// Listener defines the discovery V5 network interface that is used
// to communicate with other peers.
type Listener interface {
	Self() *enode.Node
	Close()
	Lookup(enode.ID) []*enode.Node
	Resolve(*enode.Node) *enode.Node
	RandomNodes() enode.Iterator
	Ping(*enode.Node) error
	RequestENR(*enode.Node) (*enode.Node, error)
	LocalNode() *enode.LocalNode
}

const (
	udp4 = iota
	udp6
)

type quicProtocol uint16

// quicProtocol is the "quic" key, which holds the QUIC port of the node.
func (quicProtocol) ENRKey() string { return "quic" }

type listenerWrapper struct {
	mu              sync.RWMutex
	listener        *discover.UDPv5
	listenerCreator func() (*discover.UDPv5, error)
}

func newListener(listenerCreator func() (*discover.UDPv5, error)) (*listenerWrapper, error) {
	rawListener, err := listenerCreator()
	if err != nil {
		return nil, errors.Wrap(err, "could not create new listener")
	}
	return &listenerWrapper{
		listener:        rawListener,
		listenerCreator: listenerCreator,
	}, nil
}

func (l *listenerWrapper) Self() *enode.Node {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.listener.Self()
}

func (l *listenerWrapper) Close() {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.listener.Close()
}

func (l *listenerWrapper) Lookup(id enode.ID) []*enode.Node {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.listener.Lookup(id)
}

func (l *listenerWrapper) Resolve(node *enode.Node) *enode.Node {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.listener.Resolve(node)
}

func (l *listenerWrapper) RandomNodes() enode.Iterator {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.listener.RandomNodes()
}

func (l *listenerWrapper) Ping(node *enode.Node) error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.listener.Ping(node)
}

func (l *listenerWrapper) RequestENR(node *enode.Node) (*enode.Node, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.listener.RequestENR(node)
}

func (l *listenerWrapper) LocalNode() *enode.LocalNode {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.listener.LocalNode()
}

func (l *listenerWrapper) RebootListener() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Close current listener
	l.listener.Close()

	newListener, err := l.listenerCreator()
	if err != nil {
		return err
	}

	l.listener = newListener
	return nil
}

// RefreshENR uses an epoch to refresh the enr entry for our node
// with the tracked committee ids for the epoch, allowing our node
// to be dynamically discoverable by others given our tracked committee ids.
func (s *Service) RefreshENR() {
	// return early if discv5 isn't running
	if s.dv5Listener == nil || !s.isInitialized() {
		return
	}
	currEpoch := slots.ToEpoch(slots.CurrentSlot(uint64(s.genesisTime.Unix())))
	if err := initializePersistentSubnets(s.dv5Listener.LocalNode().ID(), currEpoch); err != nil {
		log.WithError(err).Error("Could not initialize persistent subnets")
		return
	}

	bitV := bitfield.NewBitvector64()
	committees := cache.SubnetIDs.GetAllSubnets()
	for _, idx := range committees {
		bitV.SetBitAt(idx, true)
	}
	currentBitV, err := attBitvector(s.dv5Listener.Self().Record())
	if err != nil {
		log.WithError(err).Error("Could not retrieve att bitfield")
		return
	}

	// Compare current epoch with our fork epochs
	altairForkEpoch := params.BeaconConfig().AltairForkEpoch
	switch {
	case currEpoch < altairForkEpoch:
		// Phase 0 behaviour.
		if bytes.Equal(bitV, currentBitV) {
			// return early if bitfield hasn't changed
			return
		}
		s.updateSubnetRecordWithMetadata(bitV)
	default:
		// Retrieve sync subnets from application level
		// cache.
		bitS := bitfield.Bitvector4{byte(0x00)}
		committees = cache.SyncSubnetIDs.GetAllSubnets(currEpoch)
		for _, idx := range committees {
			bitS.SetBitAt(idx, true)
		}
		currentBitS, err := syncBitvector(s.dv5Listener.Self().Record())
		if err != nil {
			log.WithError(err).Error("Could not retrieve sync bitfield")
			return
		}
		if bytes.Equal(bitV, currentBitV) && bytes.Equal(bitS, currentBitS) &&
			s.Metadata().Version() == version.Altair {
			// return early if bitfields haven't changed
			return
		}
		s.updateSubnetRecordWithMetadataV2(bitV, bitS)
	}
	// ping all peers to inform them of new metadata
	s.pingPeersAndLogEnr()
}

// listen for new nodes watches for new nodes in the network and adds them to the peerstore.
func (s *Service) listenForNewNodes() {
	iterator := filterNodes(s.ctx, s.dv5Listener.RandomNodes(), s.filterPeer)
	defer iterator.Close()
	connectivityTicker := time.NewTicker(1 * time.Minute)
	thresholdCount := 0

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-connectivityTicker.C:
			// Skip the connectivity check if not enabled.
			if !features.Get().EnableDiscoveryReboot {
				continue
			}
			if !s.isBelowOutboundPeerThreshold() {
				// Reset counter if we are beyond the threshold
				thresholdCount = 0
				continue
			}
			thresholdCount++
			// Reboot listener if connectivity drops
			if thresholdCount > 5 {
				log.WithField("outboundConnectionCount", len(s.peers.OutboundConnected())).Warn("Rebooting discovery listener, reached threshold.")
				if err := s.dv5Listener.RebootListener(); err != nil {
					log.WithError(err).Error("Could not reboot listener")
					continue
				}
				iterator = filterNodes(s.ctx, s.dv5Listener.RandomNodes(), s.filterPeer)
				thresholdCount = 0
			}
		default:
			if s.isPeerAtLimit(false /* inbound */) {
				// Pause the main loop for a period to stop looking
				// for new peers.
				log.Trace("Not looking for peers, at peer limit")
				time.Sleep(pollingPeriod)
				continue
			}
			wantedCount := s.wantedPeerDials()
			if wantedCount == 0 {
				log.Trace("Not looking for peers, at peer limit")
				time.Sleep(pollingPeriod)
				continue
			}
			// Restrict dials if limit is applied.
			if flags.MaxDialIsActive() {
				wantedCount = min(wantedCount, flags.Get().MaxConcurrentDials)
			}
			wantedNodes := enode.ReadNodes(iterator, wantedCount)
			wg := new(sync.WaitGroup)
			for i := 0; i < len(wantedNodes); i++ {
				node := wantedNodes[i]
				peerInfo, _, err := convertToAddrInfo(node)
				if err != nil {
					log.WithError(err).Error("Could not convert to peer info")
					continue
				}

				if peerInfo == nil {
					continue
				}

				// Make sure that peer is not dialed too often, for each connection attempt there's a backoff period.
				s.Peers().RandomizeBackOff(peerInfo.ID)
				wg.Add(1)
				go func(info *peer.AddrInfo) {
					if err := s.connectWithPeer(s.ctx, *info); err != nil {
						log.WithError(err).Tracef("Could not connect with peer %s", info.String())
					}
					wg.Done()
				}(peerInfo)
			}
			wg.Wait()
		}
	}
}

func (s *Service) createListener(
	ipAddr net.IP,
	privKey *ecdsa.PrivateKey,
) (*discover.UDPv5, error) {
	// BindIP is used to specify the ip
	// on which we will bind our listener on
	// by default we will listen to all interfaces.
	var bindIP net.IP
	switch udpVersionFromIP(ipAddr) {
	case udp4:
		bindIP = net.IPv4zero
	case udp6:
		bindIP = net.IPv6zero
	default:
		return nil, errors.New("invalid ip provided")
	}

	// If local ip is specified then use that instead.
	if s.cfg.LocalIP != "" {
		ipAddr = net.ParseIP(s.cfg.LocalIP)
		if ipAddr == nil {
			return nil, errors.New("invalid local ip provided")
		}
		bindIP = ipAddr
	}
	udpAddr := &net.UDPAddr{
		IP:   bindIP,
		Port: int(s.cfg.UDPPort),
	}

	// Listen to all network interfaces
	// for both ip protocols.
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, errors.Wrap(err, "could not listen to UDP")
	}

	localNode, err := s.createLocalNode(
		privKey,
		ipAddr,
		int(s.cfg.UDPPort),
		int(s.cfg.TCPPort),
		int(s.cfg.QUICPort),
	)
	if err != nil {
		return nil, errors.Wrap(err, "could not create local node")
	}

	bootNodes := make([]*enode.Node, 0, len(s.cfg.Discv5BootStrapAddrs))
	for _, addr := range s.cfg.Discv5BootStrapAddrs {
		bootNode, err := enode.Parse(enode.ValidSchemes, addr)
		if err != nil {
			return nil, errors.Wrap(err, "could not bootstrap addr")
		}

		bootNodes = append(bootNodes, bootNode)
	}

	dv5Cfg := discover.Config{
		PrivateKey: privKey,
		Bootnodes:  bootNodes,
	}

	listener, err := discover.ListenV5(conn, localNode, dv5Cfg)
	if err != nil {
		return nil, errors.Wrap(err, "could not listen to discV5")
	}

	return listener, nil
}

func (s *Service) createLocalNode(
	privKey *ecdsa.PrivateKey,
	ipAddr net.IP,
	udpPort, tcpPort, quicPort int,
) (*enode.LocalNode, error) {
	db, err := enode.OpenDB("")
	if err != nil {
		return nil, errors.Wrap(err, "could not open node's peer database")
	}
	localNode := enode.NewLocalNode(db, privKey)

	ipEntry := enr.IP(ipAddr)
	localNode.Set(ipEntry)

	udpEntry := enr.UDP(udpPort)
	localNode.Set(udpEntry)

	tcpEntry := enr.TCP(tcpPort)
	localNode.Set(tcpEntry)

	if features.Get().EnableQUIC {
		quicEntry := quicProtocol(quicPort)
		localNode.Set(quicEntry)
	}

	localNode.SetFallbackIP(ipAddr)
	localNode.SetFallbackUDP(udpPort)

	localNode, err = addForkEntry(localNode, s.genesisTime, s.genesisValidatorsRoot)
	if err != nil {
		return nil, errors.Wrap(err, "could not add eth2 fork version entry to enr")
	}

	localNode = initializeAttSubnets(localNode)
	localNode = initializeSyncCommSubnets(localNode)

	if s.cfg != nil && s.cfg.HostAddress != "" {
		hostIP := net.ParseIP(s.cfg.HostAddress)
		if hostIP.To4() == nil && hostIP.To16() == nil {
			return nil, errors.Errorf("invalid host address: %s", s.cfg.HostAddress)
		} else {
			localNode.SetFallbackIP(hostIP)
			localNode.SetStaticIP(hostIP)
		}
	}

	if s.cfg != nil && s.cfg.HostDNS != "" {
		host := s.cfg.HostDNS
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, errors.Wrapf(err, "could not resolve host address: %s", host)
		}
		if len(ips) > 0 {
			// Use first IP returned from the
			// resolver.
			firstIP := ips[0]
			localNode.SetFallbackIP(firstIP)
		}
	}

	return localNode, nil
}

func (s *Service) startDiscoveryV5(
	addr net.IP,
	privKey *ecdsa.PrivateKey,
) (*listenerWrapper, error) {
	createListener := func() (*discover.UDPv5, error) {
		return s.createListener(addr, privKey)
	}
	wrappedListener, err := newListener(createListener)
	if err != nil {
		return nil, errors.Wrap(err, "could not create listener")
	}
	record := wrappedListener.Self()
	log.WithField("ENR", record.String()).Info("Started discovery v5")
	return wrappedListener, nil
}

// filterPeer validates each node that we retrieve from our dht. We
// try to ascertain that the peer can be a valid protocol peer.
// Validity Conditions:
//  1. Peer has a valid IP and a (QUIC and/or TCP) port set in their enr.
//  2. Peer hasn't been marked as 'bad'.
//  3. Peer is not currently active or connected.
//  4. Peer is ready to receive incoming connections.
//  5. Peer's fork digest in their ENR matches that of
//     our localnodes.
func (s *Service) filterPeer(node *enode.Node) bool {
	// Ignore nil node entries passed in.
	if node == nil {
		return false
	}

	// Ignore nodes with no IP address stored.
	if node.IP() == nil {
		return false
	}

	peerData, multiAddrs, err := convertToAddrInfo(node)
	if err != nil {
		log.WithError(err).Debug("Could not convert to peer data")
		return false
	}

	if peerData == nil || len(multiAddrs) == 0 {
		return false
	}

	// Ignore bad nodes.
	if s.peers.IsBad(peerData.ID) != nil {
		return false
	}

	// Ignore nodes that are already active.
	if s.peers.IsActive(peerData.ID) {
		return false
	}

	// Ignore nodes that are already connected.
	if s.host.Network().Connectedness(peerData.ID) == network.Connected {
		return false
	}

	// Ignore nodes that are not ready to receive incoming connections.
	if !s.peers.IsReadyToDial(peerData.ID) {
		return false
	}

	// Ignore nodes that don't match our fork digest.
	nodeENR := node.Record()
	if s.genesisValidatorsRoot != nil {
		if err := s.compareForkENR(nodeENR); err != nil {
			log.WithError(err).Trace("Fork ENR mismatches between peer and local node")
			return false
		}
	}

	// If the peer has 2 multiaddrs, favor the QUIC address, which is in first position.
	multiAddr := multiAddrs[0]

	// Add peer to peer handler.
	s.peers.Add(nodeENR, peerData.ID, multiAddr, network.DirUnknown)

	return true
}

// This checks our set max peers in our config, and
// determines whether our currently connected and
// active peers are above our set max peer limit.
func (s *Service) isPeerAtLimit(inbound bool) bool {
	numOfConns := len(s.host.Network().Peers())
	maxPeers := int(s.cfg.MaxPeers)
	// If we are measuring the limit for inbound peers
	// we apply the high watermark buffer.
	if inbound {
		maxPeers += highWatermarkBuffer
		maxInbound := s.peers.InboundLimit() + highWatermarkBuffer
		currInbound := len(s.peers.InboundConnected())
		// Exit early if we are at the inbound limit.
		if currInbound >= maxInbound {
			return true
		}
	}
	activePeers := len(s.Peers().Active())
	return activePeers >= maxPeers || numOfConns >= maxPeers
}

// isBelowOutboundPeerThreshold checks if the number of outbound peers that
// we are connected to satisfies the minimum expected outbound peer count
// according to our peer limit.
func (s *Service) isBelowOutboundPeerThreshold() bool {
	maxPeers := int(s.cfg.MaxPeers)
	inBoundLimit := s.Peers().InboundLimit()
	// Impossible Condition
	if maxPeers < inBoundLimit {
		return false
	}
	outboundFloor := maxPeers - inBoundLimit
	outBoundThreshold := outboundFloor / 2
	outBoundCount := len(s.Peers().OutboundConnected())
	return outBoundCount < outBoundThreshold
}

func (s *Service) wantedPeerDials() int {
	maxPeers := int(s.cfg.MaxPeers)

	activePeers := len(s.Peers().Active())
	wantedCount := 0
	if maxPeers > activePeers {
		wantedCount = maxPeers - activePeers
	}
	return wantedCount
}

// PeersFromStringAddrs converts peer raw ENRs into multiaddrs for p2p.
func PeersFromStringAddrs(addrs []string) ([]ma.Multiaddr, error) {
	var allAddrs []ma.Multiaddr
	enodeString, multiAddrString := parseGenericAddrs(addrs)
	for _, stringAddr := range multiAddrString {
		addr, err := multiAddrFromString(stringAddr)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not get multiaddr from string")
		}
		allAddrs = append(allAddrs, addr)
	}
	for _, stringAddr := range enodeString {
		enodeAddr, err := enode.Parse(enode.ValidSchemes, stringAddr)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not get enode from string")
		}
		nodeAddrs, err := retrieveMultiAddrsFromNode(enodeAddr)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not get multiaddr")
		}
		allAddrs = append(allAddrs, nodeAddrs...)
	}
	return allAddrs, nil
}

func ParseBootStrapAddrs(addrs []string) (discv5Nodes []string) {
	discv5Nodes, _ = parseGenericAddrs(addrs)
	if len(discv5Nodes) == 0 {
		log.Warn("No bootstrap addresses supplied")
	}
	return discv5Nodes
}

func parseGenericAddrs(addrs []string) (enodeString, multiAddrString []string) {
	for _, addr := range addrs {
		if addr == "" {
			// Ignore empty entries
			continue
		}
		_, err := enode.Parse(enode.ValidSchemes, addr)
		if err == nil {
			enodeString = append(enodeString, addr)
			continue
		}
		_, err = multiAddrFromString(addr)
		if err == nil {
			multiAddrString = append(multiAddrString, addr)
			continue
		}
		log.WithError(err).Errorf("Invalid address of %s provided", addr)
	}
	return enodeString, multiAddrString
}

func convertToMultiAddr(nodes []*enode.Node) []ma.Multiaddr {
	// Expect each node to have a TCP and a QUIC address.
	multiAddrs := make([]ma.Multiaddr, 0, 2*len(nodes))

	for _, node := range nodes {
		// Skip nodes with no ip address stored.
		if node.IP() == nil {
			continue
		}

		// Get up to two multiaddrs (TCP and QUIC) for each node.
		nodeMultiAddrs, err := retrieveMultiAddrsFromNode(node)
		if err != nil {
			log.WithError(err).Errorf("Could not convert to multiAddr node %s", node)
			continue
		}

		multiAddrs = append(multiAddrs, nodeMultiAddrs...)
	}

	return multiAddrs
}

func convertToAddrInfo(node *enode.Node) (*peer.AddrInfo, []ma.Multiaddr, error) {
	multiAddrs, err := retrieveMultiAddrsFromNode(node)
	if err != nil {
		return nil, nil, err
	}

	if len(multiAddrs) == 0 {
		return nil, nil, nil
	}

	infos, err := peer.AddrInfosFromP2pAddrs(multiAddrs...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "could not convert to peer info: %v", multiAddrs)
	}

	if len(infos) != 1 {
		return nil, nil, errors.Errorf("infos contains %v elements, expected exactly 1", len(infos))
	}

	return &infos[0], multiAddrs, nil
}

// retrieveMultiAddrsFromNode converts an enode.Node to a list of multiaddrs.
// If the node has a both a QUIC and a TCP port set in their ENR, then
// the multiaddr corresponding to the QUIC port is added first, followed
// by the multiaddr corresponding to the TCP port.
func retrieveMultiAddrsFromNode(node *enode.Node) ([]ma.Multiaddr, error) {
	multiaddrs := make([]ma.Multiaddr, 0, 2)

	// Retrieve the node public key.
	pubkey := node.Pubkey()
	assertedKey, err := ecdsaprysm.ConvertToInterfacePubkey(pubkey)
	if err != nil {
		return nil, errors.Wrap(err, "could not get pubkey")
	}

	// Compute the node ID from the public key.
	id, err := peer.IDFromPublicKey(assertedKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not get peer id")
	}

	if features.Get().EnableQUIC {
		// If the QUIC entry is present in the ENR, build the corresponding multiaddress.
		port, ok, err := getPort(node, quic)
		if err != nil {
			return nil, errors.Wrap(err, "could not get QUIC port")
		}

		if ok {
			addr, err := multiAddressBuilderWithID(node.IP(), quic, port, id)
			if err != nil {
				return nil, errors.Wrap(err, "could not build QUIC address")
			}

			multiaddrs = append(multiaddrs, addr)
		}
	}

	// If the TCP entry is present in the ENR, build the corresponding multiaddress.
	port, ok, err := getPort(node, tcp)
	if err != nil {
		return nil, errors.Wrap(err, "could not get TCP port")
	}

	if ok {
		addr, err := multiAddressBuilderWithID(node.IP(), tcp, port, id)
		if err != nil {
			return nil, errors.Wrap(err, "could not build TCP address")
		}

		multiaddrs = append(multiaddrs, addr)
	}

	return multiaddrs, nil
}

// getPort retrieves the port for a given node and protocol, as well as a boolean
// indicating whether the port was found, and an error
func getPort(node *enode.Node, protocol internetProtocol) (uint, bool, error) {
	var (
		port uint
		err  error
	)

	switch protocol {
	case tcp:
		var entry enr.TCP
		err = node.Load(&entry)
		port = uint(entry)
	case udp:
		var entry enr.UDP
		err = node.Load(&entry)
		port = uint(entry)
	case quic:
		var entry quicProtocol
		err = node.Load(&entry)
		port = uint(entry)
	default:
		return 0, false, errors.Errorf("invalid protocol: %v", protocol)
	}

	if enr.IsNotFound(err) {
		return port, false, nil
	}

	if err != nil {
		return 0, false, errors.Wrap(err, "could not get port")
	}

	return port, true, nil
}

func convertToUdpMultiAddr(node *enode.Node) ([]ma.Multiaddr, error) {
	pubkey := node.Pubkey()
	assertedKey, err := ecdsaprysm.ConvertToInterfacePubkey(pubkey)
	if err != nil {
		return nil, errors.Wrap(err, "could not get pubkey")
	}
	id, err := peer.IDFromPublicKey(assertedKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not get peer id")
	}

	var addresses []ma.Multiaddr
	var ip4 enr.IPv4
	var ip6 enr.IPv6
	if node.Load(&ip4) == nil {
		address, ipErr := multiAddressBuilderWithID(net.IP(ip4), udp, uint(node.UDP()), id)
		if ipErr != nil {
			return nil, errors.Wrap(ipErr, "could not build IPv4 address")
		}
		addresses = append(addresses, address)
	}
	if node.Load(&ip6) == nil {
		address, ipErr := multiAddressBuilderWithID(net.IP(ip6), udp, uint(node.UDP()), id)
		if ipErr != nil {
			return nil, errors.Wrap(ipErr, "could not build IPv6 address")
		}
		addresses = append(addresses, address)
	}

	return addresses, nil
}

func peerIdsFromMultiAddrs(addrs []ma.Multiaddr) []peer.ID {
	var peers []peer.ID
	for _, a := range addrs {
		info, err := peer.AddrInfoFromP2pAddr(a)
		if err != nil {
			log.WithError(err).Errorf("Could not derive peer info from multiaddress %s", a.String())
			continue
		}
		peers = append(peers, info.ID)
	}
	return peers
}

func multiAddrFromString(address string) (ma.Multiaddr, error) {
	return ma.NewMultiaddr(address)
}

func udpVersionFromIP(ipAddr net.IP) int {
	if ipAddr.To4() != nil {
		return udp4
	}
	return udp6
}
