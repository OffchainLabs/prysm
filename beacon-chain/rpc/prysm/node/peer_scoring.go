package node

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
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

const (
	defaultPageLimit = 50
	maxPageLimit     = 500
	agentUnknown     = "unknown"
)

// GetPeerScoring returns one peer's full scoring debug picture: connection time and tenure,
// bad responses (source, reason), rpc status incl. the chain validation error, the mirrored
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

// ListPeersScoring returns the scoring debug picture of every peer with recorded scoring or
// gossip-rejection state plus every connected peer. Filters: greylisted=true|false (absent =
// both), agent=<substring, case-insensitive> (empty = all), source=<bad-response source>.
// Optional include_topic_scores=true adds the per-topic gossip counters to every entry.
// Sorted by sort=bad_responses (grey-listed first, then standing strike count descending,
// default) or sort=peer_id; paginated via limit/offset.
func (s *Server) ListPeersScoring(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.ListPeersScoring")
	defer span.End()

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
	sourceFilter := r.URL.Query().Get("source")
	if sourceFilter != "" && !slices.Contains(peerscoring.BadResponseSourceNames(), sourceFilter) {
		httputil.HandleError(w, fmt.Sprintf("Invalid source %q, expected one of: %s", sourceFilter, strings.Join(peerscoring.BadResponseSourceNames(), ", ")), http.StatusBadRequest)
		return
	}
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "bad_responses"
	}
	if sortBy != "bad_responses" && sortBy != "peer_id" {
		httputil.HandleError(w, fmt.Sprintf("Invalid sort %q, expected bad_responses or peer_id", sortBy), http.StatusBadRequest)
		return
	}
	limit, offset, err := parsePagination(r)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	entries := make([]*peerscoring.PeerScoringDebug, 0)
	for _, d := range s.buildAllPeersDebug(topicScores) {
		if greyListed != nil && d.GreyListed != *greyListed {
			continue
		}
		if agentFilter != "" && !strings.Contains(strings.ToLower(d.Agent), strings.ToLower(agentFilter)) {
			continue
		}
		if sourceFilter != "" && !hasStrikeFromSource(d, sourceFilter) {
			continue
		}
		entries = append(entries, d)
	}
	slices.SortFunc(entries, func(a, b *peerscoring.PeerScoringDebug) int {
		if sortBy == "bad_responses" {
			if a.GreyListed != b.GreyListed {
				if a.GreyListed {
					return -1
				}
				return 1
			}
			if a.BadResponses.StandingCount != b.BadResponses.StandingCount {
				return b.BadResponses.StandingCount - a.BadResponses.StandingCount
			}
		}
		return strings.Compare(a.PeerID, b.PeerID)
	})

	total := len(entries)
	page := paginate(entries, offset, limit)
	httputil.WriteJson(w, &peerscoring.PeersScoringDebugResponse{
		Data: page,
		Meta: &peerscoring.ListMeta{Total: total, Offset: offset, Limit: limit, Returned: len(page)},
	})
}

// ListScoringAgents returns the scoring picture aggregated per agent: peer counts, grey-list
// counts, strikes by source, and rejection counts. Sorted by peer count descending;
// paginated via limit/offset.
func (s *Server) ListScoringAgents(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.ListScoringAgents")
	defer span.End()

	limit, offset, err := parsePagination(r)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	groups := make(map[string]*peerscoring.AgentScoringDebug)
	for _, d := range s.buildAllPeersDebug(false) {
		agent := d.Agent
		if agent == "" {
			agent = agentUnknown
		}
		g, ok := groups[agent]
		if !ok {
			g = &peerscoring.AgentScoringDebug{Agent: agent}
			groups[agent] = g
		}
		g.PeerCount++
		if d.GreyListed {
			g.GreyListedPeerCount++
		}
		for _, strike := range d.BadResponses.History {
			if g.BadResponsesBySource == nil {
				g.BadResponsesBySource = make(map[string]int)
			}
			g.BadResponsesBySource[strike.Source]++
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

	total := len(entries)
	page := paginate(entries, offset, limit)
	httputil.WriteJson(w, &peerscoring.ScoringAgentsResponse{
		Data: page,
		Meta: &peerscoring.ListMeta{Total: total, Offset: offset, Limit: limit, Returned: len(page)},
	})
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
// all), peer_id=<peer id>, since=<RFC3339 time>. Paginated via limit/offset.
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
	limit, offset, err := parsePagination(r)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
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

	total := len(entries)
	page := paginate(entries, offset, limit)
	data := make([]*peerscoring.PeerGossipRejectionDebug, 0, len(page))
	for _, rj := range page {
		data = append(data, &peerscoring.PeerGossipRejectionDebug{
			PeerID:    rj.PeerID.String(),
			Topic:     rj.Topic,
			Agent:     rj.Agent,
			Reason:    rj.Reason,
			Timestamp: rj.At.UTC().Format(time.RFC3339Nano),
		})
	}
	httputil.WriteJson(w, &peerscoring.GossipRejectionsResponse{
		Data: data,
		Meta: &peerscoring.ListMeta{Total: total, Offset: offset, Limit: limit, Returned: len(data)},
	})
}

// GetGossipRejectionsSummary returns retained gossip rejections counted per group_by=topic
// (default) | agent | reason | peer, largest group first. Paginated via limit/offset.
func (s *Server) GetGossipRejectionsSummary(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.GetGossipRejectionsSummary")
	defer span.End()

	groupBy := r.URL.Query().Get("group_by")
	if groupBy == "" {
		groupBy = "topic"
	}
	if groupBy != "topic" && groupBy != "agent" && groupBy != "reason" && groupBy != "peer" {
		httputil.HandleError(w, fmt.Sprintf("Invalid group_by %q, expected topic, agent, reason or peer", groupBy), http.StatusBadRequest)
		return
	}
	limit, offset, err := parsePagination(r)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	counts := make(map[string]int)
	totalRejections := 0
	for _, rj := range s.GossipRejectionsFetcher.GossipRejections().FlatRejections() {
		totalRejections++
		var key string
		switch groupBy {
		case "topic":
			key = rj.Topic
		case "agent":
			key = rj.Agent
			if key == "" {
				key = agentUnknown
			}
		case "reason":
			key = rj.Reason
		case "peer":
			key = rj.PeerID.String()
		}
		counts[key]++
	}
	entries := make([]*peerscoring.RejectionGroupDebug, 0, len(counts))
	for value, count := range counts {
		entries = append(entries, &peerscoring.RejectionGroupDebug{Value: value, Count: count})
	}
	slices.SortFunc(entries, func(a, b *peerscoring.RejectionGroupDebug) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Value, b.Value)
	})

	total := len(entries)
	page := paginate(entries, offset, limit)
	httputil.WriteJson(w, &peerscoring.GossipRejectionsSummaryResponse{
		Data: page,
		Meta: &peerscoring.GossipRejectionsSummaryMeta{
			ListMeta:        peerscoring.ListMeta{Total: total, Offset: offset, Limit: limit, Returned: len(page)},
			GroupBy:         groupBy,
			TotalRejections: totalRejections,
		},
	})
}

// buildAllPeersDebug assembles the debug model for every peer with scoring or rejection
// state plus every connected peer.
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
	for _, pid := range s.PeersFetcher.Peers().Connected() {
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
	for _, strike := range d.BadResponses.History {
		if strike.Source == source {
			return true
		}
	}
	return false
}

// parsePagination reads limit (default 50, max 500) and offset (default 0).
func parsePagination(r *http.Request) (limit, offset int, err error) {
	limit = defaultPageLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 1 {
			return 0, 0, fmt.Errorf("invalid limit %q, expected a positive integer", v)
		}
		if limit > maxPageLimit {
			limit = maxPageLimit
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 {
			return 0, 0, fmt.Errorf("invalid offset %q, expected a non-negative integer", v)
		}
	}
	return limit, offset, nil
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

// paginate returns the [offset, offset+limit) window of entries.
func paginate[T any](entries []T, offset, limit int) []T {
	if offset >= len(entries) {
		return []T{}
	}
	end := min(offset+limit, len(entries))
	return entries[offset:end]
}
