package p2p

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"math"
	"net"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	ecdsaprysm "github.com/OffchainLabs/prysm/v6/crypto/ecdsa"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/sirupsen/logrus"
)

type (
	// ListenerRebooter is an interface that extends the Listener interface
	// with the `RebootListener` method.
	ListenerRebooter interface {
		Listener
		RebootListener() error
	}

	// Listener defines the discovery V5 network interface that is used
	// to communicate with other peers.
	Listener interface {
		Self() *enode.Node
		Close()
		Lookup(enode.ID) []*enode.Node
		Resolve(*enode.Node) *enode.Node
		RandomNodes() enode.Iterator
		Ping(*enode.Node) error
		RequestENR(*enode.Node) (*enode.Node, error)
		LocalNode() *enode.LocalNode
	}

	quicProtocol uint16

	listenerWrapper struct {
		mu              sync.RWMutex
		listener        *discover.UDPv5
		listenerCreator func() (*discover.UDPv5, error)
	}

	connectivityDirection int
	udpVersion            int
)

const quickProtocolEnrKey = "quic"

const (
	udp4 udpVersion = iota
	udp6
)

const (
	inbound connectivityDirection = iota
	all
)

// quicProtocol is the "quic" key, which holds the QUIC port of the node.
func (quicProtocol) ENRKey() string { return quickProtocolEnrKey }

func newListener(listenerCreator func() (*discover.UDPv5, error)) (*listenerWrapper, error) {
	rawListener, err := listenerCreator()
	if err != nil {
		return nil, errors.Wrap(err, "create new listener")
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
	_, err := l.listener.Ping(node)
	return err
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

// RefreshPersistentSubnets checks that we are tracking our local persistent subnets for a variety of gossip topics.
// This routine verifies and updates our attestation and sync committee subnets if they have been rotated.
func (s *Service) RefreshPersistentSubnets() {
	// Return early if discv5 service isn't running.
	if s.dv5Listener == nil || !s.isInitialized() {
		return
	}

	// Get the current epoch.
	currentSlot := slots.CurrentSlot(s.genesisTime)
	currentEpoch := slots.ToEpoch(currentSlot)

	// Get our node ID.
	nodeID := s.dv5Listener.LocalNode().ID()

	// Get our node record.
	record := s.dv5Listener.Self().Record()

	// Get the version of our metadata.
	metadataVersion := s.Metadata().Version()

	// Initialize persistent subnets.
	if err := initializePersistentSubnets(nodeID, currentEpoch); err != nil {
		log.WithError(err).Error("Could not initialize persistent subnets")
		return
	}

	// Get the current attestation subnet bitfield.
	bitV := bitfield.NewBitvector64()
	attestationCommittees := cache.SubnetIDs.GetAllSubnets()
	for _, idx := range attestationCommittees {
		bitV.SetBitAt(idx, true)
	}

	// Get the attestation subnet bitfield we store in our record.
	inRecordBitV, err := attBitvector(record)
	if err != nil {
		log.WithError(err).Error("Could not retrieve att bitfield")
		return
	}

	// Get the attestation subnet bitfield in our metadata.
	inMetadataBitV := s.Metadata().AttnetsBitfield()

	// Is our attestation bitvector record up to date?
	isBitVUpToDate := bytes.Equal(bitV, inRecordBitV) && bytes.Equal(bitV, inMetadataBitV)

	// Compare current epoch with Altair fork epoch
	altairForkEpoch := params.BeaconConfig().AltairForkEpoch

	// We add `1` to the current epoch because we want to prepare one epoch before the Altair fork.
	if currentEpoch+1 < altairForkEpoch {
		// Phase 0 behaviour.
		if isBitVUpToDate {
			// Return early if bitfield hasn't changed.
			return
		}

		// Some data changed. Update the record and the metadata.
		// Not returning early here because the error comes from saving the metadata sequence number.
		if err := s.updateSubnetRecordWithMetadata(bitV); err != nil {
			log.WithError(err).Error("Failed to update subnet record with metadata")
		}

		// Ping all peers.
		s.pingPeersAndLogEnr()

		return
	}

	// Get the current sync subnet bitfield.
	bitS := bitfield.Bitvector4{byte(0x00)}
	syncCommittees := cache.SyncSubnetIDs.GetAllSubnets(currentEpoch)
	for _, idx := range syncCommittees {
		bitS.SetBitAt(idx, true)
	}

	// Get the sync subnet bitfield we store in our record.
	inRecordBitS, err := syncBitvector(record)
	if err != nil {
		log.WithError(err).Error("Could not retrieve sync bitfield")
		return
	}

	// Get the sync subnet bitfield in our metadata.
	currentBitSInMetadata := s.Metadata().SyncnetsBitfield()

	isBitSUpToDate := bytes.Equal(bitS, inRecordBitS) && bytes.Equal(bitS, currentBitSInMetadata)

	// Compare current epoch with the Fulu fork epoch.
	fuluForkEpoch := params.BeaconConfig().FuluForkEpoch

	custodyGroupCount, inRecordCustodyGroupCount := uint64(0), uint64(0)
	if params.FuluEnabled() {
		// Get the custody group count we store in our record.
		inRecordCustodyGroupCount, err = peerdas.CustodyGroupCountFromRecord(record)
		if err != nil {
			log.WithError(err).Error("Could not retrieve custody group count")
			return
		}

		custodyGroupCount, err = s.CustodyGroupCount(s.ctx)
		if err != nil {
			log.WithError(err).Error("Could not retrieve custody group count")
			return
		}
	}

	// We add `1` to the current epoch because we want to prepare one epoch before the Fulu fork.
	if currentEpoch+1 < fuluForkEpoch {
		// Is our custody group count record up to date?
		isCustodyGroupCountUpToDate := custodyGroupCount == inRecordCustodyGroupCount

		// Altair behaviour.
		if metadataVersion == version.Altair && isBitVUpToDate && isBitSUpToDate && (!params.FuluEnabled() || isCustodyGroupCountUpToDate) {
			// Nothing to do, return early.
			return
		}

		// Some data have changed, update our record and metadata.
		// Not returning early here because the error comes from saving the metadata sequence number.
		if err := s.updateSubnetRecordWithMetadataV2(bitV, bitS, custodyGroupCount); err != nil {
			log.WithError(err).Error("Failed to update subnet record with metadata")
		}

		// Ping all peers to inform them of new metadata
		s.pingPeersAndLogEnr()

		return
	}

	// Get the custody group count in our metadata.
	inMetadataCustodyGroupCount := s.Metadata().CustodyGroupCount()

	// Is our custody group count record up to date?
	isCustodyGroupCountUpToDate := (custodyGroupCount == inRecordCustodyGroupCount && custodyGroupCount == inMetadataCustodyGroupCount)

	if isBitVUpToDate && isBitSUpToDate && isCustodyGroupCountUpToDate {
		// Nothing to do, return early.
		return
	}

	// Some data changed. Update the record and the metadata.
	// Not returning early here because the error comes from saving the metadata sequence number.
	if err := s.updateSubnetRecordWithMetadataV3(bitV, bitS, custodyGroupCount); err != nil {
		log.WithError(err).Error("Failed to update subnet record with metadata")
	}

	// Ping all peers.
	s.pingPeersAndLogEnr()
}

// listen for new nodes watches for new nodes in the network and adds them to the peerstore.
func (s *Service) listenForNewNodes() {
	const (
		thresholdLimit = 5
		searchPeriod   = 20 * time.Second
	)

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
			if thresholdCount > thresholdLimit {
				outBoundConnectedCount := len(s.peers.OutboundConnected())
				log.WithField("outboundConnectionCount", outBoundConnectedCount).Warn("Rebooting discovery listener, reached threshold.")
				if err := s.dv5Listener.RebootListener(); err != nil {
					log.WithError(err).Error("Could not reboot listener")
					continue
				}

				thresholdCount = 0
			}
		default:
			if s.isPeerAtLimit(all) {
				// Pause the main loop for a period to stop looking for new peers.
				log.Trace("Not looking for peers, at peer limit")
				time.Sleep(pollingPeriod)
				continue
			}

			// Return early if the discovery listener isn't set.
			if s.dv5Listener == nil {
				return
			}

			func() {
				ctx, cancel := context.WithTimeout(s.ctx, searchPeriod)
				defer cancel()

				if err := s.findAndDialPeers(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
					log.WithError(err).Error("Failed to find and dial peers")
				}
			}()
		}
	}
}

// FindAndDialPeersWithSubnets ensures that our node is connected to enough peers.
// If, the threshold is met, then this function immediately returns.
// Otherwise, it searches for new peers and dials them.
// If `ctx“ is canceled while searching for peers, search is stopped, but new found peers are still dialed.
// In this case, the function returns an error.
func (s *Service) findAndDialPeers(ctx context.Context) error {
	// Restrict dials if limit is applied.
	maxConcurrentDials := math.MaxInt
	if flags.MaxDialIsActive() {
		maxConcurrentDials = flags.Get().MaxConcurrentDials
	}

	missingPeerCount := s.missingPeerCount(s.cfg.MaxPeers)
	for missingPeerCount > 0 {
		// Stop the search/dialing loop if the context is canceled.
		if err := ctx.Err(); err != nil {
			return err
		}

		peersToDial, err := func() ([]*enode.Node, error) {
			ctx, cancel := context.WithTimeout(ctx, batchPeriod)
			defer cancel()

			peersToDial, err := s.findPeers(ctx, missingPeerCount)
			if err != nil && !errors.Is(err, context.DeadlineExceeded) {
				return nil, errors.Wrap(err, "find peers")
			}

			return peersToDial, nil
		}()

		if err != nil {
			return err
		}

		dialedPeerCount := s.dialPeers(s.ctx, maxConcurrentDials, peersToDial)

		if dialedPeerCount > missingPeerCount {
			missingPeerCount = 0
			continue
		}

		missingPeerCount -= dialedPeerCount
	}

	return nil
}

// findAndDialPeers finds new peers until `targetPeerCount` is reached, `batchPeriod` is over,
// the peers iterator is exhausted or the context is canceled.
func (s *Service) findPeers(ctx context.Context, missingPeerCount uint) ([]*enode.Node, error) {
	// Create an discovery iterator to find new peers.
	iterator := s.dv5Listener.RandomNodes()

	// `iterator.Next` can block indefinitely. `iterator.Close` unblocks it.
	// So it is important to close the iterator when the context is done to ensure
	// that the search does not hang indefinitely.
	go func() {
		<-ctx.Done()
		iterator.Close()
	}()

	// Crawl the network for peers subscribed to the defective subnets.
	nodeByNodeID := make(map[enode.ID]*enode.Node)
	for missingPeerCount > 0 && iterator.Next() {
		if ctx.Err() != nil {
			peersToDial := make([]*enode.Node, 0, len(nodeByNodeID))
			for _, node := range nodeByNodeID {
				peersToDial = append(peersToDial, node)
			}

			return peersToDial, ctx.Err()
		}

		node := iterator.Node()

		// Remove duplicates, keeping the node with higher seq.
		existing, ok := nodeByNodeID[node.ID()]
		if ok && existing.Seq() >= node.Seq() {
			continue // keep existing and skip.
		}

		// Treat nodes that exist in nodeByNodeID with higher seq numbers as new peers
		// Skip peer not matching the filter.
		if !s.filterPeer(node) {
			if ok {
				// this means the existing peer with the lower sequence number is no longer valid
				delete(nodeByNodeID, existing.ID())
				missingPeerCount++
			}
			continue
		}

		// We found a new peer. Decrease the missing peer count.
		nodeByNodeID[node.ID()] = node
		missingPeerCount--
	}

	// Convert the map to a slice.
	peersToDial := make([]*enode.Node, 0, len(nodeByNodeID))
	for _, node := range nodeByNodeID {
		peersToDial = append(peersToDial, node)
	}

	return peersToDial, nil
}

// missingPeerCount computes how many peers we are missing to reach the target peer count.
func (s *Service) missingPeerCount(targetCount uint) uint {
	// Retrieve how many active peers we have.
	activePeers := s.Peers().Active()
	activePeerCount := uint(len(activePeers))

	// Compute how many peers we are missing to reach the threshold.
	missingPeerCount := uint(0)
	if targetCount > activePeerCount {
		missingPeerCount = targetCount - activePeerCount
	}

	return missingPeerCount
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
		return nil, errors.Wrap(err, "create local node")
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
		PrivateKey:              privKey,
		Bootnodes:               bootNodes,
		PingInterval:            s.cfg.PingInterval,
		NoFindnodeLivenessCheck: s.cfg.DisableLivenessCheck,
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
	db, err := enode.OpenDB(s.cfg.DiscoveryDir)
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

	currentSlot := slots.CurrentSlot(s.genesisTime)
	currentEpoch := slots.ToEpoch(currentSlot)
	current := params.GetNetworkScheduleEntry(currentEpoch)
	next := params.NextNetworkScheduleEntry(currentEpoch)
	if err := updateENR(localNode, current, next); err != nil {
		return nil, errors.Wrap(err, "could not add eth2 fork version entry to enr")
	}

	localNode = initializeAttSubnets(localNode)
	localNode = initializeSyncCommSubnets(localNode)

	if params.FuluEnabled() {
		custodyGroupCount, err := s.CustodyGroupCount(s.ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not retrieve custody group count")
		}

		custodyGroupCountEntry := peerdas.Cgc(custodyGroupCount)
		localNode.Set(custodyGroupCountEntry)
	}

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
	log.WithFields(logrus.Fields{
		"seq": localNode.Seq(),
		"id":  localNode.ID(),
	}).Debug("Local node created")
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
		return nil, errors.Wrap(err, "create listener")
	}
	record := wrappedListener.Self()

	log.WithFields(logrus.Fields{
		"ENR": record.String(),
		"seq": record.Seq(),
	}).Info("Started discovery v5")
	return wrappedListener, nil
}

// filterPeer validates each node that we retrieve from our dht. We
// try to ascertain that the peer can be a valid protocol peer.
// Validity Conditions:
//  1. Peer has a valid IP and a (QUIC and/or TCP) port set in their enr.
//  2. Peer hasn't been marked as 'bad'.
//  3. Peer is not currently active or connected.
//  4. Peer is ready to receive incoming connections.
//  5. Peer's fork digest in their ENR matches that of our localnodes.
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
		log.WithError(err).WithField("node", node.String()).Debug("Could not convert to peer data")
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
		// Constantly update enr for known peers
		s.peers.UpdateENR(node.Record(), peerData.ID)
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
		if err := compareForkENR(s.dv5Listener.LocalNode().Node().Record(), nodeENR); err != nil {
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
func (s *Service) isPeerAtLimit(direction connectivityDirection) bool {
	maxPeers := int(s.cfg.MaxPeers)

	// If we are measuring the limit for inbound peers we apply the high watermark buffer.
	if direction == inbound {
		maxPeers += highWatermarkBuffer
		maxInbound := s.peers.InboundLimit() + highWatermarkBuffer
		inboundCount := len(s.peers.InboundConnected())

		// Return early if we are at the inbound limit.
		if inboundCount >= maxInbound {
			return true
		}
	}

	peerCount := len(s.host.Network().Peers())
	activePeerCount := len(s.Peers().Active())
	return activePeerCount >= maxPeers || peerCount >= maxPeers
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
		return nil, nil, errors.Wrap(err, "retrieve multiaddrs from node")
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

func udpVersionFromIP(ipAddr net.IP) udpVersion {
	if ipAddr.To4() != nil {
		return udp4
	}
	return udp6
}
