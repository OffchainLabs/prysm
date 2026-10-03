package peerscoring

import (
	"fmt"
	"strconv"
	"time"

	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p/core/peer"
)

// GreyListExemptionTrusted marks a trusted peer whose scoring state would otherwise refuse it.
const GreyListExemptionTrusted = "trusted"

// PeerScoringDebugResponse is the debug RPC envelope for one peer's scoring picture.
type PeerScoringDebugResponse struct {
	Data *PeerScoringDebug `json:"data"`
}

// PeersScoringDebugResponse is the debug RPC envelope for the list of peers.
type PeersScoringDebugResponse struct {
	Data []*PeerScoringDebug `json:"data"`
}

// ScoringAgentsResponse is the debug RPC envelope for the per-agent scoring rollup.
type ScoringAgentsResponse struct {
	Data []*AgentScoringDebug `json:"data"`
}

// GossipRejectionsResponse is the debug RPC envelope for the list of gossip rejections.
type GossipRejectionsResponse struct {
	Data []*PeerGossipRejectionDebug `json:"data"`
}

// GossipRejectionsSummaryMeta describes the summary dimensions.
type GossipRejectionsSummaryMeta struct {
	GroupBy         string `json:"group_by"`
	TotalRejections int    `json:"total_rejections"`
}

// GossipRejectionsSummaryResponse is the debug RPC envelope for grouped rejection counts.
type GossipRejectionsSummaryResponse struct {
	Data []*RejectionGroupDebug       `json:"data"`
	Meta *GossipRejectionsSummaryMeta `json:"meta"`
}

// ScoringConfigResponse is the debug RPC envelope for the scoring configuration.
type ScoringConfigResponse struct {
	Data *ScoringConfigDebug `json:"data"`
}

// PeerScoringDebug is the full scoring picture of one peer for the debug RPC.
type PeerScoringDebug struct {
	PeerID string `json:"peer_id"`
	// Agent is the peer's libp2p agent string, kept after libp2p forgets it on disconnection;
	// empty when never learned.
	Agent string `json:"agent,omitempty"`
	// AgentType is the client the agent belongs to; distinct agents share one agent type.
	AgentType       string `json:"agent_type"`
	ConnectionState string `json:"connection_state,omitempty"`
	Direction       string `json:"direction,omitempty"`
	// ConnectedAt is when the peer last transitioned to Connected; empty when never connected.
	ConnectedAt string `json:"connected_at,omitempty"`
	// Tenure is how long the peer has been connected, human-readable; empty when the peer
	// is not currently connected.
	Tenure string `json:"tenure,omitempty"`
	// GreyListed is the node's composite refusal verdict, including non-scoring refusals
	// (bad IP) and the trusted-peer exemption.
	GreyListed bool `json:"grey_listed"`
	// GreyListDetails lists every refusal source that fires for the peer, even when the
	// composite verdict is exempted (trusted peers).
	GreyListDetails *GreyListDetailsDebug `json:"grey_list_details,omitempty"`
	// GreyListExemption is set when a refusal source fires but the peer is exempt from it.
	GreyListExemption string `json:"grey_list_exemption,omitempty"`
	// GreyListRecovery estimates recovery per refusal source; "unknown" means no local estimate.
	GreyListRecovery map[string]string `json:"grey_list_recovery,omitempty"`
	Strikes          StrikesDebug      `json:"strikes"`
	// RpcStatus is nil when the peer never completed a status exchange.
	RpcStatus *RpcStatusDebug `json:"rpc_status,omitempty"`
	Gossip    GossipDebug     `json:"gossip"`
}

// GreyListDetailsDebug carries each refusal source's own verdict, set only when it fires.
type GreyListDetailsDebug struct {
	Strikes    string `json:"strikes,omitempty"`
	PeerStatus string `json:"peer_status,omitempty"`
	Gossip     string `json:"gossip,omitempty"`
	BadIP      string `json:"bad_ip,omitempty"`
}

// StrikesDebug is a peer's standing strike count and recent strike history.
type StrikesDebug struct {
	// StandingCount is the decayed strike count grey-listing is judged by; the history below
	// is a bounded record and can hold fewer entries.
	StandingCount     int           `json:"standing_count"`
	GreyListThreshold int           `json:"grey_list_threshold"`
	History           []StrikeDebug `json:"history"`
}

// StrikeDebug is one recorded strike.
type StrikeDebug struct {
	Source    string `json:"source"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

// RpcStatusDebug is the peer's last status exchange and our validation verdict on it.
type RpcStatusDebug struct {
	// ChainState is nil when no parseable status was stored with the exchange.
	ChainState      *ChainStateDebug `json:"chain_state,omitempty"`
	ValidationError string           `json:"validation_error,omitempty"`
	// LastUpdated is when the most recent status exchange (and its verdict) was recorded.
	LastUpdated string `json:"last_updated"`
}

// ChainStateDebug is the chain view the peer advertised in its last status exchange.
type ChainStateDebug struct {
	ForkDigest            string `json:"fork_digest"`
	FinalizedRoot         string `json:"finalized_root"`
	FinalizedEpoch        string `json:"finalized_epoch"`
	HeadRoot              string `json:"head_root"`
	HeadSlot              string `json:"head_slot"`
	EarliestAvailableSlot string `json:"earliest_available_slot"`
}

// GossipDebug is the mirrored libp2p gossip opinion plus every recorded validator rejection.
type GossipDebug struct {
	Score            float64 `json:"score"`
	BehaviourPenalty float64 `json:"behaviour_penalty"`
	// TopicScores mirrors libp2p's per-topic counters; only populated on request.
	TopicScores map[string]*TopicScoreDebug `json:"topic_scores,omitempty"`
	Rejections  []GossipRejectionDebug      `json:"rejections"`
}

// TopicScoreDebug mirrors libp2p's per-topic gossip counters for one peer.
type TopicScoreDebug struct {
	TimeInMeshMs             uint64  `json:"time_in_mesh_ms"`
	FirstMessageDeliveries   float64 `json:"first_message_deliveries"`
	MeshMessageDeliveries    float64 `json:"mesh_message_deliveries"`
	InvalidMessageDeliveries float64 `json:"invalid_message_deliveries"`
}

// GossipRejectionDebug is one gossip message our topic validators rejected.
type GossipRejectionDebug struct {
	Topic     string `json:"topic"`
	Agent     string `json:"agent"`
	AgentType string `json:"agent_type"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

// PeerGossipRejectionDebug is one rejection with its sending peer, for cross-peer queries.
type PeerGossipRejectionDebug struct {
	PeerID    string `json:"peer_id"`
	Topic     string `json:"topic"`
	Agent     string `json:"agent"`
	AgentType string `json:"agent_type"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

// RejectionGroupDebug is one group of the rejections summary.
type RejectionGroupDebug struct {
	Value string `json:"value"`
	// AgentType is set when grouping by agent.
	AgentType string `json:"agent_type,omitempty"`
	Count     int    `json:"count"`
}

// AgentScoringDebug aggregates the scoring picture of all peers sharing one agent.
type AgentScoringDebug struct {
	Agent                 string         `json:"agent"`
	AgentType             string         `json:"agent_type"`
	PeerCount             int            `json:"peer_count"`
	GreyListedPeerCount   int            `json:"grey_listed_peer_count"`
	StrikesBySource       map[string]int `json:"strikes_by_source,omitempty"`
	GossipRejectionsCount int            `json:"gossip_rejections_count"`
}

// ScoringConfigDebug is the scoring configuration plus the node-side scoring context.
type ScoringConfigDebug struct {
	StrikeGreyListThreshold    int    `json:"strike_grey_list_threshold"`
	StrikeHistorySize          int    `json:"strike_history_size"`
	DecayInterval              string `json:"decay_interval"`
	GossipGreyListThreshold    int    `json:"gossip_grey_list_threshold"`
	StatusGreyListTTL          string `json:"status_grey_list_ttl"`
	MaxGossipRejectionsPerPeer int    `json:"max_gossip_rejections_per_peer"`
	OurHeadSlot                string `json:"our_head_slot"`
	HighestKnownHeadSlot       string `json:"highest_known_head_slot"`
	TrackedPeerCount           int    `json:"tracked_peer_count"`
	PeersWithGossipRejections  int    `json:"peers_with_gossip_rejections"`
}

// PeerDebugOptions carries the caller-supplied context BuildPeerDebug cannot derive itself:
// the composite refusal verdict and the peer registry facts (peerscoring is leaf-like and
// must not call back into the p2p service or peer store).
type PeerDebugOptions struct {
	IncludeTopicScores bool
	// Trusted reports whether the peer is in the trusted set (exempt from refusal).
	Trusted bool
	// GreyListed is the node's composite refusal verdict (p2p.Service.IsPeerGreyListed != nil).
	GreyListed bool
	// Tenure is how long the peer has been connected; 0 when not currently connected.
	Tenure          time.Duration
	Direction       string
	ConnectionState string
	Agent           string
	// BadIPError is the IP-colocation verdict (peers.Status.IsFromBadIP); nil means clean.
	BadIPError error
	// ConnectedAt is when the peer last transitioned to Connected; zero when unknown.
	ConnectedAt time.Time
}

// FlatRejection is one recorded rejection tagged with its sending peer.
type FlatRejection struct {
	PeerID peer.ID
	GossipRejection
}

// BuildPeerDebug assembles the debug model for one peer. rejections may be nil.
func BuildPeerDebug(pid peer.ID, opts PeerDebugOptions, scorer *Scorer, rejections *GossipRejectionsStore) *PeerScoringDebug {
	d := scorer.debugInfo(pid, opts.IncludeTopicScores)
	switch {
	case opts.Agent != "":
		d.Agent = opts.Agent
		d.AgentType = AgentTypeOf(opts.Agent)
	case d.AgentType == "":
		d.AgentType = AgentTypeUnknown
	}
	d.ConnectionState = opts.ConnectionState
	d.Direction = opts.Direction
	if !opts.ConnectedAt.IsZero() {
		d.ConnectedAt = debugTime(opts.ConnectedAt)
	}
	if opts.Tenure > 0 {
		d.Tenure = opts.Tenure.Truncate(time.Second).String()
	}
	if opts.BadIPError != nil {
		if d.GreyListDetails == nil {
			d.GreyListDetails = &GreyListDetailsDebug{}
		}
		d.GreyListDetails.BadIP = opts.BadIPError.Error()
		if d.GreyListRecovery == nil {
			d.GreyListRecovery = make(map[string]string)
		}
		d.GreyListRecovery[AspectBadIP] = "unknown"
	}
	d.GreyListed = opts.GreyListed
	if !d.GreyListed {
		d.GreyListRecovery = nil
	}
	if opts.Trusted && !opts.GreyListed && d.GreyListDetails != nil {
		d.GreyListExemption = GreyListExemptionTrusted
	}
	if rejections != nil {
		d.Gossip.Rejections = rejections.debugRejections(pid)
	}
	return d
}

// BuildScoringConfig assembles the scoring configuration and context. rejections may be nil.
func BuildScoringConfig(scorer *Scorer, rejections *GossipRejectionsStore) *ScoringConfigDebug {
	scorer.mu.RLock()
	c := &ScoringConfigDebug{
		StrikeGreyListThreshold: scorer.params.strikeGreyListThreshold,
		StrikeHistorySize:       scorer.params.strikeHistorySize,
		DecayInterval:           scorer.params.decayInterval.String(),
		GossipGreyListThreshold: scorer.params.gossipGreyListThreshold,
		StatusGreyListTTL:       scorer.params.statusGreyListTTL.String(),
		OurHeadSlot:             strconv.FormatUint(uint64(scorer.ourHeadSlot), 10),
		HighestKnownHeadSlot:    strconv.FormatUint(uint64(scorer.highestKnownHeadSlot), 10),
		TrackedPeerCount:        len(scorer.info),
	}
	scorer.mu.RUnlock()

	if rejections != nil {
		rejections.mu.RLock()
		c.MaxGossipRejectionsPerPeer = rejections.maxPerPeer
		c.PeersWithGossipRejections = len(rejections.rejections)
		rejections.mu.RUnlock()
	}
	return c
}

// StrikeSourceNames returns the names of all known strike sources.
func StrikeSourceNames() []string {
	names := make([]string, 0, int(SourceDAS)+1)
	for s := Unknown; s <= SourceDAS; s++ {
		names = append(names, s.String())
	}
	return names
}

// debugInfo builds every scorer-derived field of the debug model under one lock.
func (s *Scorer) debugInfo(pid peer.ID, includeTopicScores bool) *PeerScoringDebug {
	s.mu.RLock()
	defer s.mu.RUnlock()

	d := &PeerScoringDebug{
		PeerID:  pid.String(),
		Strikes: StrikesDebug{GreyListThreshold: s.params.strikeGreyListThreshold, History: []StrikeDebug{}},
		Gossip:  GossipDebug{Rejections: []GossipRejectionDebug{}},
	}
	pi, tracked := s.info[pid]
	if !tracked {
		pi = &PeerScoringInfo{}
	}
	si := &scoringInfo{params: s.params, peerInfo: pi}

	// Per-aspect grey-list verdicts, kept individually so every firing aspect is visible.
	if verdicts := s.verdictsByAspect(pid, si); len(verdicts) > 0 {
		details := &GreyListDetailsDebug{}
		for aspect, verdict := range verdicts {
			switch aspect {
			case AspectStrikes:
				details.Strikes = verdict.Error()
			case AspectPeerStatus:
				details.PeerStatus = verdict.Error()
			case AspectGossip:
				details.Gossip = verdict.Error()
			}
		}
		d.GreyListDetails = details

		d.GreyListRecovery = make(map[string]string, len(verdicts))
		for _, greyLister := range s.greyListers {
			aspect := greyLister.Aspect()
			if verdicts[aspect] == nil {
				continue
			}
			d.GreyListRecovery[aspect] = "unknown"
			if ttw := greyLister.TimeToWhiteListing(pid, si); ttw > 0 {
				d.GreyListRecovery[aspect] = ttw.String()
			}
		}
	}

	d.Agent = pi.agent
	d.AgentType = pi.agentType
	d.Strikes.StandingCount = pi.strikeCount
	for _, strike := range pi.strikes {
		d.Strikes.History = append(d.Strikes.History, StrikeDebug{
			Source:    strike.Source.String(),
			Reason:    strike.Reason,
			Timestamp: debugTime(strike.at),
		})
	}
	if rs := pi.rpcStatus; rs != nil {
		st := &RpcStatusDebug{LastUpdated: debugTime(rs.verdictAt)}
		if rs.validationError != nil {
			st.ValidationError = rs.validationError.Error()
		}
		if cs := rs.chainState; cs != nil {
			st.ChainState = &ChainStateDebug{
				ForkDigest:            fmt.Sprintf("%#x", cs.ForkDigest),
				FinalizedRoot:         fmt.Sprintf("%#x", cs.FinalizedRoot),
				FinalizedEpoch:        strconv.FormatUint(uint64(cs.FinalizedEpoch), 10),
				HeadRoot:              fmt.Sprintf("%#x", cs.HeadRoot),
				HeadSlot:              strconv.FormatUint(uint64(cs.HeadSlot), 10),
				EarliestAvailableSlot: strconv.FormatUint(uint64(cs.EarliestAvailableSlot), 10),
			}
		}
		d.RpcStatus = st
	}
	d.Gossip.Score = pi.gossipScore
	d.Gossip.BehaviourPenalty = pi.behaviourPenalty
	if includeTopicScores && len(pi.topicScores) > 0 {
		d.Gossip.TopicScores = buildTopicScores(pi.topicScores)
	}
	return d
}

// buildTopicScores converts the mirrored libp2p per-topic snapshots to the debug model.
func buildTopicScores(snapshots map[string]*pb.TopicScoreSnapshot) map[string]*TopicScoreDebug {
	out := make(map[string]*TopicScoreDebug, len(snapshots))
	for topic, snap := range snapshots {
		if snap == nil {
			continue
		}
		out[topic] = &TopicScoreDebug{
			TimeInMeshMs:             snap.TimeInMesh,
			FirstMessageDeliveries:   float64(snap.FirstMessageDeliveries),
			MeshMessageDeliveries:    float64(snap.MeshMessageDeliveries),
			InvalidMessageDeliveries: float64(snap.InvalidMessageDeliveries),
		}
	}
	return out
}

// TrackedPeers returns every peer the scorer holds state for.
func (s *Scorer) TrackedPeers() []peer.ID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pids := make([]peer.ID, 0, len(s.info))
	for pid := range s.info {
		pids = append(pids, pid)
	}
	return pids
}

// debugRejections returns the peer's recorded rejections as debug entries, oldest first.
func (s *GossipRejectionsStore) debugRejections(pid peer.ID) []GossipRejectionDebug {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]GossipRejectionDebug, 0, len(s.rejections[pid]))
	for _, rj := range s.rejections[pid] {
		out = append(out, GossipRejectionDebug{Topic: rj.Topic, Agent: rj.Agent, AgentType: rj.AgentType, Reason: rj.Reason, Timestamp: debugTime(rj.At)})
	}
	return out
}

// TrackedPeers returns every peer with recorded rejections.
func (s *GossipRejectionsStore) TrackedPeers() []peer.ID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pids := make([]peer.ID, 0, len(s.rejections))
	for pid := range s.rejections {
		pids = append(pids, pid)
	}
	return pids
}

// FlatRejections returns every recorded rejection tagged with its peer, oldest first per peer.
func (s *GossipRejectionsStore) FlatRejections() []FlatRejection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]FlatRejection, 0, len(s.rejections))
	for pid, entries := range s.rejections {
		for _, rj := range entries {
			out = append(out, FlatRejection{PeerID: pid, GossipRejection: rj})
		}
	}
	return out
}

// debugTime renders timestamps for the debug RPC.
func debugTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
