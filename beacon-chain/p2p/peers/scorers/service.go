package scorers

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers/peerdata"
)

// Service holds the block provider scorer used for sync peer selection.
// Peer scoring and grey-listing live in the p2p/peerscoring package.
type Service struct {
	store               *peerdata.Store
	blockProviderScorer *BlockProviderScorer
}

// NewService provides a fully initialized block provider scoring service.
func NewService(ctx context.Context, store *peerdata.Store, config *BlockProviderScorerConfig) *Service {
	s := &Service{
		store:               store,
		blockProviderScorer: newBlockProviderScorer(store, config),
	}

	// Start background tasks.
	go s.loop(ctx)

	return s
}

// BlockProviderScorer exposes block provider scoring service.
func (s *Service) BlockProviderScorer() *BlockProviderScorer {
	return s.blockProviderScorer
}

// loop handles background tasks.
func (s *Service) loop(ctx context.Context) {
	decayBlockProviderStats := time.NewTicker(s.blockProviderScorer.Params().DecayInterval)
	defer decayBlockProviderStats.Stop()

	for {
		select {
		case <-decayBlockProviderStats.C:
			// Exit early if context is canceled.
			if ctx.Err() != nil {
				return
			}
			s.blockProviderScorer.Decay()
		case <-ctx.Done():
			return
		}
	}
}
