package scorers_test

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers/scorers"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestScorers_Service_Init(t *testing.T) {
	ctx := t.Context()

	batchSize := uint64(flags.Get().BlockBatchLimit)

	t.Run("default config", func(t *testing.T) {
		peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
			PeerLimit:    30,
			ScorerParams: &scorers.Config{},
		})

		t.Run("block providers scorer", func(t *testing.T) {
			params := peerStatuses.Scorers().BlockProviderScorer().Params()
			assert.Equal(t, scorers.DefaultBlockProviderProcessedBatchWeight, params.ProcessedBatchWeight)
			assert.Equal(t, scorers.DefaultBlockProviderProcessedBlocksCap, params.ProcessedBlocksCap)
			assert.Equal(t, scorers.DefaultBlockProviderDecayInterval, params.DecayInterval)
			assert.Equal(t, scorers.DefaultBlockProviderDecay, params.Decay)
			assert.Equal(t, scorers.DefaultBlockProviderStalePeerRefreshInterval, params.StalePeerRefreshInterval)
		})
	})

	t.Run("nil config", func(t *testing.T) {
		peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
			PeerLimit: 30,
		})
		params := peerStatuses.Scorers().BlockProviderScorer().Params()
		assert.Equal(t, scorers.DefaultBlockProviderProcessedBatchWeight, params.ProcessedBatchWeight)
		assert.Equal(t, scorers.DefaultBlockProviderDecayInterval, params.DecayInterval)
	})

	t.Run("explicit config", func(t *testing.T) {
		peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
			PeerLimit: 30,
			ScorerParams: &scorers.Config{
				BlockProviderScorerConfig: &scorers.BlockProviderScorerConfig{
					ProcessedBatchWeight:     0.2,
					ProcessedBlocksCap:       batchSize * 5,
					DecayInterval:            1 * time.Minute,
					Decay:                    16,
					StalePeerRefreshInterval: 5 * time.Hour,
				},
			},
		})

		t.Run("block provider scorer", func(t *testing.T) {
			params := peerStatuses.Scorers().BlockProviderScorer().Params()
			assert.Equal(t, 0.2, params.ProcessedBatchWeight)
			assert.Equal(t, batchSize*5, params.ProcessedBlocksCap)
			assert.Equal(t, 1*time.Minute, params.DecayInterval)
			assert.Equal(t, uint64(16), params.Decay)
			assert.Equal(t, 5*time.Hour, params.StalePeerRefreshInterval)
			assert.Equal(t, 1.0, peerStatuses.Scorers().BlockProviderScorer().MaxScore())
		})
	})
}

func TestScorers_Service_BlockProvidersScore(t *testing.T) {
	batchSize := uint64(flags.Get().BlockBatchLimit)

	blkProviderScorers := func(s *scorers.Service, pids []peer.ID) map[string]float64 {
		scores := make(map[string]float64, len(pids))
		for _, pid := range pids {
			scores[string(pid)] = s.BlockProviderScorer().Score(pid)
		}
		return scores
	}

	pack := func(s1, s2, s3 float64) map[string]float64 {
		return map[string]float64{
			"peer1": roundScore(s1),
			"peer2": roundScore(s2),
			"peer3": roundScore(s3),
		}
	}

	peerStatuses := peers.NewStatus(t.Context(), &peers.StatusConfig{
		PeerLimit: 30,
		ScorerParams: &scorers.Config{
			BlockProviderScorerConfig: &scorers.BlockProviderScorerConfig{
				Decay: 64,
			},
		},
	})
	s := peerStatuses.Scorers()
	pids := []peer.ID{"peer1", "peer2", "peer3"}
	for _, pid := range pids {
		peerStatuses.Add(nil, pid, nil, network.DirUnknown)
	}

	s1 := s.BlockProviderScorer()
	// Peers start with boosted start score (new peers are boosted by block provider).
	startScore := s1.MaxScore()
	batchWeight := s1.Params().ProcessedBatchWeight

	// Partial batch.
	s1.IncrementProcessedBlocks("peer1", batchSize/4)
	assert.Equal(t, 0.0, s1.Score("peer1"), "Unexpected %q score", "peer1")

	// Single batch.
	s1.IncrementProcessedBlocks("peer1", batchSize)
	assert.DeepEqual(t, pack(batchWeight, startScore, startScore), blkProviderScorers(s, pids), "Unexpected scores")

	// Multiple batches.
	s1.IncrementProcessedBlocks("peer2", batchSize*4)
	assert.DeepEqual(t, pack(batchWeight, batchWeight*4, startScore), blkProviderScorers(s, pids), "Unexpected scores")

	// Partial batch.
	s1.IncrementProcessedBlocks("peer3", batchSize/2)
	assert.DeepEqual(t, pack(batchWeight, batchWeight*4, 0), blkProviderScorers(s, pids), "Unexpected scores")

	// See effect of decaying.
	assert.Equal(t, batchSize+batchSize/4, s1.ProcessedBlocks("peer1"))
	assert.Equal(t, batchSize*4, s1.ProcessedBlocks("peer2"))
	assert.Equal(t, batchSize/2, s1.ProcessedBlocks("peer3"))
	assert.DeepEqual(t, pack(batchWeight, batchWeight*4, 0), blkProviderScorers(s, pids), "Unexpected scores")
	s1.Decay()
	assert.Equal(t, batchSize/4, s1.ProcessedBlocks("peer1"))
	assert.Equal(t, batchSize*3, s1.ProcessedBlocks("peer2"))
	assert.Equal(t, uint64(0), s1.ProcessedBlocks("peer3"))
	assert.DeepEqual(t, pack(0, batchWeight*3, 0), blkProviderScorers(s, pids), "Unexpected scores")
}

func TestScorers_Service_loop(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
		PeerLimit: 30,
		ScorerParams: &scorers.Config{
			BlockProviderScorerConfig: &scorers.BlockProviderScorerConfig{
				DecayInterval: 25 * time.Millisecond,
				Decay:         64,
			},
		},
	})
	s2 := peerStatuses.Scorers().BlockProviderScorer()

	s2.IncrementProcessedBlocks("peer1", 221)
	assert.Equal(t, uint64(221), s2.ProcessedBlocks("peer1"))

	done := make(chan struct{}, 1)
	go func() {
		defer func() {
			done <- struct{}{}
		}()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if s2.ProcessedBlocks("peer1") == 0 {
					return
				}
			case <-ctx.Done():
				t.Error("Timed out")
				return
			}
		}
	}()

	<-done
	assert.Equal(t, uint64(0), s2.ProcessedBlocks("peer1"), "No blocks are expected")
}
