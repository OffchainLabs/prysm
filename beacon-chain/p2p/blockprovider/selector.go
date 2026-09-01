// Package blockprovider tracks how many blocks each peer has recently served and turns
// that into a selection signal for initial-sync: productive peers are preferred, while
// stale or unknown peers get a max-score boost so they keep getting a chance to serve.
// This is a work-allocation mechanism, not peer scoring — reputation and grey-listing
// live in the p2p/peerscoring package.
package blockprovider

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	"github.com/libp2p/go-libp2p/core/peer"
)

// ScoreRoundingFactor defines how many digits to keep in decimal part.
// This parameter is used in math.Round(score*ScoreRoundingFactor) / ScoreRoundingFactor.
const ScoreRoundingFactor = 10000

const (
	// DefaultProcessedBatchWeight is a default reward weight of a processed batch of blocks.
	DefaultProcessedBatchWeight = float64(0.1)
	// DefaultProcessedBlocksCap defines default value for processed blocks cap.
	// e.g. 20 * 64 := 20 batches of size 64 (with 0.05 per batch reward, 20 batches result in score of 1.0).
	DefaultProcessedBlocksCap = uint64(10 * 64)
	// DefaultDecayInterval defines how often the decaying routine is called.
	DefaultDecayInterval = 30 * time.Second
	// DefaultDecay defines default blocks that are to be subtracted from stats on each
	// decay interval. Effectively, this param provides minimum expected performance for a peer to remain
	// high scorer.
	DefaultDecay = uint64(1 * 64)
	// DefaultStalePeerRefreshInterval defines default interval at which peers should be given
	// opportunity to provide blocks (their score gets boosted, up until they are selected for
	// fetching).
	DefaultStalePeerRefreshInterval = 5 * time.Minute
)

// peerStats is the per-peer state the selector judges a peer by.
type peerStats struct {
	processedBlocks uint64
	updated         time.Time
}

// Selector tracks per-peer block-serving throughput and ranks peers for block fetching.
type Selector struct {
	config *SelectorConfig
	// maxScore is a cached value for maximum attainable block provider score.
	// It is calculated, on startup, as following: (processedBlocksCap / batchSize) * batchWeight.
	maxScore float64

	lock  sync.RWMutex
	peers map[peer.ID]*peerStats
}

// SelectorConfig holds configuration parameters for the block provider selector.
type SelectorConfig struct {
	// ProcessedBatchWeight defines a reward for a single processed batch of blocks.
	ProcessedBatchWeight float64
	// ProcessedBlocksCap defines the highest number of processed blocks that are counted towards peer's score.
	// Once that cap is attained, peer is considered good to fetch from (and several peers having the
	// same score, are picked at random). To stay at max score, peer must continue to perform, as
	// stats decays quickly.
	ProcessedBlocksCap uint64
	// DecayInterval defines how often stats should be decayed.
	DecayInterval time.Duration
	// Decay specifies number of blocks subtracted from stats on each decay step.
	Decay uint64
	// StalePeerRefreshInterval is an interval at which peers should be given an opportunity
	// to provide blocks (scores are boosted to max up until such peers are selected).
	StalePeerRefreshInterval time.Duration
}

// NewSelector creates a block provider selector and starts its decay loop.
func NewSelector(ctx context.Context, config *SelectorConfig) *Selector {
	if config == nil {
		config = &SelectorConfig{}
	}
	s := &Selector{
		config: config,
		peers:  make(map[peer.ID]*peerStats),
	}
	if s.config.ProcessedBatchWeight == 0.0 {
		s.config.ProcessedBatchWeight = DefaultProcessedBatchWeight
	}
	if s.config.DecayInterval == 0 {
		s.config.DecayInterval = DefaultDecayInterval
	}
	if s.config.ProcessedBlocksCap == 0 {
		s.config.ProcessedBlocksCap = DefaultProcessedBlocksCap
	}
	if s.config.Decay == 0 {
		s.config.Decay = DefaultDecay
	}
	if s.config.StalePeerRefreshInterval == 0 {
		s.config.StalePeerRefreshInterval = DefaultStalePeerRefreshInterval
	}
	batchSize := uint64(flags.Get().BlockBatchLimit)
	s.maxScore = 1.0
	if batchSize > 0 {
		totalBatches := float64(s.config.ProcessedBlocksCap / batchSize)
		s.maxScore = totalBatches * s.config.ProcessedBatchWeight
		s.maxScore = math.Round(s.maxScore*ScoreRoundingFactor) / ScoreRoundingFactor
	}
	go s.decayLoop(ctx)
	return s
}

// decayLoop periodically decays block provider stats until ctx is canceled.
func (s *Selector) decayLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.DecayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Exit early if context is canceled.
			if ctx.Err() != nil {
				return
			}
			s.Decay()
		case <-ctx.Done():
			return
		}
	}
}

// Score calculates and returns block provider score.
func (s *Selector) Score(pid peer.ID) float64 {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.score(pid)
}

// score is the lock-free version of Score; callers must hold s.lock.
func (s *Selector) score(pid peer.ID) float64 {
	score := float64(0)
	stats, ok := s.peers[pid]
	// Boost score of new peers or peers that haven't been accessed for too long.
	if !ok || time.Since(stats.updated) >= s.config.StalePeerRefreshInterval {
		return s.maxScore
	}
	batchSize := uint64(flags.Get().BlockBatchLimit)
	if batchSize > 0 {
		processedBatches := float64(stats.processedBlocks / batchSize)
		score += processedBatches * s.config.ProcessedBatchWeight
	}
	return math.Round(score*ScoreRoundingFactor) / ScoreRoundingFactor
}

// Params exposes the selector's parameters.
func (s *Selector) Params() *SelectorConfig {
	return s.config
}

// IncrementProcessedBlocks increments the number of blocks that have been successfully processed.
func (s *Selector) IncrementProcessedBlocks(pid peer.ID, cnt uint64) {
	if pid == "" {
		return
	}
	s.lock.Lock()
	defer s.lock.Unlock()

	stats := s.getOrCreate(pid)
	stats.updated = time.Now()
	if cnt == 0 {
		return
	}
	if stats.processedBlocks+cnt > s.config.ProcessedBlocksCap {
		cnt = s.config.ProcessedBlocksCap - stats.processedBlocks
	}
	stats.processedBlocks += cnt
}

// Touch updates last access time for a given peer. This allows to detect peers that are
// stale and boost their scores to increase chances in block fetching participation.
func (s *Selector) Touch(pid peer.ID, t ...time.Time) {
	s.lock.Lock()
	defer s.lock.Unlock()

	stats := s.getOrCreate(pid)
	if len(t) == 1 {
		stats.updated = t[0]
	} else {
		stats.updated = time.Now()
	}
}

// ProcessedBlocks returns number of peer returned blocks that are successfully processed.
func (s *Selector) ProcessedBlocks(pid peer.ID) uint64 {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if stats, ok := s.peers[pid]; ok {
		return stats.processedBlocks
	}
	return 0
}

// Decay updates block provider counters by decaying them.
// This urges peers to keep up the performance to continue getting a high score (and allows
// new peers to contest previously high scoring ones). Fully decayed peers whose stats have
// gone stale are dropped: they already score identically to unknown peers (max-score boost),
// so removing their entries only reclaims memory.
func (s *Selector) Decay() {
	s.lock.Lock()
	defer s.lock.Unlock()

	for pid, stats := range s.peers {
		if stats.processedBlocks > s.config.Decay {
			stats.processedBlocks -= s.config.Decay
		} else {
			stats.processedBlocks = 0
		}
		if stats.processedBlocks == 0 && time.Since(stats.updated) >= s.config.StalePeerRefreshInterval {
			delete(s.peers, pid)
		}
	}
}

// TrackedPeerCount returns how many peers the selector currently holds stats for.
func (s *Selector) TrackedPeerCount() int {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return len(s.peers)
}

// RemovePeers drops all block provider stats for the given peers.
func (s *Selector) RemovePeers(pids []peer.ID) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for _, pid := range pids {
		delete(s.peers, pid)
	}
}

// WeightSorted returns a list of block providers weight sorted by score, where items are selected
// probabilistically with more "heavy" items having a higher chance of being picked.
func (s *Selector) WeightSorted(
	r *rand.Rand, pids []peer.ID, scoreFn func(pid peer.ID, score float64) float64,
) []peer.ID {
	if len(pids) == 0 {
		return pids
	}
	s.lock.RLock()
	defer s.lock.RUnlock()

	// See http://eli.thegreenplace.net/2010/01/22/weighted-random-generation-in-python/ for details.
	nextPID := func(weights map[peer.ID]float64) peer.ID {
		totalWeight := 0
		for _, w := range weights {
			// Factor by 100, to allow weights in (0; 1) range.
			totalWeight += int(w * 100)
		}
		if totalWeight <= 0 {
			return ""
		}
		rnd := r.Intn(totalWeight)
		for pid, w := range weights {
			rnd -= int(w * 100)
			if rnd < 0 {
				return pid
			}
		}
		return ""
	}

	scores, _ := s.mapScoresAndPeers(pids, scoreFn)
	peers := make([]peer.ID, 0)
	for range pids {
		if pid := nextPID(scores); pid != "" {
			peers = append(peers, pid)
			delete(scores, pid)
		}
	}
	// Left over peers (like peers having zero weight), are added at the end of the list.
	for pid := range scores {
		peers = append(peers, pid)
	}

	return peers
}

// Sorted returns a list of block providers sorted by score in descending order.
// When custom scorer function is provided, items are returned in order provided by it.
func (s *Selector) Sorted(
	pids []peer.ID, scoreFn func(pid peer.ID, score float64) float64,
) []peer.ID {
	if len(pids) == 0 {
		return pids
	}
	s.lock.RLock()
	defer s.lock.RUnlock()

	scores, peers := s.mapScoresAndPeers(pids, scoreFn)
	sort.Slice(peers, func(i, j int) bool {
		return scores[peers[i]] > scores[peers[j]]
	})
	return peers
}

// mapScoresAndPeers is a utility function to map peers and their respective scores (using custom
// scoring function if necessary); callers must hold s.lock.
func (s *Selector) mapScoresAndPeers(
	pids []peer.ID, scoreFn func(pid peer.ID, score float64) float64,
) (map[peer.ID]float64, []peer.ID) {
	scores := make(map[peer.ID]float64, len(pids))
	peers := make([]peer.ID, len(pids))
	for i, pid := range pids {
		if scoreFn != nil {
			scores[pid] = scoreFn(pid, s.score(pid))
		} else {
			scores[pid] = s.score(pid)
		}
		peers[i] = pid
	}
	return scores, peers
}

// FormatScorePretty returns full scoring information in a human-readable format.
func (s *Selector) FormatScorePretty(pid peer.ID) string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	score := s.score(pid)
	processedBlocks := uint64(0)
	if stats, ok := s.peers[pid]; ok {
		processedBlocks = stats.processedBlocks
	}
	return fmt.Sprintf("[%0.1f%%, raw: %0.2f,  blocks: %d/%d]",
		(score/s.MaxScore())*100, score, processedBlocks, s.config.ProcessedBlocksCap)
}

// MaxScore exposes maximum score attainable by peers.
func (s *Selector) MaxScore() float64 {
	return s.maxScore
}

// getOrCreate returns the peer's stats, creating them if absent; callers must hold s.lock.
func (s *Selector) getOrCreate(pid peer.ID) *peerStats {
	stats, ok := s.peers[pid]
	if !ok {
		stats = &peerStats{}
		s.peers[pid] = stats
	}
	return stats
}
