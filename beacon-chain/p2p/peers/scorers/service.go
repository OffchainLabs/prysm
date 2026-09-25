package scorers

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers/peerdata"
	"github.com/libp2p/go-libp2p/core/peer"
)

// ScoreRoundingFactor defines how many digits to keep in decimal part.
// This parameter is used in math.Round(score*ScoreRoundingFactor) / ScoreRoundingFactor.
const ScoreRoundingFactor = 10000

// Scorer defines minimum set of methods every peer scorer must expose.
type Scorer interface {
	Score(pid peer.ID) float64
	IsBadPeer(pid peer.ID) error
	BadPeers() []peer.ID
}

// Service manages the block provider scorer used for sync peer selection.
// Grey-listing and reputation live in the p2p/peerscoring package.
type Service struct {
	scorers struct {
		blockProviderScorer *BlockProviderScorer
	}
}

// Config holds configuration parameters for scoring service.
type Config struct {
	BlockProviderScorerConfig *BlockProviderScorerConfig
}

// NewService provides fully initialized peer scoring service.
func NewService(ctx context.Context, store *peerdata.Store, config *Config) *Service {
	if config == nil {
		config = &Config{}
	}
	s := &Service{}

	// Register scorers.
	s.scorers.blockProviderScorer = newBlockProviderScorer(store, config.BlockProviderScorerConfig)

	// Start background tasks.
	go s.loop(ctx)

	return s
}

// BlockProviderScorer exposes block provider scoring service.
func (s *Service) BlockProviderScorer() *BlockProviderScorer {
	return s.scorers.blockProviderScorer
}

// loop handles background tasks.
func (s *Service) loop(ctx context.Context) {
	decayBlockProviderStats := time.NewTicker(s.scorers.blockProviderScorer.Params().DecayInterval)
	defer decayBlockProviderStats.Stop()

	for {
		select {
		case <-decayBlockProviderStats.C:
			// Exit early if context is canceled.
			if ctx.Err() != nil {
				return
			}
			s.scorers.blockProviderScorer.Decay()
		case <-ctx.Done():
			return
		}
	}
}
