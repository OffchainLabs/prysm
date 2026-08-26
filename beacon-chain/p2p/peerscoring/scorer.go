package peerscoring

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p/core/peer"
)

var (
	// ErrPeerUnknown is returned when the scorer holds no record for a peer.
	ErrPeerUnknown = errors.New("peer unknown")
	// ErrNoPeerStatus is returned when a known peer has not completed a status exchange yet.
	ErrNoPeerStatus = errors.New("no chain status for peer")
	// ErrPeerGreyListed reports that a peer is grey-listed and must be refused.
	ErrPeerGreyListed = errors.New("peer is grey-listed")
)

// BadResponseSource identifies the call site that reported a bad response.
type BadResponseSource int

const (
	Unknown BadResponseSource = iota
	// SourceDial reports outbound dial failures.
	SourceDial
	// SourceRPCStatus reports faults in the status RPC exchange, inbound or outbound.
	SourceRPCStatus
	// SourceRPCPing reports faults in the ping RPC exchange.
	SourceRPCPing
	// SourceRPCMetadata reports faults in the metadata RPC exchange.
	SourceRPCMetadata
	// SourceRPCRequest reports malformed or excessive inbound RPC requests.
	SourceRPCRequest
	// SourceRPCResponse reports invalid or unreadable outbound RPC responses served to us.
	SourceRPCResponse
	// SourceRateLimit reports rate-limit violations.
	SourceRateLimit
	// SourceGossip reports invalid gossip messages attributed during processing.
	SourceGossip
	// SourceSync reports invalid data discovered by initial sync pipelines.
	SourceSync
	// SourceBackfill reports invalid data discovered by backfill.
	SourceBackfill
	// SourceDAS reports invalid data column sidecars pinpointed by DAS verification.
	SourceDAS
)

// String returns the source's log-friendly name.
func (s BadResponseSource) String() string {
	switch s {
	case SourceDial:
		return "dial"
	case SourceRPCStatus:
		return "rpc-status"
	case SourceRPCPing:
		return "rpc-ping"
	case SourceRPCMetadata:
		return "rpc-metadata"
	case SourceRPCRequest:
		return "rpc-request"
	case SourceRPCResponse:
		return "rpc-response"
	case SourceRateLimit:
		return "rate-limit"
	case SourceGossip:
		return "gossip"
	case SourceSync:
		return "sync"
	case SourceBackfill:
		return "backfill"
	case SourceDAS:
		return "das"
	default:
		return "unknown"
	}
}

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
	lastUpdated     time.Time
}

// PeerScoringInfo holds all per-peer state the scorers judge a peer by.
type PeerScoringInfo struct {
	// badResponseCount is the standing strike count: incremented per strike, decremented by decay.
	badResponseCount int
	// badResponses is the most recent strike history, capped at badResponseHistorySize.
	badResponses []BadResponse

	rpcStatus *RpcStatus

	gossipScore      float64
	behaviourPenalty float64
	topicScores      map[string]*pb.TopicScoreSnapshot
}

// Defaults mirror the production wiring of the legacy scorers service.
const (
	defaultDecayInterval                = time.Hour
	defaultBadResponseGreyListThreshold = 5
	// defaultGossipGreyListThreshold mirrors the PeerScoreThresholds.GraylistThreshold prysm hands
	// to gossipsub (gossip_scoring_params.go); pubsub exports no constant for it, so it is restated here.
	defaultGossipGreyListThreshold = -16000
	// defaultGossipPositiveScoreCap mirrors the PeerScoreParams.TopicScoreCap prysm hands to
	// gossipsub (gossip_scoring_params.go) — the highest positive gossip score a peer can reach;
	// pubsub exports no constant for it, so it is restated here.
	defaultGossipPositiveScoreCap = 32.72
	defaultBadResponseWeight      = 0.3
	defaultPeerStatusWeight       = 0.3
	defaultGossipWeight           = 0.4
	// defaultBadResponseHistorySize caps how many recent strikes are retained per peer.
	defaultBadResponseHistorySize = 25

	// scoreRoundingFactor keeps four decimal digits in scores.
	scoreRoundingFactor = 10000
	// greyListedPeerScore is the composite score reported for a greylisted peer.
	greyListedPeerScore = -float64(math.MaxInt)
)

// Option configures a Scorer.
type Option func(*Scorer)

// WithGossipGreyListThreshold sets the gossip score below which a peer is greylisted.
func WithGossipGreyListThreshold(threshold int) Option {
	return func(s *Scorer) {
		s.params.gossipGreyListThreshold = threshold
	}
}

// WithGossipPositiveScoreCap sets the positive gossip score against which the gossip
// contribution is normalized.
func WithGossipPositiveScoreCap(scoreCap float64) Option {
	return func(s *Scorer) {
		s.params.gossipPositiveScoreCap = scoreCap
	}
}

// WithBadResponseGreyListThreshold sets how many bad responses greylist a peer.
func WithBadResponseGreyListThreshold(threshold int) Option {
	return func(s *Scorer) {
		s.params.badResponseGreyListThreshold = threshold
	}
}

// WithDecayInterval sets how often one bad response per peer is forgiven.
func WithDecayInterval(decayInterval time.Duration) Option {
	return func(s *Scorer) {
		s.params.decayInterval = decayInterval
	}
}

// WithBadResponseHistorySize sets how many recent strikes are retained per peer.
func WithBadResponseHistorySize(n int) Option {
	return func(s *Scorer) {
		s.params.badResponseHistorySize = n
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
	badResponseGreyListThreshold int
	badResponseHistorySize       int
	gossipGreyListThreshold      int
	gossipPositiveScoreCap       float64
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
			badResponseGreyListThreshold: defaultBadResponseGreyListThreshold,
			badResponseHistorySize:       defaultBadResponseHistorySize,
			gossipGreyListThreshold:      defaultGossipGreyListThreshold,
			gossipPositiveScoreCap:       defaultGossipPositiveScoreCap,
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

// RecordBadResponse adds one strike against the peer, tagged with its source and reason,
// and returns the standing (un-decayed) strike count.
func (s *Scorer) RecordBadResponse(pid peer.ID, source BadResponseSource, reason string) int {
	if pid == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	pi := s.getPeerScoringInfo(pid)
	pi.badResponseCount++
	pi.badResponses = append(pi.badResponses, BadResponse{Source: source, Reason: reason, at: time.Now()})
	if excess := len(pi.badResponses) - s.params.badResponseHistorySize; excess > 0 {
		pi.badResponses = append(pi.badResponses[:0], pi.badResponses[excess:]...)
	}
	return pi.badResponseCount
}

// BadResponseCount returns the peer's standing (un-decayed) strike count.
func (s *Scorer) BadResponseCount(pid peer.ID) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pi, ok := s.info[pid]
	if !ok {
		return 0
	}
	return pi.badResponseCount
}

// RemovePeers drops all scoring state for the given peers. Grey-listed peers are retained:
// the scorer is the node's only memory of who misbehaved.
func (s *Scorer) RemovePeers(pids []peer.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pid := range pids {
		if si, ok := s.snapshot(pid); ok && s.isGreyListed(pid, si) == nil {
			delete(s.info, pid)
		}
	}
}

// SetPeerStatus stores the peer's latest status exchange and our validation verdict on it.
func (s *Scorer) SetPeerStatus(pid peer.ID, chainState *pb.StatusV2, validationError error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.getPeerScoringInfo(pid).rpcStatus = &RpcStatus{chainState: chainState, validationError: validationError, lastUpdated: time.Now()}
	if validationError == nil && chainState != nil && chainState.HeadSlot > s.highestKnownHeadSlot {
		s.highestKnownHeadSlot = chainState.HeadSlot
	}
}

// PeerStatus returns the chain status the peer advertised in its last status exchange.
func (s *Scorer) PeerStatus(pid peer.ID) (*pb.StatusV2, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pi, ok := s.info[pid]
	if !ok {
		return nil, ErrPeerUnknown
	}
	if pi.rpcStatus == nil || pi.rpcStatus.chainState == nil {
		return nil, ErrNoPeerStatus
	}
	return pi.rpcStatus.chainState, nil
}

// ChainStateLastUpdated returns when the peer's status was last stored; zero if never.
func (s *Scorer) ChainStateLastUpdated(pid peer.ID) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pi, ok := s.info[pid]
	if !ok || pi.rpcStatus == nil {
		return time.Time{}
	}
	return pi.rpcStatus.lastUpdated
}

// ValidationError returns the validation verdict stored with the peer's last status, if any.
func (s *Scorer) ValidationError(pid peer.ID) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pi, ok := s.info[pid]
	if !ok || pi.rpcStatus == nil {
		return nil
	}
	return pi.rpcStatus.validationError
}

// SetGossipScore mirrors the peer's latest libp2p gossip score, behaviour penalty and topic snapshots.
func (s *Scorer) SetGossipScore(pid peer.ID, gScore, bPenalty float64, topicScores map[string]*pb.TopicScoreSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pi := s.getPeerScoringInfo(pid)
	pi.gossipScore = gScore
	pi.behaviourPenalty = bPenalty
	pi.topicScores = topicScores
}

// GossipScoreUpdate carries one peer's snapshot from the libp2p score inspector.
type GossipScoreUpdate struct {
	Score            float64
	BehaviourPenalty float64
	TopicScores      map[string]*pb.TopicScoreSnapshot
}

// ReconcileGossipScores makes the gossip mirror match a full inspector report: reported peers
// are updated; peers absent from it were purged by libp2p (its retention expired), so their
// gossip state is cleared — lifting any gossip grey-listing — and entries left with no other
// scoring state are dropped.
func (s *Scorer) ReconcileGossipScores(updates map[peer.ID]GossipScoreUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for pid, u := range updates {
		pi := s.getPeerScoringInfo(pid)
		pi.gossipScore = u.Score
		pi.behaviourPenalty = u.BehaviourPenalty
		pi.topicScores = u.TopicScores
	}
	for pid, pi := range s.info {
		if _, ok := updates[pid]; ok {
			continue
		}
		pi.gossipScore = 0
		pi.behaviourPenalty = 0
		pi.topicScores = nil
		if pi.badResponseCount == 0 && len(pi.badResponses) == 0 && pi.rpcStatus == nil {
			delete(s.info, pid)
		}
	}
}

// GossipData returns the mirrored gossip score, behaviour penalty and per-topic snapshots.
func (s *Scorer) GossipData(pid peer.ID) (float64, float64, map[string]*pb.TopicScoreSnapshot) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pi, ok := s.info[pid]
	if !ok {
		return 0, 0, nil
	}
	return pi.gossipScore, pi.behaviourPenalty, pi.topicScores
}

// SetHeadSlot updates our own head slot, the baseline for status scoring.
func (s *Scorer) SetHeadSlot(slot primitives.Slot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ourHeadSlot = slot
}

// HighestHeadSlot returns the highest head slot any tracked peer has advertised.
func (s *Scorer) HighestHeadSlot() primitives.Slot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var highest primitives.Slot
	for _, pi := range s.info {
		if pi.rpcStatus != nil && pi.rpcStatus.chainState != nil && pi.rpcStatus.chainState.HeadSlot > highest {
			highest = pi.rpcStatus.chainState.HeadSlot
		}
	}
	return highest
}

// IsPeerGreyListed returns nil when no registered scorer greylists the peer, or an error
// wrapping ErrPeerGreyListed describing which aspect greylists it and why.
func (s *Scorer) IsPeerGreyListed(pid peer.ID) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	si, ok := s.snapshot(pid)
	if !ok {
		return nil
	}
	return s.isGreyListed(pid, si)
}

// Score returns the composite peer score: greylisted peers score greyListedPeerScore, others
// the weight-normalized sum of scorer contributions rounded to four decimals, unknown peers 0.
func (s *Scorer) Score(pid peer.ID) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	si, ok := s.snapshot(pid)
	if !ok {
		return 0
	}
	if s.isGreyListed(pid, si) != nil {
		return greyListedPeerScore
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

// GreyListedPeers returns every peer currently grey-listed by any scorer.
func (s *Scorer) GreyListedPeers() []peer.ID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	greyListed := make([]peer.ID, 0)
	for pid := range s.info {
		if si, ok := s.snapshot(pid); ok && s.isGreyListed(pid, si) != nil {
			greyListed = append(greyListed, pid)
		}
	}
	return greyListed
}

// isGreyListed returns the first scorer's greylist verdict, nil if none; callers must hold s.mu.
func (s *Scorer) isGreyListed(pid peer.ID, si *scoringInfo) error {
	for _, scorer := range s.scorers {
		if err := scorer.IsPeerGreyListed(pid, si); err != nil {
			return err
		}
	}
	return nil
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
				if pi.badResponseCount > 0 {
					pi.badResponseCount--
				}
			}
			s.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}
