package node

import (
	"embed"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
)

//go:embed peer_scores_ui.html
var peerScoresUIFS embed.FS

var knownAgentVersions = []string{
	"erigon/caplin", "grandine", "js-libp2p", "lighthouse", "lodestar",
	"nimbus", "prysm", "teku", "rust-libp2p",
}

type peerScoreSample struct {
	connectScore     float64
	connectTime      time.Time
	connectScoreSet  bool // true once we've seen a non-zero score and locked in connectScore
	history          []scoreAt
	lastTopicInvalid map[string]float64
}

type scoreAt struct {
	score float64
	ts    time.Time
}

// deltaWindow is the rolling window for the "Δ last" column. Wider than the
// poll interval so the column shows a meaningful number of recent activity
// instead of always being 0.
const deltaWindow = 30 * time.Second

var (
	peerScoreState   = map[peer.ID]*peerScoreSample{}
	peerScoreStateMu sync.Mutex
)

type peerScoreRow struct {
	PeerID                  string  `json:"peer_id"`
	PeerIDShort             string  `json:"peer_id_short"`
	Implementation          string  `json:"implementation"`
	ConnectionState         string  `json:"connection_state"`
	StartScore              float64 `json:"start_score"`
	CurrentScore            float64 `json:"current_score"`
	BehaviourPenalty        float64 `json:"behaviour_penalty"`
	RatePerMin              float64 `json:"rate_per_min"`
	LastDelta               float64 `json:"last_delta"`
	LastDownscoreTopic      string  `json:"last_downscore_topic"`
	LastDownscoreInfo       string  `json:"last_downscore_info"`
	LastDownscoreSecondsAgo int64   `json:"last_downscore_seconds_ago"`
	GossipScore             float64 `json:"gossip_score"`
	PeerStatusScore         float64 `json:"peer_status_score"`
	BadResponseScore        float64 `json:"bad_response_score"`
	BadResponses            int     `json:"bad_responses"`
}

// disconnectedRetention controls how long disconnected peers remain visible
// in the peer scores response after disconnect.
const disconnectedRetention = 5 * time.Minute

type peerScoresResponse struct {
	GeneratedAt int64          `json:"generated_at"`
	Peers       []peerScoreRow `json:"peers"`
}

// ListPeerScores returns gossip score data for every connected peer.
// Tracks per-peer first-seen score and topic-level invalid-message deltas in
// process memory; values reset on node restart.
func (s *Server) ListPeerScores(w http.ResponseWriter, r *http.Request) {
	peers := s.PeersFetcher.Peers()
	scorers := peers.Scorers()
	gossip := scorers.GossipScorer()
	bad := scorers.BadResponsesScorer()
	peerStatus := scorers.PeerStatusScorer()

	// Build the working set: connected peers + recently disconnected peers.
	connected := peers.Connected()
	visible := make([]peer.ID, 0, len(connected))
	stateOf := make(map[peer.ID]string, len(connected))
	for _, pid := range connected {
		visible = append(visible, pid)
		stateOf[pid] = "connected"
	}
	cutoff := time.Now().Add(-disconnectedRetention)
	for _, pid := range peers.Disconnected() {
		_, dt := peers.LastDisconnect(pid)
		if dt.IsZero() || dt.Before(cutoff) {
			continue
		}
		visible = append(visible, pid)
		stateOf[pid] = "disconnected"
	}

	var pStore peerstore.Peerstore
	if s.PeerManager != nil && s.PeerManager.Host() != nil {
		pStore = s.PeerManager.Host().Peerstore()
	}

	now := time.Now()
	rows := make([]peerScoreRow, 0, len(connected))

	peerScoreStateMu.Lock()
	defer peerScoreStateMu.Unlock()

	live := make(map[peer.ID]struct{}, len(visible))
	for _, pid := range visible {
		live[pid] = struct{}{}

		// scorers.Score sums every scorer (gossip + bad responses + block provider +
		// peer status). This reflects bad-response increments immediately, even
		// before libp2p's peerInspector has run. GossipData is read separately for
		// the topic-level signals.
		score := scorers.Score(pid)
		_, behaviour, topicScores, _ := gossip.GossipData(pid)

		state, ok := peerScoreState[pid]
		if !ok {
			state = &peerScoreSample{
				lastTopicInvalid: map[string]float64{},
			}
			peerScoreState[pid] = state
		}
		// Lock in the start score the first time we see a non-zero value, so
		// the column doesn't perpetually report 0 just because libp2p hadn't
		// scored the peer when we first observed it.
		if !state.connectScoreSet && score != 0 {
			state.connectScore = score
			state.connectTime = now
			state.connectScoreSet = true
		}
		// Append current sample, drop ones older than the delta window.
		state.history = append(state.history, scoreAt{score: score, ts: now})
		cutoff := now.Add(-deltaWindow - time.Second)
		i := 0
		for i < len(state.history) && state.history[i].ts.Before(cutoff) {
			i++
		}
		state.history = state.history[i:]
		// Δ over the rolling window: current minus the oldest sample still in window.
		var lastDelta float64
		if len(state.history) > 1 {
			lastDelta = score - state.history[0].score
		}

		// Only report actual downscore events. Priority:
		//   1. last bad-response reason (explicit downscorePeer calls)
		//   2. invalid_message_deliveries that grew since last poll
		//   3. any topic with cumulative invalid_message_deliveries > 0
		//   4. behaviour_penalty > 0
		var lastTopic, lastInfo string
		var maxJump, maxInvalid float64
		var jumpTopic, invalidTopic string
		for topic, ts := range topicScores {
			if ts == nil {
				continue
			}
			cur := float64(ts.InvalidMessageDeliveries)
			delta := cur - state.lastTopicInvalid[topic]
			if delta > maxJump {
				maxJump = delta
				jumpTopic = topic
			}
			if cur > maxInvalid {
				maxInvalid = cur
				invalidTopic = topic
			}
			state.lastTopicInvalid[topic] = cur
		}
		badCount, _ := bad.Count(pid)
		badReason, badTime := bad.LastDownscore(pid)
		gossipScoreVal := gossip.Score(pid)
		peerStatusScoreVal := peerStatus.Score(pid)
		badResponseScoreVal := bad.Score(pid)
		discReason, discTime := peers.LastDisconnect(pid)
		connState := stateOf[pid]
		// Disconnect reason takes precedence when the peer is disconnected.
		switch {
		case connState == "disconnected" && discReason != "":
			lastInfo = fmt.Sprintf("disconnected: %s (%s ago)", discReason, shortDuration(now.Sub(discTime)))
		case badReason != "":
			lastInfo = fmt.Sprintf("%s (×%d, %s ago)", badReason, badCount, shortDuration(now.Sub(badTime)))
		case maxJump > 0:
			lastTopic = jumpTopic
			lastInfo = fmt.Sprintf("+%.1f invalid msgs since last poll", maxJump)
		case maxInvalid > 0:
			lastTopic = invalidTopic
			lastInfo = fmt.Sprintf("%.1f cumulative invalid msgs", maxInvalid)
		case behaviour > 0:
			lastInfo = fmt.Sprintf("behaviour_penalty=%.2f", behaviour)
		case score < -0.05 && gossipScoreVal < -0.05:
			lastInfo = fmt.Sprintf("libp2p gossip score=%.2f", gossipScoreVal)
		case score <= 0.001 && peerStatusScoreVal <= 0.001:
			lastInfo = "peer behind our head (peerStatus=0)"
		}

		// rate/min: use the same 30s rolling window as delta so it reflects
		// recent activity rather than long-window average since first observation.
		var ratePerMin float64
		if len(state.history) > 1 {
			windowMinutes := now.Sub(state.history[0].ts).Minutes()
			if windowMinutes > 0 {
				ratePerMin = (score - state.history[0].score) / windowMinutes
			}
		}

		var secondsAgo int64 = -1
		if !badTime.IsZero() {
			secondsAgo = int64(now.Sub(badTime).Seconds())
		}
		pidStr := pid.String()
		rows = append(rows, peerScoreRow{
			PeerID:                  pidStr,
			PeerIDShort:             shortPeerID(pidStr),
			Implementation:          agentForPeer(pStore, pid),
			ConnectionState:         connState,
			StartScore:              state.connectScore,
			CurrentScore:            score,
			BehaviourPenalty:        behaviour,
			RatePerMin:              ratePerMin,
			LastDelta:               lastDelta,
			LastDownscoreTopic:      shortTopic(lastTopic),
			LastDownscoreInfo:       lastInfo,
			LastDownscoreSecondsAgo: secondsAgo,
			GossipScore:             gossipScoreVal,
			PeerStatusScore:         peerStatusScoreVal,
			BadResponseScore:        badResponseScoreVal,
			BadResponses:            badCount,
		})
	}

	for pid := range peerScoreState {
		if _, ok := live[pid]; !ok {
			delete(peerScoreState, pid)
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].CurrentScore < rows[j].CurrentScore })

	httputil.WriteJson(w, &peerScoresResponse{
		GeneratedAt: now.Unix(),
		Peers:       rows,
	})
}

// PeerScoresUI serves a static HTML page that polls ListPeerScores.
func (s *Server) PeerScoresUI(w http.ResponseWriter, r *http.Request) {
	data, err := peerScoresUIFS.ReadFile("peer_scores_ui.html")
	if err != nil {
		httputil.HandleError(w, "Could not load UI: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func agentForPeer(store peerstore.Peerstore, pid peer.ID) string {
	if store == nil {
		return "unknown"
	}
	raw, err := store.Get(pid, "AgentVersion")
	if err != nil {
		return "unknown"
	}
	agent, ok := raw.(string)
	if !ok {
		return "unknown"
	}
	low := strings.ToLower(agent)
	for _, k := range knownAgentVersions {
		if strings.Contains(low, k) {
			return k
		}
	}
	return "unknown"
}

func shortPeerID(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:6] + "…" + s[len(s)-4:]
}

func shortTopic(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" && parts[i] != "ssz_snappy" {
			return parts[i]
		}
	}
	return s
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
