// Package peers provides information about peers at the Ethereum consensus protocol level.
//
// "Protocol level" is the level above the network level, so this layer never sees or interacts with
// (for example) hosts that are unreachable due to being down, firewalled, etc. Instead, this works
// with peers that are contactable but may or may not be of the correct fork version, not currently
// required due to the number of current connections, etc.
//
// A peer can have one of a number of states:
//
// - connected if we are able to talk to the remote peer
// - connecting if we are attempting to be able to talk to the remote peer
// - disconnecting if we are attempting to stop being able to talk to the remote peer
// - disconnected if we are not able to talk to the remote peer
//
// For convenience, there are two aggregate states expressed in functions:
//
// - active if we are connecting or connected
// - inactive if we are disconnecting or disconnected
//
// Peer information is persistent for the run of the service. This allows for collection of useful
// long-term statistics such as the peers' advertised chain state, giving the basis for
// decisions to not talk to grey-listed peers (see the p2p/peerscoring package).
package peers

import (
	"context"
	"net"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers/peerdata"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peerscoring"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/metadata"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/pkg/errors"
)

const (
	// Disconnected means there is no connection to the peer.
	Disconnected peerdata.ConnectionState = iota
	// Disconnecting means there is an on-going attempt to disconnect from the peer.
	Disconnecting
	// Connected means the peer has an active connection.
	Connected
	// Connecting means there is an on-going attempt to connect to the peer.
	Connecting
)

const (
	// CollocationLimit restricts how many peer identities we can see from a single ip or ipv6 subnet.
	CollocationLimit = 5

	// Additional buffer beyond current peer limit, from which we can store the relevant peer statuses.
	maxLimitBuffer = 150

	// InboundRatio is the proportion of our connected peer limit at which we will allow inbound peers.
	InboundRatio = float64(0.8)

	// MinBackOffDuration minimum amount (in milliseconds) to wait before peer is re-dialed.
	// When node and peer are dialing each other simultaneously connection may fail. In order, to break
	// of constant dialing, peer is assigned some backoff period, and only dialed again once that backoff is up.
	MinBackOffDuration = 100
	// MaxBackOffDuration maximum amount (in milliseconds) to wait before peer is re-dialed.
	MaxBackOffDuration = 5000

	// pruneTenureEpsilon is the probability that one PruneCandidates round ignores tenure
	// protection, so long-lived peers cannot become permanently unevictable.
	pruneTenureEpsilon = 0.05
	// pruneTenureProtectedDenominator: the oldest 1/4 of non-grey-listed candidates are
	// moved to the end of the eviction order.
	pruneTenureProtectedDenominator = 4
)

type InternetProtocol string

const (
	TCP  = InternetProtocol("tcp")
	QUIC = InternetProtocol("quic")
)

type (
	// Status is the structure holding the peer status information.
	Status struct {
		ctx     context.Context
		scoring *peerscoring.Scorer
		store   *peerdata.Store
		// rand is not concurrency-safe; every use must hold the store write lock.
		rand                  *rand.Rand
		ipTracker             map[string]uint64
		ipColocationWhitelist []*net.IPNet
		onTrustedPeerAdded    func(peer.ID)
		onTrustedPeerRemoved  func(peer.ID)
	}

	// StatusConfig represents peer status service params.
	StatusConfig struct {
		// PeerLimit specifies maximum amount of concurrent peers that are expected to be connect to the node.
		PeerLimit int
		// Scoring judges peers; a default scorer is created when nil.
		Scoring *peerscoring.Scorer
		// Rand overrides the internal random generator; a deterministic generator is created
		// when nil. Used by tests that need reproducible eviction ordering.
		Rand *rand.Rand
		// IPColocationWhitelist contains CIDR ranges that are exempt from IP colocation limits.
		IPColocationWhitelist []*net.IPNet
		// OnTrustedPeerAdded, if set, is called for every peer added to the trusted set.
		OnTrustedPeerAdded func(peer.ID)
		// OnTrustedPeerRemoved, if set, is called for every peer removed from the trusted set.
		OnTrustedPeerRemoved func(peer.ID)
	}
)

// NewStatus creates a new status entity.
func NewStatus(ctx context.Context, config *StatusConfig) *Status {
	store := peerdata.NewStore(ctx, &peerdata.StoreConfig{
		MaxPeers: maxLimitBuffer + config.PeerLimit,
	})

	scoring := config.Scoring
	if scoring == nil {
		scoring = peerscoring.NewScorer()
	}

	// Random generator used for dial backoff periods and eviction ordering.
	// It is ok to use deterministic generator, no need for true entropy.
	randGen := config.Rand
	if randGen == nil {
		randGen = rand.NewDeterministicGenerator()
	}

	return &Status{
		ctx:                   ctx,
		store:                 store,
		scoring:               scoring,
		ipTracker:             map[string]uint64{},
		ipColocationWhitelist: config.IPColocationWhitelist,
		onTrustedPeerAdded:    config.OnTrustedPeerAdded,
		onTrustedPeerRemoved:  config.OnTrustedPeerRemoved,
		rand:                  randGen,
	}
}

func (p *Status) UpdateENR(record *enr.Record, pid peer.ID) {
	p.store.Lock()
	defer p.store.Unlock()

	if peerData, ok := p.store.PeerData(pid); ok {
		peerData.Enr = record
	}
}

// MaxPeerLimit returns the max peer limit stored in the current peer store.
func (p *Status) MaxPeerLimit() int {
	return p.store.Config().MaxPeers
}

// Add adds a peer.
// If a peer already exists with this ID its address and direction are updated with the supplied data.
func (p *Status) Add(record *enr.Record, pid peer.ID, address ma.Multiaddr, direction network.Direction) {
	p.store.Lock()
	defer p.store.Unlock()

	if peerData, ok := p.store.PeerData(pid); ok {
		// Peer already exists, just update its address info.
		prevAddress := peerData.Address
		peerData.Address = address
		peerData.Direction = direction
		if record != nil {
			peerData.Enr = record
		}
		if !sameIP(prevAddress, address) {
			p.addIpToTracker(pid)
		}
		return
	}
	peerData := &peerdata.PeerData{
		Address:   address,
		Direction: direction,
		// Peers start disconnected; state will be updated when the handshake process begins.
		ConnState: Disconnected,
	}
	if record != nil {
		peerData.Enr = record
	}
	p.store.SetPeerData(pid, peerData)
	p.addIpToTracker(pid)
}

// Address returns the multiaddress of the given remote peer.
// This will error if the peer does not exist.
func (p *Status) Address(pid peer.ID) (ma.Multiaddr, error) {
	p.store.RLock()
	defer p.store.RUnlock()

	if peerData, ok := p.store.PeerData(pid); ok {
		return peerData.Address, nil
	}
	return nil, peerdata.ErrPeerUnknown
}

// Direction returns the direction of the given remote peer.
// This will error if the peer does not exist.
func (p *Status) Direction(pid peer.ID) (network.Direction, error) {
	p.store.RLock()
	defer p.store.RUnlock()

	if peerData, ok := p.store.PeerData(pid); ok {
		return peerData.Direction, nil
	}
	return network.DirUnknown, peerdata.ErrPeerUnknown
}

// ENR returns the enr for the corresponding peer id.
func (p *Status) ENR(pid peer.ID) (*enr.Record, error) {
	p.store.RLock()
	defer p.store.RUnlock()

	if peerData, ok := p.store.PeerData(pid); ok {
		return peerData.Enr, nil
	}
	return nil, peerdata.ErrPeerUnknown
}

// IsActive checks if a peers is active and returns the result appropriately.
func (p *Status) IsActive(pid peer.ID) bool {
	p.store.RLock()
	defer p.store.RUnlock()

	peerData, ok := p.store.PeerData(pid)
	return ok && (peerData.ConnState == Connected || peerData.ConnState == Connecting)
}

// IsAboveInboundLimit checks if we are above our current inbound
// peer limit.
func (p *Status) IsAboveInboundLimit() bool {
	p.store.RLock()
	defer p.store.RUnlock()
	totalInbound := 0
	for _, peerData := range p.store.Peers() {
		if peerData.ConnState == Connected &&
			peerData.Direction == network.DirInbound {
			totalInbound += 1
		}
	}
	inboundLimit := int(float64(p.ConnectedPeerLimit()) * InboundRatio)
	return totalInbound > inboundLimit
}

// InboundLimit returns the current inbound
// peer limit.
func (p *Status) InboundLimit() int {
	p.store.RLock()
	defer p.store.RUnlock()
	return int(float64(p.ConnectedPeerLimit()) * InboundRatio)
}

// SetMetadata sets the metadata of the given remote peer.
func (p *Status) SetMetadata(pid peer.ID, metaData metadata.Metadata) {
	p.store.Lock()
	defer p.store.Unlock()

	peerData := p.store.PeerDataGetOrCreate(pid)
	peerData.MetaData = metaData.Copy()
}

// Metadata returns a copy of the metadata corresponding to the provided
// peer id.
func (p *Status) Metadata(pid peer.ID) (metadata.Metadata, error) {
	p.store.RLock()
	defer p.store.RUnlock()

	if peerData, ok := p.store.PeerData(pid); ok {
		if peerData.MetaData == nil || peerData.MetaData.IsNil() {
			return nil, nil
		}
		return peerData.MetaData.Copy(), nil
	}
	return nil, peerdata.ErrPeerUnknown
}

// CommitteeIndices retrieves the committee subnets the peer is subscribed to.
func (p *Status) CommitteeIndices(pid peer.ID) ([]uint64, error) {
	p.store.RLock()
	defer p.store.RUnlock()

	if peerData, ok := p.store.PeerData(pid); ok {
		if peerData.Enr == nil || peerData.MetaData == nil || peerData.MetaData.IsNil() {
			return []uint64{}, nil
		}
		return indicesFromBitfield(peerData.MetaData.AttnetsBitfield()), nil
	}
	return nil, peerdata.ErrPeerUnknown
}

// SubscribedToSubnet retrieves the peers subscribed to the given
// committee subnet.
func (p *Status) SubscribedToSubnet(index uint64) []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()

	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		// look at active peers
		connectedStatus := peerData.ConnState == Connecting || peerData.ConnState == Connected
		if connectedStatus && peerData.MetaData != nil && !peerData.MetaData.IsNil() && peerData.MetaData.AttnetsBitfield() != nil {
			indices := indicesFromBitfield(peerData.MetaData.AttnetsBitfield())
			if slices.Contains(indices, index) {
				peers = append(peers, pid)
			}
		}
	}
	return peers
}

// SetConnectionState sets the connection state of the given remote peer.
func (p *Status) SetConnectionState(pid peer.ID, state peerdata.ConnectionState) {
	p.store.Lock()
	defer p.store.Unlock()

	peerData := p.store.PeerDataGetOrCreate(pid)
	// Stamp connection time on the transition into Connected only, so redundant
	// updates don't reset tenure while reconnects start it afresh.
	if state == Connected && peerData.ConnState != Connected {
		peerData.ConnectedAt = prysmTime.Now()
	}
	peerData.ConnState = state
}

// ConnectionState gets the connection state of the given remote peer.
// This will error if the peer does not exist.
func (p *Status) ConnectionState(pid peer.ID) (peerdata.ConnectionState, error) {
	p.store.RLock()
	defer p.store.RUnlock()

	if peerData, ok := p.store.PeerData(pid); ok {
		return peerData.ConnState, nil
	}
	return Disconnected, peerdata.ErrPeerUnknown
}

// ConnectedAt returns when the peer last transitioned to Connected; zero if never.
// This will error if the peer does not exist.
func (p *Status) ConnectedAt(pid peer.ID) (time.Time, error) {
	p.store.RLock()
	defer p.store.RUnlock()

	if peerData, ok := p.store.PeerData(pid); ok {
		return peerData.ConnectedAt, nil
	}
	return time.Time{}, peerdata.ErrPeerUnknown
}

// SetConnectedAt overrides the peer's connection timestamp. Production code relies on
// SetConnectionState stamping it; this exists for tests that need controlled tenure.
func (p *Status) SetConnectedAt(pid peer.ID, connectedAt time.Time) {
	p.store.Lock()
	defer p.store.Unlock()

	peerData := p.store.PeerDataGetOrCreate(pid)
	peerData.ConnectedAt = connectedAt
}

// IsFromBadIP states if the peer is from an IP exceeding the colocation limit.
func (p *Status) IsFromBadIP(pid peer.ID) error {
	p.store.RLock()
	defer p.store.RUnlock()

	return p.isfromBadIP(pid)
}

// IsPeerGreyListed returns why the peer must be refused: from an IP exceeding the
// colocation limit, or grey-listed by peer scoring. Trusted peers are never refused.
func (p *Status) IsPeerGreyListed(pid peer.ID) error {
	p.store.RLock()
	defer p.store.RUnlock()

	return p.isPeerGreyListed(pid)
}

// isPeerGreyListed is the lock-free version of IsPeerGreyListed.
func (p *Status) isPeerGreyListed(pid peer.ID) error {
	if p.isTrustedPeers(pid) {
		return nil
	}
	if err := p.isfromBadIP(pid); err != nil {
		return &peerscoring.GreyListError{Aspect: peerscoring.AspectBadIP, Err: errors.Wrap(err, "peer is from a bad IP")}
	}
	return p.scoring.IsPeerGreyListed(pid)
}

// NextValidTime gets the earliest possible time it is to contact/dial
// a peer again. This is used to back-off from peers in the event
// they are 'full' or have banned us.
func (p *Status) NextValidTime(pid peer.ID) (time.Time, error) {
	p.store.RLock()
	defer p.store.RUnlock()

	if peerData, ok := p.store.PeerData(pid); ok {
		return peerData.NextValidTime, nil
	}
	return prysmTime.Now(), peerdata.ErrPeerUnknown
}

// SetNextValidTime sets the earliest possible time we are
// able to contact this peer again.
func (p *Status) SetNextValidTime(pid peer.ID, nextTime time.Time) {
	p.store.Lock()
	defer p.store.Unlock()

	peerData := p.store.PeerDataGetOrCreate(pid)
	peerData.NextValidTime = nextTime
}

// RandomizeBackOff adds extra backoff period during which peer won't be dialed.
func (p *Status) RandomizeBackOff(pid peer.ID) {
	p.store.Lock()
	defer p.store.Unlock()

	peerData := p.store.PeerDataGetOrCreate(pid)

	// No need to add backoff period, if the previous one hasn't expired yet.
	if !time.Now().After(peerData.NextValidTime) {
		return
	}

	duration := time.Duration(max(MinBackOffDuration, float64(p.rand.Intn(MaxBackOffDuration)))) * time.Millisecond
	peerData.NextValidTime = time.Now().Add(duration)
}

// IsReadyToDial checks where the given peer is ready to be
// dialed again.
func (p *Status) IsReadyToDial(pid peer.ID) bool {
	p.store.RLock()
	defer p.store.RUnlock()

	if peerData, ok := p.store.PeerData(pid); ok {
		timeIsZero := peerData.NextValidTime.IsZero()
		isInvalidTime := peerData.NextValidTime.After(time.Now())
		return timeIsZero || !isInvalidTime
	}
	// If no record exists, we don't restrict dials to the
	// peer.
	return true
}

// Connecting returns the peers that are connecting.
func (p *Status) Connecting() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.ConnState == Connecting {
			peers = append(peers, pid)
		}
	}
	return peers
}

// Connected returns the peers that are connected.
func (p *Status) Connected() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.ConnState == Connected {
			peers = append(peers, pid)
		}
	}
	return peers
}

// Inbound returns the current batch of inbound peers.
func (p *Status) Inbound() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.Direction == network.DirInbound {
			peers = append(peers, pid)
		}
	}
	return peers
}

// InboundConnected returns the current batch of inbound peers that are connected.
func (p *Status) InboundConnected() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.ConnState == Connected && peerData.Direction == network.DirInbound {
			peers = append(peers, pid)
		}
	}
	return peers
}

// InboundConnectedWithProtocol returns the current batch of inbound peers that are connected with a given protocol.
func (p *Status) InboundConnectedWithProtocol(protocol InternetProtocol) []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.ConnState == Connected && peerData.Direction == network.DirInbound && strings.Contains(peerData.Address.String(), string(protocol)) {
			peers = append(peers, pid)
		}
	}
	return peers
}

// Outbound returns the current batch of outbound peers.
func (p *Status) Outbound() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.Direction == network.DirOutbound {
			peers = append(peers, pid)
		}
	}
	return peers
}

// OutboundConnected returns the current batch of outbound peers that are connected.
func (p *Status) OutboundConnected() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.ConnState == Connected && peerData.Direction == network.DirOutbound {
			peers = append(peers, pid)
		}
	}
	return peers
}

// OutboundConnectedWithProtocol returns the current batch of outbound peers that are connected with a given protocol.
func (p *Status) OutboundConnectedWithProtocol(protocol InternetProtocol) []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.ConnState == Connected && peerData.Direction == network.DirOutbound && strings.Contains(peerData.Address.String(), string(protocol)) {
			peers = append(peers, pid)
		}
	}
	return peers
}

// Active returns the peers that are active (connecting or connected).
func (p *Status) Active() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.ConnState == Connecting || peerData.ConnState == Connected {
			peers = append(peers, pid)
		}
	}
	return peers
}

// Disconnecting returns the peers that are disconnecting.
func (p *Status) Disconnecting() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.ConnState == Disconnecting {
			peers = append(peers, pid)
		}
	}
	return peers
}

// Disconnected returns the peers that are disconnected.
func (p *Status) Disconnected() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.ConnState == Disconnected {
			peers = append(peers, pid)
		}
	}
	return peers
}

// Inactive returns the peers that are inactive (disconnecting or disconnected).
func (p *Status) Inactive() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	peers := make([]peer.ID, 0)
	for pid, peerData := range p.store.Peers() {
		if peerData.ConnState == Disconnecting || peerData.ConnState == Disconnected {
			peers = append(peers, pid)
		}
	}
	return peers
}

// All returns all the peers regardless of state.
func (p *Status) All() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	pids := make([]peer.ID, 0, len(p.store.Peers()))
	for pid := range p.store.Peers() {
		pids = append(pids, pid)
	}
	return pids
}

// Prune clears out and removes outdated and disconnected peers,
// returning the IDs of the pruned peers.
func (p *Status) Prune() []peer.ID {
	p.store.Lock()
	defer p.store.Unlock()

	// Exit early if there is nothing to prune.
	if len(p.store.Peers()) <= p.store.Config().MaxPeers {
		return nil
	}
	// Grey-listed and colocated-IP peers are never pruned: the store is the node's only memory
	// of who misbehaved. Trusted peers pass the composite verdict but are kept by the trusted check.
	notBadPeer := func(pid peer.ID) bool {
		return p.isPeerGreyListed(pid) == nil
	}
	notTrustedPeer := func(pid peer.ID) bool {
		return !p.isTrustedPeers(pid)
	}
	type peerResp struct {
		pid     peer.ID
		strikes int
	}
	peersToPrune := make([]*peerResp, 0)
	// Select disconnected peers with a smaller bad response count.
	for pid, peerData := range p.store.Peers() {
		// Should not prune trusted peer or prune the peer dara and unset trusted peer.
		if peerData.ConnState == Disconnected && notBadPeer(pid) && notTrustedPeer(pid) {
			peersToPrune = append(peersToPrune, &peerResp{
				pid:     pid,
				strikes: p.scoring.BadResponseCount(pid),
			})
		}
	}

	// Sort in ascending strike order, so the peers with the fewest standing
	// strikes are forgotten first and the memory of the worst misbehavers is
	// kept the longest.
	sort.Slice(peersToPrune, func(i, j int) bool {
		return peersToPrune[i].strikes < peersToPrune[j].strikes
	})

	limitDiff := min(len(p.store.Peers())-p.store.Config().MaxPeers, len(peersToPrune))

	peersToPrune = peersToPrune[:limitDiff]

	// Delete peers from the store and drop their scoring state.
	prunedPIDs := make([]peer.ID, 0, len(peersToPrune))
	for _, peerData := range peersToPrune {
		p.store.DeletePeerData(peerData.pid)
		prunedPIDs = append(prunedPIDs, peerData.pid)
	}
	p.scoring.RemovePeers(prunedPIDs)
	p.tallyIPTracker()
	return prunedPIDs
}

// BestFinalized groups all peers by their last known finalized epoch
// and selects the epoch of the largest group as best.
// Any peer with a finalized epoch < ourFinalized is excluded from consideration.
// In the event of a tie in largest group size, the higher epoch is the tie breaker.
// The selected epoch is returned, along with a list of peers with a finalized epoch >= the selected epoch.
func (p *Status) BestFinalized(ourFinalized primitives.Epoch) (primitives.Epoch, []peer.ID) {
	connected := p.Connected()
	pids := make([]peer.ID, 0, len(connected))
	views := make(map[peer.ID]*pb.StatusV2, len(connected))

	votes := make(map[primitives.Epoch]uint64)
	winner := primitives.Epoch(0)
	for _, pid := range connected {
		view, err := p.scoring.PeerStatus(pid)
		if err != nil || view == nil || view.FinalizedEpoch < ourFinalized {
			continue
		}
		pids = append(pids, pid)
		views[pid] = view

		votes[view.FinalizedEpoch]++
		if winner == 0 {
			winner = view.FinalizedEpoch
			continue
		}
		e, v := view.FinalizedEpoch, votes[view.FinalizedEpoch]
		if v > votes[winner] || v == votes[winner] && e > winner {
			winner = e
		}
	}

	// Descending sort by (finalized, head).
	sort.Slice(pids, func(i, j int) bool {
		iv, jv := views[pids[i]], views[pids[j]]
		if iv.FinalizedEpoch == jv.FinalizedEpoch {
			return iv.HeadSlot > jv.HeadSlot
		}

		return iv.FinalizedEpoch > jv.FinalizedEpoch
	})

	// Find the first peer with finalized epoch < winner, trim and all following (lower) peers.
	trim := sort.Search(len(pids), func(i int) bool {
		return views[pids[i]].FinalizedEpoch < winner
	})
	pids = pids[:trim]

	return winner, pids
}

// BestNonFinalized returns the highest known epoch, higher than ours,
// and is shared by at least minPeers.
func (p *Status) BestNonFinalized(minPeers int, ourHeadEpoch primitives.Epoch) (primitives.Epoch, []peer.ID) {
	connected := p.Connected()
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	ourHeadSlot := slotsPerEpoch.Mul(uint64(ourHeadEpoch))

	// key: head epoch, value: number of peers that support this epoch.
	epochVotes := make(map[primitives.Epoch]uint64)

	// key: peer ID, value: head epoch of the peer.
	pidEpoch := make(map[peer.ID]primitives.Epoch, len(connected))

	// key: peer ID, value: head slot of the peer.
	pidHead := make(map[peer.ID]primitives.Slot, len(connected))

	potentialPIDs := make([]peer.ID, 0, len(connected))
	for _, pid := range connected {
		peerChainState, err := p.scoring.PeerStatus(pid)
		// Skip if the peer's head epoch is not defined, or if the peer's head slot is
		// lower or equal than ours.
		if err != nil || peerChainState == nil || peerChainState.HeadSlot <= ourHeadSlot {
			continue
		}

		epoch := slots.ToEpoch(peerChainState.HeadSlot)
		epochVotes[epoch]++
		pidEpoch[pid] = epoch
		pidHead[pid] = peerChainState.HeadSlot
		potentialPIDs = append(potentialPIDs, pid)
	}

	// Select the target epoch, which has enough peers' votes (>= minPeers).
	targetEpoch := primitives.Epoch(0)
	for epoch, votes := range epochVotes {
		if votes >= uint64(minPeers) && targetEpoch < epoch {
			targetEpoch = epoch
		}
	}

	// Sort PIDs by head slot, in decreasing order.
	sort.Slice(potentialPIDs, func(i, j int) bool {
		return pidHead[potentialPIDs[i]] > pidHead[potentialPIDs[j]]
	})

	// Trim potential peers to those on or after target epoch.
	for i, pid := range potentialPIDs {
		if pidEpoch[pid] < targetEpoch {
			potentialPIDs = potentialPIDs[:i]
			break
		}
	}

	return targetEpoch, potentialPIDs
}

// PruneCandidates returns every connected, inbound, non-trusted peer ordered by eviction
// priority, along with how many of them must be disconnected to get back under the
// connection and inbound limits. Grey-listed peers always come first (shuffled); the rest
// are ordered uniformly at random, except that the oldest quarter by connection time is
// moved to the back, youngest-first, so long-lived peers are evicted last. With probability
// pruneTenureEpsilon a round ignores tenure so it can never confer permanent immunity.
// Callers may drop protected candidates (e.g. peers needed for subnet coverage) before
// disconnecting the first numToPrune of the remainder.
func (p *Status) PruneCandidates() ([]peer.ID, uint64) {
	connLimit := p.ConnectedPeerLimit()
	inBoundLimit := uint64(p.InboundLimit())
	numActivePeers := uint64(len(p.Active()))
	numInboundPeers := uint64(len(p.InboundConnected()))

	// Prune the largest amount between excess active peers and excess inbound peers.
	numToPrune := uint64(0)
	if numActivePeers > connLimit {
		numToPrune = numActivePeers - connLimit
	}
	if numInboundPeers > inBoundLimit && numInboundPeers-inBoundLimit > numToPrune {
		numToPrune = numInboundPeers - inBoundLimit
	}
	// Exit early if we are within both limits.
	if numToPrune == 0 {
		return nil, 0
	}

	p.store.Lock()
	defer p.store.Unlock()

	type candidate struct {
		pid         peer.ID
		connectedAt time.Time
	}
	greyListed := make([]peer.ID, 0)
	remainder := make([]candidate, 0)
	// Select connected and inbound peers as prune candidates.
	for pid, peerData := range p.store.Peers() {
		if peerData.ConnState != Connected ||
			peerData.Direction != network.DirInbound || p.store.IsTrustedPeer(pid) {
			continue
		}
		if p.scoring.IsPeerGreyListed(pid) != nil {
			greyListed = append(greyListed, pid)
			continue
		}
		remainder = append(remainder, candidate{pid: pid, connectedAt: peerData.ConnectedAt})
	}

	// Grey-listed peers are always evicted first, in random order.
	p.rand.Shuffle(len(greyListed), func(i, j int) { greyListed[i], greyListed[j] = greyListed[j], greyListed[i] })
	ids := append(make([]peer.ID, 0, len(greyListed)+len(remainder)), greyListed...)

	// Shuffle before the stable sort so equal connection times order randomly, and so the
	// epsilon round below is uniform.
	p.rand.Shuffle(len(remainder), func(i, j int) { remainder[i], remainder[j] = remainder[j], remainder[i] })

	if p.rand.Float64() >= pruneTenureEpsilon {
		// Tenure round: youngest first, so the oldest quarter becomes the list's tail,
		// ordered youngest-first with the absolute oldest peer strictly last.
		sort.SliceStable(remainder, func(i, j int) bool {
			return remainder[i].connectedAt.After(remainder[j].connectedAt)
		})
		unprotected := len(remainder) - len(remainder)/pruneTenureProtectedDenominator
		p.rand.Shuffle(unprotected, func(i, j int) { remainder[i], remainder[j] = remainder[j], remainder[i] })
	}

	for _, c := range remainder {
		ids = append(ids, c.pid)
	}
	return ids, numToPrune
}

// ConnectedPeerLimit returns the peer limit of
// concurrent peers connected to the beacon-node.
func (p *Status) ConnectedPeerLimit() uint64 {
	maxLim := p.MaxPeerLimit()
	if maxLim <= maxLimitBuffer {
		return 0
	}
	return uint64(maxLim) - maxLimitBuffer
}

// SetTrustedPeers sets our trusted peer set into
// our peerstore.
func (p *Status) SetTrustedPeers(peers []peer.ID) {
	p.store.Lock()
	p.store.SetTrustedPeers(peers)
	p.store.Unlock()

	// Notify outside the store lock: the callback may take other locks (e.g. the connmgr's).
	if p.onTrustedPeerAdded != nil {
		for _, pid := range peers {
			p.onTrustedPeerAdded(pid)
		}
	}
}

// GetTrustedPeers returns a list of all trusted peers' ids
func (p *Status) GetTrustedPeers() []peer.ID {
	p.store.RLock()
	defer p.store.RUnlock()
	return p.store.GetTrustedPeers()
}

// DeleteTrustedPeers removes peers from trusted peer set
func (p *Status) DeleteTrustedPeers(peers []peer.ID) {
	p.store.Lock()
	p.store.DeleteTrustedPeers(peers)
	p.store.Unlock()

	// Notify outside the store lock: the callback may take other locks (e.g. the connmgr's).
	if p.onTrustedPeerRemoved != nil {
		for _, pid := range peers {
			p.onTrustedPeerRemoved(pid)
		}
	}
}

// IsTrustedPeers returns if given peer is a Trusted peer
func (p *Status) IsTrustedPeers(pid peer.ID) bool {
	p.store.RLock()
	defer p.store.RUnlock()
	return p.isTrustedPeers(pid)
}

// isTrustedPeers is the lock-free version of IsTrustedPeers.
func (p *Status) isTrustedPeers(pid peer.ID) bool {
	return p.store.IsTrustedPeer(pid)
}

// this method assumes the store lock is acquired before
// executing the method.
func (p *Status) isfromBadIP(pid peer.ID) error {
	peerData, ok := p.store.PeerData(pid)
	if !ok {
		return nil
	}

	if peerData.Address == nil {
		return nil
	}

	ip, err := manet.ToIP(peerData.Address)
	if err != nil {
		return errors.Wrap(err, "to ip")
	}

	if val, ok := p.ipTracker[ip.String()]; ok {
		if val > CollocationLimit {
			// Check if IP is in the whitelist
			for _, ipNet := range p.ipColocationWhitelist {
				if ipNet.Contains(ip) {
					// IP is whitelisted, skip colocation limit check
					return nil
				}
			}
			return errors.Errorf(
				"colocation limit exceeded: got %d - limit %d for peer %v with IP %v",
				val, CollocationLimit, pid, ip.String(),
			)
		}
	}

	return nil
}

func (p *Status) addIpToTracker(pid peer.ID) {
	data, ok := p.store.PeerData(pid)
	if !ok {
		return
	}
	if data.Address == nil {
		return
	}
	ip, err := manet.ToIP(data.Address)
	if err != nil {
		// Should never happen, it is
		// assumed every IP coming in
		// is a valid ip.
		return
	}
	// Ignore loopback addresses.
	if ip.IsLoopback() {
		return
	}
	stringIP := ip.String()
	p.ipTracker[stringIP] += 1
}

func (p *Status) tallyIPTracker() {
	tracker := map[string]uint64{}
	// Iterate through all peers.
	for _, peerData := range p.store.Peers() {
		if peerData.Address == nil {
			continue
		}
		ip, err := manet.ToIP(peerData.Address)
		if err != nil {
			// Should never happen, it is
			// assumed every IP coming in
			// is a valid ip.
			continue
		}
		stringIP := ip.String()
		tracker[stringIP] += 1
	}
	p.ipTracker = tracker
}

func sameIP(firstAddr, secondAddr ma.Multiaddr) bool {
	// Exit early if we do get nil multi-addresses
	if firstAddr == nil || secondAddr == nil {
		return false
	}
	firstIP, err := manet.ToIP(firstAddr)
	if err != nil {
		return false
	}
	secondIP, err := manet.ToIP(secondAddr)
	if err != nil {
		return false
	}
	return firstIP.Equal(secondIP)
}

func indicesFromBitfield(bitV bitfield.Bitvector64) []uint64 {
	committeeIdxs := make([]uint64, 0, bitV.Count())
	for i := range uint64(64) {
		if bitV.BitAt(i) {
			committeeIdxs = append(committeeIdxs, i)
		}
	}
	return committeeIdxs
}
