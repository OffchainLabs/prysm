package peerscoring

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

var (
	// ErrPeerUnknown is returned when the scorer holds no record for a peer.
	ErrPeerUnknown = errors.New("peer unknown")
	// ErrNoPeerStatus is returned when a known peer has not completed a status exchange yet.
	ErrNoPeerStatus = errors.New("no chain status for peer")
	// ErrPeerGreyListed reports that a peer is grey-listed and must be refused.
	ErrPeerGreyListed = errors.New("peer is grey-listed")
)

// GreyListError is a grey-list verdict carrying the aspect that fired; it wraps the
// aspect's descriptive error, which in turn wraps ErrPeerGreyListed.
type GreyListError struct {
	Aspect string
	Err    error
}

func (e *GreyListError) Error() string { return e.Err.Error() }

func (e *GreyListError) Unwrap() error { return e.Err }

// AspectFromError returns the aspect recorded in a grey-list verdict, or "unknown" for
// errors that do not carry one.
func AspectFromError(err error) string {
	var glErr *GreyListError
	if errors.As(err, &glErr) {
		return glErr.Aspect
	}
	return "unknown"
}

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
	// defaultBadResponseHistorySize caps how many recent strikes are retained per peer.
	defaultBadResponseHistorySize = 25
	// defaultStatusGreyListTTL bounds how long a terminal status verdict greylists a peer.
	// A refused peer can never re-exchange status to clear the verdict, so without an expiry
	// every wrong-network peer would be retained forever; it mirrors the
	// GoodbyeCodeWrongNetwork dial backoff (sync/rpc_goodbye.go).
	defaultStatusGreyListTTL = 24 * time.Hour
)

// Option configures a Scorer.
type Option func(*Scorer)

// WithGossipGreyListThreshold sets the gossip score below which a peer is greylisted.
func WithGossipGreyListThreshold(threshold int) Option {
	return func(s *Scorer) {
		s.params.gossipGreyListThreshold = threshold
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

// WithStatusGreyListTTL sets how long a terminal status verdict greylists a peer.
func WithStatusGreyListTTL(ttl time.Duration) Option {
	return func(s *Scorer) {
		s.params.statusGreyListTTL = ttl
	}
}

type scoringParams struct {
	decayInterval                time.Duration
	badResponseGreyListThreshold int
	badResponseHistorySize       int
	gossipGreyListThreshold      int
	statusGreyListTTL            time.Duration
}

// Scorer aggregates per-aspect grey-listers into a composite greylist verdict.
type Scorer struct {
	params      *scoringParams
	greyListers []GreyLister

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
			statusGreyListTTL:            defaultStatusGreyListTTL,
		},
		greyListers: []GreyLister{badResponsesScorer{}, rpcStatusScorer{}, gossipScorer{}},
		info:        make(map[peer.ID]*PeerScoringInfo),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// RecordBadResponse adds one strike against the peer, tagged with its source and reason,
// logs the downscore event, and returns the standing (un-decayed) strike count.
func (s *Scorer) RecordBadResponse(pid peer.ID, source BadResponseSource, reason string) int {
	if pid == "" {
		return 0
	}
	badResponsesTotal.WithLabelValues(source.String()).Inc()
	s.mu.Lock()
	pi := s.getPeerScoringInfo(pid)
	pi.badResponseCount++
	pi.badResponses = append(pi.badResponses, BadResponse{Source: source, Reason: reason, at: time.Now()})
	if excess := len(pi.badResponses) - s.params.badResponseHistorySize; excess > 0 {
		pi.badResponses = append(pi.badResponses[:0], pi.badResponses[excess:]...)
	}
	count := pi.badResponseCount
	s.mu.Unlock()

	log.WithFields(logrus.Fields{"peerID": pid, "source": source, "reason": reason, "badResponses": count}).Debug("Downscore peer")
	return count
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

// HighestHeadSlot returns the highest head slot any tracked peer has advertised in a status that passed validation.
func (s *Scorer) HighestHeadSlot() primitives.Slot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var highest primitives.Slot
	for _, pi := range s.info {
		if pi.rpcStatus != nil && pi.rpcStatus.validationError == nil && pi.rpcStatus.chainState != nil && pi.rpcStatus.chainState.HeadSlot > highest {
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

// TimeToWhiteListing estimates how long until the scoring aspects stop grey-listing the peer;
// 0 when none does or when recovery is not time based.
func (s *Scorer) TimeToWhiteListing(pid peer.ID) time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	si, ok := s.snapshot(pid)
	if !ok {
		return 0
	}
	var longest time.Duration
	for _, greyLister := range s.greyListers {
		if d := greyLister.TimeToWhiteListing(pid, si); d > longest {
			longest = d
		}
	}
	return longest
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

// Aspect names for per-aspect grey-list verdicts, shared by metrics and the debug API.
// AspectBadIP is the IP-colocation refusal source, judged by the p2p service outside the scorer.
const (
	AspectBadResponses = "bad_responses"
	AspectPeerStatus   = "peer_status"
	AspectGossip       = "gossip"
	AspectBadIP        = "bad_ip"
)

// GreyListedPeersByAspect returns the peers each scoring aspect currently grey-lists, keyed
// by aspect name. A peer grey-listed by several aspects appears under each of them.
func (s *Scorer) GreyListedPeersByAspect() map[string][]peer.ID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byAspect := make(map[string][]peer.ID)
	for pid := range s.info {
		si, ok := s.snapshot(pid)
		if !ok {
			continue
		}
		for aspect := range s.verdictsByAspect(pid, si) {
			byAspect[aspect] = append(byAspect[aspect], pid)
		}
	}
	return byAspect
}

// Aspects returns the aspect names of the registered scoring grey-listers, in evaluation order.
func (s *Scorer) Aspects() []string {
	aspects := make([]string, 0, len(s.greyListers))
	for _, greyLister := range s.greyListers {
		aspects = append(aspects, greyLister.Aspect())
	}
	return aspects
}

// TrackedPeerCount returns how many peers the scorer currently holds scoring state for.
func (s *Scorer) TrackedPeerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.info)
}

// isGreyListed returns the first grey-lister's verdict tagged with its aspect, nil if
// none; callers must hold s.mu.
func (s *Scorer) isGreyListed(pid peer.ID, si *scoringInfo) error {
	for _, greyLister := range s.greyListers {
		if err := greyLister.IsPeerGreyListed(pid, si); err != nil {
			return &GreyListError{Aspect: greyLister.Aspect(), Err: err}
		}
	}
	return nil
}

// verdictsByAspect returns every firing grey-lister's verdict keyed by aspect, empty if
// none; callers must hold s.mu.
func (s *Scorer) verdictsByAspect(pid peer.ID, si *scoringInfo) map[string]error {
	verdicts := make(map[string]error)
	for _, greyLister := range s.greyListers {
		if err := greyLister.IsPeerGreyListed(pid, si); err != nil {
			verdicts[greyLister.Aspect()] = err
		}
	}
	return verdicts
}

// snapshot bundles a known peer's state for the grey-listers; callers must hold s.mu.
func (s *Scorer) snapshot(pid peer.ID) (*scoringInfo, bool) {
	pi, ok := s.info[pid]
	if !ok {
		return nil, false
	}
	return &scoringInfo{params: s.params, peerInfo: pi}, true
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
