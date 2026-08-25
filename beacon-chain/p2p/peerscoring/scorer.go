package peerscoring

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p/core/peer"
)

// BadResponseSource identifies the subsystem that reported a bad response.
type BadResponseSource int

const (
	Unknown BadResponseSource = iota
)

// BadResponse is a single recorded misbehaviour, kept with its origin for observability.
type BadResponse struct {
	Source BadResponseSource
	Reason string
	at     time.Time
}

// RpcStatus is a peer's last status exchange together with our validation verdict on it.
type RpcStatus struct {
	chainState      *pb.StatusV2
	validationError error
}

// PeerScoringInfo holds all per-peer state the scorers judge a peer by.
type PeerScoringInfo struct {
	badResponses []BadResponse
	nDecays      int

	rpcStatus   *RpcStatus
	gossipScore float64
}

// Defaults mirror the production wiring of the legacy scorers service.
const (
	defaultDecayInterval                = time.Hour
	defaultBadResponseGreylistThreshold = 5
	// defaultGossipGreylistThreshold mirrors the PeerScoreThresholds.GraylistThreshold prysm hands
	// to gossipsub (gossip_scoring_params.go); pubsub exports no constant for it, so it is restated here.
	defaultGossipGreylistThreshold = -16000
	defaultBadResponseWeight       = 0.3
	defaultPeerStatusWeight        = 0.3
	defaultGossipWeight            = 0.4

	// scoreRoundingFactor keeps four decimal digits in scores.
	scoreRoundingFactor = 10000
	// greylistedPeerScore is the composite score reported for a greylisted peer.
	greylistedPeerScore = -float64(math.MaxInt)
)

// Option configures a Scorer.
type Option func(*Scorer)

// WithGossipGreylistThreshold sets the gossip score below which a peer is greylisted.
func WithGossipGreylistThreshold(threshold int) Option {
	return func(s *Scorer) {
		s.params.gossipGreylistThreshold = threshold
	}
}

// WithBadResponseGreylistThreshold sets how many bad responses greylist a peer.
func WithBadResponseGreylistThreshold(threshold int) Option {
	return func(s *Scorer) {
		s.params.badResponseGreylistThreshold = threshold
	}
}

// WithDecayInterval sets how often one bad response per peer is forgiven.
func WithDecayInterval(decayInterval time.Duration) Option {
	return func(s *Scorer) {
		s.params.decayInterval = decayInterval
	}
}

// WithBadResponseWeight sets the bad-responses contribution to the composite score.
func WithBadResponseWeight(w float64) Option {
	return func(s *Scorer) {
		s.params.badResponseWeight = w
	}
}

// WithPeerStatusWeight sets the peer-status contribution to the composite score.
func WithPeerStatusWeight(w float64) Option {
	return func(s *Scorer) {
		s.params.peerStatusWeight = w
	}
}

// WithGossipWeight sets the gossip contribution to the composite score.
func WithGossipWeight(w float64) Option {
	return func(s *Scorer) {
		s.params.gossipWeight = w
	}
}

type scoringParams struct {
	decayInterval                time.Duration
	badResponseGreylistThreshold int
	gossipGreylistThreshold      int
	badResponseWeight            float64
	peerStatusWeight             float64
	gossipWeight                 float64
}

// totalWeight is the normalization denominator for the composite score.
func (p *scoringParams) totalWeight() float64 {
	return p.badResponseWeight + p.peerStatusWeight + p.gossipWeight
}

// Scorer aggregates per-aspect scorers into a composite peer score and greylist verdict.
type Scorer struct {
	params  *scoringParams
	scorers []GreyListerAndScorer

	mu                   sync.RWMutex
	ourHeadSlot          primitives.Slot
	highestKnownHeadSlot primitives.Slot
	info                 map[peer.ID]*PeerScoringInfo
}

// NewScorer creates a Scorer with production defaults, overridable via opts.
func NewScorer(opts ...Option) *Scorer {
	s := &Scorer{
		params: &scoringParams{
			decayInterval:                defaultDecayInterval,
			badResponseGreylistThreshold: defaultBadResponseGreylistThreshold,
			gossipGreylistThreshold:      defaultGossipGreylistThreshold,
			badResponseWeight:            defaultBadResponseWeight,
			peerStatusWeight:             defaultPeerStatusWeight,
			gossipWeight:                 defaultGossipWeight,
		},
		scorers: []GreyListerAndScorer{badResponsesScorer{}, rpcStatusScorer{}, gossipScorer{}},
		info:    make(map[peer.ID]*PeerScoringInfo),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// RecordBadResponse adds one strike against the peer, tagged with its source and reason.
func (s *Scorer) RecordBadResponse(pid peer.ID, source BadResponseSource, reason string) {
	if pid == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	pi := s.getPeerScoringInfo(pid)
	pi.badResponses = append(pi.badResponses, BadResponse{Source: source, Reason: reason, at: time.Now()})
}

// SetPeerStatus stores the peer's latest status exchange and our validation verdict on it.
func (s *Scorer) SetPeerStatus(pid peer.ID, chainState *pb.StatusV2, validationError error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.getPeerScoringInfo(pid).rpcStatus = &RpcStatus{chainState: chainState, validationError: validationError}
	if validationError == nil && chainState != nil && chainState.HeadSlot > s.highestKnownHeadSlot {
		s.highestKnownHeadSlot = chainState.HeadSlot
	}
}

// SetGossipScore mirrors the peer's latest libp2p gossip score.
func (s *Scorer) SetGossipScore(pid peer.ID, gScore float64, _ float64, _ map[string]*pb.TopicScoreSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.getPeerScoringInfo(pid).gossipScore = gScore
}

// SetHeadSlot updates our own head slot, the baseline for status scoring.
func (s *Scorer) SetHeadSlot(slot primitives.Slot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ourHeadSlot = slot
}

// IsPeerGreylisted reports whether any registered scorer greylists the peer.
func (s *Scorer) IsPeerGreylisted(pid peer.ID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	si, ok := s.snapshot(pid)
	if !ok {
		return false
	}
	return s.isGreylisted(pid, si)
}

// Score returns the composite peer score: greylisted peers score greylistedPeerScore, others
// the weight-normalized sum of scorer contributions rounded to four decimals, unknown peers 0.
func (s *Scorer) Score(pid peer.ID) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	si, ok := s.snapshot(pid)
	if !ok {
		return 0
	}
	if s.isGreylisted(pid, si) {
		return greylistedPeerScore
	}
	score := float64(0)
	for _, scorer := range s.scorers {
		score += scorer.Score(pid, si)
	}
	if tw := s.params.totalWeight(); tw > 0 {
		score /= tw
	}
	return math.Round(score*scoreRoundingFactor) / scoreRoundingFactor
}

// isGreylisted reports whether any scorer greylists the peer; callers must hold s.mu.
func (s *Scorer) isGreylisted(pid peer.ID, si *scoringInfo) bool {
	for _, scorer := range s.scorers {
		if scorer.IsPeerGreylisted(pid, si) {
			return true
		}
	}
	return false
}

// snapshot bundles a known peer's state for the scorers; callers must hold s.mu.
func (s *Scorer) snapshot(pid peer.ID) (*scoringInfo, bool) {
	pi, ok := s.info[pid]
	if !ok {
		return nil, false
	}
	return &scoringInfo{
		params:               s.params,
		peerInfo:             pi,
		ourHeadSlot:          s.ourHeadSlot,
		highestKnownHeadSlot: s.highestKnownHeadSlot,
	}, true
}

// getPeerScoringInfo returns the peer's info, creating it if absent; callers must hold s.mu.
func (s *Scorer) getPeerScoringInfo(pid peer.ID) *PeerScoringInfo {
	pi, ok := s.info[pid]
	if !ok {
		pi = &PeerScoringInfo{}
		s.info[pid] = pi
	}
	return pi
}

// Start runs the bad-responses decay loop until ctx is canceled.
func (s *Scorer) Start(ctx context.Context) {
	ticker := time.NewTicker(s.params.decayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			for _, pi := range s.info {
				if effectiveBadResponses(pi) > 0 {
					pi.nDecays++
				}
			}
			s.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}
