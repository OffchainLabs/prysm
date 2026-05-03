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
	lastScore        float64
	lastTopicInvalid map[string]float64
}

var (
	peerScoreState   = map[peer.ID]*peerScoreSample{}
	peerScoreStateMu sync.Mutex
)

type peerScoreRow struct {
	PeerID                  string  `json:"peer_id"`
	PeerIDShort             string  `json:"peer_id_short"`
	Implementation          string  `json:"implementation"`
	StartScore              float64 `json:"start_score"`
	CurrentScore            float64 `json:"current_score"`
	BehaviourPenalty        float64 `json:"behaviour_penalty"`
	RatePerMin              float64 `json:"rate_per_min"`
	LastDelta               float64 `json:"last_delta"`
	LastDownscoreTopic      string  `json:"last_downscore_topic"`
	LastDownscoreInfo       string  `json:"last_downscore_info"`
	LastDownscoreSecondsAgo int64   `json:"last_downscore_seconds_ago"`
}

type peerScoresResponse struct {
	GeneratedAt int64          `json:"generated_at"`
	Peers       []peerScoreRow `json:"peers"`
}

// ListPeerScores returns gossip score data for every connected peer.
// Tracks per-peer first-seen score and topic-level invalid-message deltas in
// process memory; values reset on node restart.
func (s *Server) ListPeerScores(w http.ResponseWriter, r *http.Request) {
	peers := s.PeersFetcher.Peers()
	connected := peers.Connected()
	scorers := peers.Scorers()
	gossip := scorers.GossipScorer()
	bad := scorers.BadResponsesScorer()

	var pStore peerstore.Peerstore
	if s.PeerManager != nil && s.PeerManager.Host() != nil {
		pStore = s.PeerManager.Host().Peerstore()
	}

	now := time.Now()
	rows := make([]peerScoreRow, 0, len(connected))

	peerScoreStateMu.Lock()
	defer peerScoreStateMu.Unlock()

	live := make(map[peer.ID]struct{}, len(connected))
	for _, pid := range connected {
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
				lastScore:        score,
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
		switch {
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
		}

		var ratePerMin float64
		if state.connectScoreSet {
			minutes := now.Sub(state.connectTime).Minutes()
			if minutes > 0 {
				ratePerMin = (score - state.connectScore) / minutes
			}
		}
		lastDelta := score - state.lastScore
		state.lastScore = score

		var secondsAgo int64 = -1
		if !badTime.IsZero() {
			secondsAgo = int64(now.Sub(badTime).Seconds())
		}
		pidStr := pid.String()
		rows = append(rows, peerScoreRow{
			PeerID:                  pidStr,
			PeerIDShort:             shortPeerID(pidStr),
			Implementation:          agentForPeer(pStore, pid),
			StartScore:              state.connectScore,
			CurrentScore:            score,
			BehaviourPenalty:        behaviour,
			RatePerMin:              ratePerMin,
			LastDelta:               lastDelta,
			LastDownscoreTopic:      shortTopic(lastTopic),
			LastDownscoreInfo:       lastInfo,
			LastDownscoreSecondsAgo: secondsAgo,
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
