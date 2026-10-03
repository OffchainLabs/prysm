package node

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peerscoring"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	corenet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const agentUnknown = "unknown"

// GetPeerScoring returns one peer's full scoring debug picture: connection time and tenure,
// strikes (source, reason), rpc status incl. the chain validation error, the mirrored
// gossip score with every recorded gossip rejection, and every firing grey-list verdict
// with the time remaining. Optional: include_topic_scores=true adds the per-topic gossip
// counters.
func (s *Server) GetPeerScoring(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.GetPeerScoring")
	defer span.End()

	pid, err := peer.Decode(r.PathValue("peer_id"))
	if err != nil {
		httputil.HandleError(w, "Could not decode peer id: "+err.Error(), http.StatusBadRequest)
		return
	}
	topicScores, err := parseBoolFlag(r, "include_topic_scores")
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	data := peerscoring.BuildPeerDebug(pid, s.peerDebugOptions(pid, topicScores), s.PeerScoringFetcher.PeerScoring(), s.GossipRejectionsFetcher.GossipRejections())
	httputil.WriteJson(w, &peerscoring.PeerScoringDebugResponse{Data: data})
}

// ListPeersScoring returns the scoring debug picture of every known peer, connected or not:
// every peer in the peer store plus every peer with recorded scoring or gossip-rejection state.
// Filters: state=connecting|connected|disconnecting|disconnected (repeatable, case-insensitive;
// absent = all, as in /eth/v1/node/peers), greylisted=true|false (absent = both),
// agent=<substring, case-insensitive> (empty = all), agent_type=<agent type> (case-insensitive;
// absent = all), source=<strike source>.
// Optional include_topic_scores=true adds the per-topic gossip counters to every entry.
// Sorted by sort=strikes (grey-listed first, then standing strike count descending,
// default) or sort=peer_id.
func (s *Server) ListPeersScoring(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.ListPeersScoring")
	defer span.End()

	states, err := parseStates(r)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}
	greyListed, err := parseTriStateBool(r, "greylisted")
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}
	topicScores, err := parseBoolFlag(r, "include_topic_scores")
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}
	agentFilter := r.URL.Query().Get("agent")
	agentTypeFilter, err := parseAgentType(r)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}
	sourceFilter := r.URL.Query().Get("source")
	if sourceFilter != "" && !slices.Contains(peerscoring.StrikeSourceNames(), sourceFilter) {
		httputil.HandleError(w, fmt.Sprintf("Invalid source %q, expected one of: %s", sourceFilter, strings.Join(peerscoring.StrikeSourceNames(), ", ")), http.StatusBadRequest)
		return
	}
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "strikes"
	}
	if sortBy != "strikes" && sortBy != "peer_id" {
		httputil.HandleError(w, fmt.Sprintf("Invalid sort %q, expected strikes or peer_id", sortBy), http.StatusBadRequest)
		return
	}

	entries := make([]*peerscoring.PeerScoringDebug, 0)
	for _, d := range s.buildAllPeersDebug(topicScores) {
		if len(states) > 0 && !states[d.ConnectionState] {
			continue
		}
		if greyListed != nil && d.GreyListed != *greyListed {
			continue
		}
		if agentFilter != "" && !strings.Contains(strings.ToLower(d.Agent), strings.ToLower(agentFilter)) {
			continue
		}
		if agentTypeFilter != "" && d.AgentType != agentTypeFilter {
			continue
		}
		if sourceFilter != "" && !hasStrikeFromSource(d, sourceFilter) {
			continue
		}
		entries = append(entries, d)
	}
	slices.SortFunc(entries, func(a, b *peerscoring.PeerScoringDebug) int {
		if sortBy == "strikes" {
			if a.GreyListed != b.GreyListed {
				if a.GreyListed {
					return -1
				}
				return 1
			}
			if a.Strikes.StandingCount != b.Strikes.StandingCount {
				return b.Strikes.StandingCount - a.Strikes.StandingCount
			}
		}
		return strings.Compare(a.PeerID, b.PeerID)
	})

	httputil.WriteJson(w, &peerscoring.PeersScoringDebugResponse{Data: entries})
}

// ListScoringAgents returns the scoring picture aggregated per agent, each tagged with its agent
// type: peer counts, grey-list counts, strikes by source, and rejection counts. Filter:
// agent_type=<agent type> (case-insensitive; absent = all). Sorted by peer count descending.
func (s *Server) ListScoringAgents(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.ListScoringAgents")
	defer span.End()

	agentTypeFilter, err := parseAgentType(r)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	groups := make(map[string]*peerscoring.AgentScoringDebug)
	for _, d := range s.buildAllPeersDebug(false) {
		if agentTypeFilter != "" && d.AgentType != agentTypeFilter {
			continue
		}
		agent := d.Agent
		if agent == "" {
			agent = agentUnknown
		}
		g, ok := groups[agent]
		if !ok {
			g = &peerscoring.AgentScoringDebug{Agent: agent, AgentType: d.AgentType}
			groups[agent] = g
		}
		g.PeerCount++
		if d.GreyListed {
			g.GreyListedPeerCount++
		}
		for _, strike := range d.Strikes.History {
			if g.StrikesBySource == nil {
				g.StrikesBySource = make(map[string]int)
			}
			g.StrikesBySource[strike.Source]++
		}
		g.GossipRejectionsCount += len(d.Gossip.Rejections)
	}
	entries := make([]*peerscoring.AgentScoringDebug, 0, len(groups))
	for _, g := range groups {
		entries = append(entries, g)
	}
	slices.SortFunc(entries, func(a, b *peerscoring.AgentScoringDebug) int {
		if a.PeerCount != b.PeerCount {
			return b.PeerCount - a.PeerCount
		}
		return strings.Compare(a.Agent, b.Agent)
	})

	httputil.WriteJson(w, &peerscoring.ScoringAgentsResponse{Data: entries})
}

// GetPeerScoringConfig returns the scoring configuration (thresholds, decay, history size)
// plus the node-side scoring context (our head slot, highest known head slot, peer counts).
func (s *Server) GetPeerScoringConfig(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.GetPeerScoringConfig")
	defer span.End()

	data := peerscoring.BuildScoringConfig(s.PeerScoringFetcher.PeerScoring(), s.GossipRejectionsFetcher.GossipRejections())
	httputil.WriteJson(w, &peerscoring.ScoringConfigResponse{Data: data})
}

// ListGossipRejections returns every currently retained gossip rejection across all peers,
// newest first. Filters: topic=<substring>, agent=<substring> (both case-insensitive, empty =
// all), peer_id=<peer id>, since=<RFC3339 time>.
func (s *Server) ListGossipRejections(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.ListGossipRejections")
	defer span.End()

	q := r.URL.Query()
	topicFilter := strings.ToLower(q.Get("topic"))
	agentFilter := strings.ToLower(q.Get("agent"))
	var peerFilter peer.ID
	if pidStr := q.Get("peer_id"); pidStr != "" {
		pid, err := peer.Decode(pidStr)
		if err != nil {
			httputil.HandleError(w, "Could not decode peer id: "+err.Error(), http.StatusBadRequest)
			return
		}
		peerFilter = pid
	}
	var since time.Time
	if sinceStr := q.Get("since"); sinceStr != "" {
		t, err := time.Parse(time.RFC3339Nano, sinceStr)
		if err != nil {
			httputil.HandleError(w, "Could not parse since as RFC3339 time: "+err.Error(), http.StatusBadRequest)
			return
		}
		since = t
	}

	entries := make([]peerscoring.FlatRejection, 0)
	for _, rj := range s.GossipRejectionsFetcher.GossipRejections().FlatRejections() {
		if topicFilter != "" && !strings.Contains(strings.ToLower(rj.Topic), topicFilter) {
			continue
		}
		if agentFilter != "" && !strings.Contains(strings.ToLower(rj.Agent), agentFilter) {
			continue
		}
		if peerFilter != "" && rj.PeerID != peerFilter {
			continue
		}
		if !since.IsZero() && rj.At.Before(since) {
			continue
		}
		entries = append(entries, rj)
	}
	slices.SortFunc(entries, func(a, b peerscoring.FlatRejection) int {
		if !a.At.Equal(b.At) {
			if a.At.After(b.At) {
				return -1
			}
			return 1
		}
		return strings.Compare(a.PeerID.String(), b.PeerID.String())
	})

	data := make([]*peerscoring.PeerGossipRejectionDebug, 0, len(entries))
	for _, rj := range entries {
		data = append(data, &peerscoring.PeerGossipRejectionDebug{
			PeerID:    rj.PeerID.String(),
			Topic:     rj.Topic,
			Agent:     rj.Agent,
			AgentType: rj.AgentType,
			Reason:    rj.Reason,
			Timestamp: rj.At.UTC().Format(time.RFC3339Nano),
		})
	}
	httputil.WriteJson(w, &peerscoring.GossipRejectionsResponse{Data: data})
}

// GetGossipRejectionsSummary returns retained gossip rejections counted per group_by=topic
// (default) | agent | agent_type | reason | peer, largest group first.
func (s *Server) GetGossipRejectionsSummary(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.GetGossipRejectionsSummary")
	defer span.End()

	groupBy := r.URL.Query().Get("group_by")
	if groupBy == "" {
		groupBy = "topic"
	}
	if !slices.Contains([]string{"topic", "agent", "agent_type", "reason", "peer"}, groupBy) {
		httputil.HandleError(w, fmt.Sprintf("Invalid group_by %q, expected topic, agent, agent_type, reason or peer", groupBy), http.StatusBadRequest)
		return
	}

	groups := make(map[string]*peerscoring.RejectionGroupDebug)
	totalRejections := 0
	for _, rj := range s.GossipRejectionsFetcher.GossipRejections().FlatRejections() {
		totalRejections++
		var key, agentType string
		switch groupBy {
		case "topic":
			key = rj.Topic
		case "agent":
			key = rj.Agent
			if key == "" {
				key = agentUnknown
			}
			agentType = rj.AgentType
		case "agent_type":
			key = rj.AgentType
		case "reason":
			key = rj.Reason
		case "peer":
			key = rj.PeerID.String()
		}
		g, ok := groups[key]
		if !ok {
			g = &peerscoring.RejectionGroupDebug{Value: key, AgentType: agentType}
			groups[key] = g
		}
		g.Count++
	}
	entries := make([]*peerscoring.RejectionGroupDebug, 0, len(groups))
	for _, g := range groups {
		entries = append(entries, g)
	}
	slices.SortFunc(entries, func(a, b *peerscoring.RejectionGroupDebug) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Value, b.Value)
	})

	httputil.WriteJson(w, &peerscoring.GossipRejectionsSummaryResponse{
		Data: entries,
		Meta: &peerscoring.GossipRejectionsSummaryMeta{GroupBy: groupBy, TotalRejections: totalRejections},
	})
}

// buildAllPeersDebug assembles the debug model for every peer in the peer store, connected or
// not, plus every peer with scoring or rejection state.
func (s *Server) buildAllPeersDebug(topicScores bool) []*peerscoring.PeerScoringDebug {
	scorer := s.PeerScoringFetcher.PeerScoring()
	rejections := s.GossipRejectionsFetcher.GossipRejections()

	pids := make(map[peer.ID]struct{})
	for _, pid := range scorer.TrackedPeers() {
		pids[pid] = struct{}{}
	}
	for _, pid := range rejections.TrackedPeers() {
		pids[pid] = struct{}{}
	}
	for _, pid := range s.PeersFetcher.Peers().All() {
		pids[pid] = struct{}{}
	}
	all := make([]*peerscoring.PeerScoringDebug, 0, len(pids))
	for pid := range pids {
		all = append(all, peerscoring.BuildPeerDebug(pid, s.peerDebugOptions(pid, topicScores), scorer, rejections))
	}
	return all
}

// peerDebugOptions gathers the composite refusal verdict and peer registry facts for one peer.
func (s *Server) peerDebugOptions(pid peer.ID, topicScores bool) peerscoring.PeerDebugOptions {
	peerStatus := s.PeersFetcher.Peers()
	opts := peerscoring.PeerDebugOptions{
		GreyListed:         s.PeerGreyLister.IsPeerGreyListed(pid) != nil,
		BadIPError:         peerStatus.IsFromBadIP(pid),
		Trusted:            peerStatus.IsTrustedPeers(pid),
		Agent:              s.peerAgent(pid),
		ConnectionState:    eth.ConnectionState(corenet.NotConnected).String(),
		Direction:          eth.PeerDirection(corenet.DirUnknown).String(),
		IncludeTopicScores: topicScores,
	}
	connected := false
	if connState, err := peerStatus.ConnectionState(pid); err == nil {
		opts.ConnectionState = eth.ConnectionState(connState).String()
		connected = connState == peers.Connected
	}
	if direction, err := peerStatus.Direction(pid); err == nil {
		opts.Direction = eth.PeerDirection(direction).String()
	}
	if connectedAt, err := peerStatus.ConnectedAt(pid); err == nil && !connectedAt.IsZero() {
		opts.ConnectedAt = connectedAt
		if connected {
			opts.Tenure = time.Since(connectedAt)
		}
	}
	return opts
}

// peerAgent reads the peer's agent string from the libp2p peerstore; empty when unknown.
func (s *Server) peerAgent(pid peer.ID) string {
	if s.PeerManager == nil {
		return ""
	}
	host := s.PeerManager.Host()
	if host == nil {
		return ""
	}
	raw, err := host.Peerstore().Get(pid, "AgentVersion")
	if err != nil {
		return ""
	}
	agent, ok := raw.(string)
	if !ok {
		return ""
	}
	return agent
}

// hasStrikeFromSource reports whether the peer's retained strike history has the source.
func hasStrikeFromSource(d *peerscoring.PeerScoringDebug, source string) bool {
	for _, strike := range d.Strikes.History {
		if strike.Source == source {
			return true
		}
	}
	return false
}

// parseBoolFlag reads a boolean query flag; absent or empty means false.
func parseBoolFlag(r *http.Request, name string) (bool, error) {
	switch v := r.URL.Query().Get(name); v {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, fmt.Errorf("invalid %s %q, expected true or false", name, v)
	}
}

// parseStates reads the repeatable, case-insensitive state filter as upper-case connection
// states; empty means no filtering.
func parseStates(r *http.Request) (map[string]bool, error) {
	states := make(map[string]bool)
	for _, v := range r.URL.Query()["state"] {
		if v == "" {
			continue
		}
		state := strings.ToUpper(v)
		if _, ok := eth.ConnectionState_value[state]; !ok {
			return nil, fmt.Errorf("invalid state %q, expected connecting, connected, disconnecting or disconnected", v)
		}
		states[state] = true
	}
	return states, nil
}

// parseAgentType reads the case-insensitive agent_type filter; empty means no filtering.
func parseAgentType(r *http.Request) (string, error) {
	agentType := strings.ToLower(r.URL.Query().Get("agent_type"))
	if agentType != "" && !slices.Contains(peerscoring.AgentTypes(), agentType) {
		return "", fmt.Errorf("invalid agent_type %q, expected one of: %s", agentType, strings.Join(peerscoring.AgentTypes(), ", "))
	}
	return agentType, nil
}

// parseTriStateBool reads a boolean query filter; absent or empty means no filtering.
func parseTriStateBool(r *http.Request, name string) (*bool, error) {
	switch v := r.URL.Query().Get(name); v {
	case "":
		return nil, nil
	case "true", "false":
		b := v == "true"
		return &b, nil
	default:
		return nil, fmt.Errorf("invalid %s %q, expected true or false", name, v)
	}
}
