package peerscoring

import (
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// defaultMaxRejectionsPerPeer caps how many rejections are retained per peer.
	defaultMaxRejectionsPerPeer = 100
	// maxRejectionReasonLen truncates oversized rejection reasons.
	maxRejectionReasonLen = 512
	// unspecifiedRejectionReason is recorded when a validator rejects without an error.
	unspecifiedRejectionReason = "unspecified"
)

// GossipRejection is one gossip message rejected by our topic validators.
type GossipRejection struct {
	Topic  string
	Agent  string
	Reason string
	At     time.Time
}

// PeerGossipRejections groups a peer's recorded rejections by agent and by topic, oldest first.
type PeerGossipRejections struct {
	ByAgent map[string][]GossipRejection
	ByTopic map[string][]GossipRejection
}

// GossipRejectionsStore records gossip messages our topic validators rejected, per peer, to debug
// which peers send invalid messages on which topics and why. It is pure observability: nothing
// here feeds scoring or grey-listing. Only the most recent maxPerPeer rejections are retained per
// peer, and the p2p service removes peers pruned from the peer store.
type GossipRejectionsStore struct {
	mu         sync.RWMutex
	maxPerPeer int
	rejections map[peer.ID][]GossipRejection
}

// GossipRejectionsOption configures a GossipRejectionsStore.
type GossipRejectionsOption func(*GossipRejectionsStore)

// WithMaxRejectionsPerPeer caps how many rejections are retained per peer.
func WithMaxRejectionsPerPeer(n int) GossipRejectionsOption {
	return func(s *GossipRejectionsStore) {
		s.maxPerPeer = n
	}
}

// NewGossipRejectionsStore creates a store with default bounds, overridable via opts.
func NewGossipRejectionsStore(opts ...GossipRejectionsOption) *GossipRejectionsStore {
	s := &GossipRejectionsStore{
		maxPerPeer: defaultMaxRejectionsPerPeer,
		rejections: make(map[peer.ID][]GossipRejection),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Record stores one rejected gossip message from the peer.
func (s *GossipRejectionsStore) Record(pid peer.ID, topic, agent string, rejectionErr error) {
	if pid == "" {
		return
	}
	reason := unspecifiedRejectionReason
	if rejectionErr != nil {
		reason = rejectionErr.Error()
	}
	if len(reason) > maxRejectionReasonLen {
		reason = reason[:maxRejectionReasonLen]
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := append(s.rejections[pid], GossipRejection{Topic: topic, Agent: agent, Reason: reason, At: time.Now()})
	if excess := len(entries) - s.maxPerPeer; excess > 0 {
		entries = append(entries[:0], entries[excess:]...)
	}
	s.rejections[pid] = entries
}

// Rejections returns the peer's recorded rejections grouped by agent and by topic; the zero
// value for unknown peers.
func (s *GossipRejectionsStore) Rejections(pid peer.ID) PeerGossipRejections {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return groupRejections(s.rejections[pid])
}

// All returns every tracked peer's recorded rejections, grouped by agent and by topic.
func (s *GossipRejectionsStore) All() map[peer.ID]PeerGossipRejections {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := make(map[peer.ID]PeerGossipRejections, len(s.rejections))
	for pid, entries := range s.rejections {
		all[pid] = groupRejections(entries)
	}
	return all
}

// RemovePeers drops all recorded rejections for the given peers.
func (s *GossipRejectionsStore) RemovePeers(pids []peer.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pid := range pids {
		delete(s.rejections, pid)
	}
}

// groupRejections builds the by-agent and by-topic views as copies, safe outside the store's lock.
func groupRejections(entries []GossipRejection) PeerGossipRejections {
	if len(entries) == 0 {
		return PeerGossipRejections{}
	}
	pr := PeerGossipRejections{
		ByAgent: make(map[string][]GossipRejection),
		ByTopic: make(map[string][]GossipRejection),
	}
	for _, r := range entries {
		pr.ByAgent[r.Agent] = append(pr.ByAgent[r.Agent], r)
		pr.ByTopic[r.Topic] = append(pr.ByTopic[r.Topic], r)
	}
	return pr
}
