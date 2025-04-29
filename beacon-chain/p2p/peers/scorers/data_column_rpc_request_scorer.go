package scorers

import (
	"fmt"
	"math"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/peerdata"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/libp2p/go-libp2p/core/peer"
)

var _ Scorer = (*DataColumnRPCRequestScorer)(nil)

const (
	// DefaultDataColumnRPCRequestDecayInterval defines how often the decaying routine is called.
	DefaultDataColumnRPCRequestDecayInterval = 30 * time.Second
	// DefaultDataColumnRPCRequestDecay defines default number of requests that are to be subtracted
	// from stats on each decay interval.
	DefaultDataColumnRPCRequestDecay = uint64(10)
	// DefaultDataColumnRPCRequestThreshold defines the maximum number of requests a peer can make
	// before being considered bad.
	DefaultDataColumnRPCRequestThreshold = uint64(100)
	// DefaultDataColumnRPCRequestPenaltyFactor defines the penalty factor applied to request count.
	DefaultDataColumnRPCRequestPenaltyFactor = float64(0.02)
	// DefaultDataColumnMaxGossipAgeSlots defines the maximum age in slots for a requested column
	// to trigger downscoring. Defaults to half an epoch on mainnet.
	DefaultDataColumnMaxGossipAgeSlots = uint64(16)
)

// DataColumnRPCRequestScorer represents scoring service for tracking data column RPC requests.
type DataColumnRPCRequestScorer struct {
	config *DataColumnRPCRequestScorerConfig
	store  *peerdata.Store
}

// DataColumnRPCRequestScorerConfig holds configuration parameters for data column RPC request scoring service.
type DataColumnRPCRequestScorerConfig struct {
	// DecayInterval defines how often stats should be decayed.
	DecayInterval time.Duration
	// Decay specifies number of requests subtracted from stats on each decay step.
	Decay uint64
	// Threshold defines maximum number of requests before peer is considered bad.
	Threshold uint64
	// PenaltyFactor defines multiplier applied to request count when calculating score.
	PenaltyFactor float64
	// MaxGossipAgeSlots defines the maximum age in slots for a requested column
	// to trigger downscoring. Columns requested for slots older than
	// current_slot - MaxGossipAgeSlots will not be penalized.
	MaxGossipAgeSlots uint64
}

// newDataColumnRPCRequestScorer creates new scoring service for data column RPC requests.
func newDataColumnRPCRequestScorer(store *peerdata.Store, config *DataColumnRPCRequestScorerConfig) *DataColumnRPCRequestScorer {
	// Create a new config with default values
	cfg := &DataColumnRPCRequestScorerConfig{
		DecayInterval:     DefaultDataColumnRPCRequestDecayInterval,
		Decay:             DefaultDataColumnRPCRequestDecay,
		Threshold:         DefaultDataColumnRPCRequestThreshold,
		PenaltyFactor:     DefaultDataColumnRPCRequestPenaltyFactor,
		MaxGossipAgeSlots: DefaultDataColumnMaxGossipAgeSlots,
	}

	// Override with provided config values if present
	if config != nil {
		if config.DecayInterval != 0 {
			cfg.DecayInterval = config.DecayInterval
		}
		if config.Decay != 0 {
			cfg.Decay = config.Decay
		}
		if config.Threshold != 0 {
			cfg.Threshold = config.Threshold
		}
		if config.PenaltyFactor != 0 {
			cfg.PenaltyFactor = config.PenaltyFactor
		}
		if config.MaxGossipAgeSlots != 0 {
			cfg.MaxGossipAgeSlots = config.MaxGossipAgeSlots
		}
	}

	return &DataColumnRPCRequestScorer{
		config: cfg,
		store:  store,
	}
}

// Score returns calculated peer score.
func (s *DataColumnRPCRequestScorer) Score(pid peer.ID) float64 {
	s.store.RLock()
	defer s.store.RUnlock()
	return s.scoreNoLock(pid)
}

// scoreNoLock is a lock-free version of Score.
func (s *DataColumnRPCRequestScorer) scoreNoLock(pid peer.ID) float64 {
	if peerData, ok := s.store.PeerData(pid); ok {
		// Apply penalty based on request count
		score := -1 * float64(peerData.DataColumnRequestCount) * s.config.PenaltyFactor
		return math.Round(score*ScoreRoundingFactor) / ScoreRoundingFactor
	}
	return 0
}

// IsBadPeer implements Scorer interface.
func (s *DataColumnRPCRequestScorer) IsBadPeer(pid peer.ID) error {
	s.store.RLock()
	defer s.store.RUnlock()
	return s.isBadPeerNoLock(pid)
}

// isBadPeerNoLock is a lock-free version of IsBadPeer.
func (s *DataColumnRPCRequestScorer) isBadPeerNoLock(pid peer.ID) error {
	if peerData, ok := s.store.PeerData(pid); ok && peerData.DataColumnRequestCount >= s.config.Threshold {
		return fmt.Errorf("bad peer %s: request count %d exceeds threshold %d", pid, peerData.DataColumnRequestCount, s.config.Threshold)
	}
	return nil
}

// BadPeers returns the peers that are considered bad by the scorer.
func (s *DataColumnRPCRequestScorer) BadPeers() []peer.ID {
	s.store.RLock()
	defer s.store.RUnlock()

	badPeers := make([]peer.ID, 0)
	for pid := range s.store.Peers() {
		if s.isBadPeerNoLock(pid) != nil {
			badPeers = append(badPeers, pid)
		}
	}
	return badPeers
}

// RecordRequest records a potentially downscorable data column RPC request for a peer.
// Downscoring only occurs if the requested columnSlot is recent enough compared to the currentSlot.
func (s *DataColumnRPCRequestScorer) RecordRequest(pid peer.ID, currentSlot, columnSlot primitives.Slot) {
	if pid == "" {
		return
	}
	// Check if the column is recent enough to warrant downscoring.
	// A peer shouldn't be penalized for requesting old columns it might have missed.
	if uint64(columnSlot)+s.config.MaxGossipAgeSlots < uint64(currentSlot) {
		return
	}

	s.store.Lock()
	defer s.store.Unlock()

	peerData := s.store.PeerDataGetOrCreate(pid)
	peerData.DataColumnRequestCount++ // Increment count for each recent column requested
	peerData.DataColumnRPCLastRequestTime = time.Now()
}

// Decay implements periodic decay of request counts.
func (s *DataColumnRPCRequestScorer) Decay() {
	s.store.Lock()
	defer s.store.Unlock()

	for _, peerData := range s.store.Peers() {
		if peerData.DataColumnRequestCount > s.config.Decay {
			peerData.DataColumnRequestCount -= s.config.Decay
		} else {
			peerData.DataColumnRequestCount = 0
		}
	}
}

// Params exposes scorer's parameters.
func (s *DataColumnRPCRequestScorer) Params() *DataColumnRPCRequestScorerConfig {
	return s.config
}
