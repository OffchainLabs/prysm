package scorers_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/scorers"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestScorers_DataColumnRPCRequest_Score(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tests := []struct {
		name   string
		update func(scorer *scorers.DataColumnRPCRequestScorer)
		check  func(scorer *scorers.DataColumnRPCRequestScorer)
	}{
		{
			name: "nonexistent peer",
			update: func(*scorers.DataColumnRPCRequestScorer) {
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				assert.Equal(t, 0.0, scorer.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "peer with no requests",
			update: func(scorer *scorers.DataColumnRPCRequestScorer) {
				scorer.RecordRequest("peer1", 0)
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				assert.Equal(t, 0.0, scorer.Score("peer1"), "Unexpected score")
				assert.NoError(t, scorer.IsBadPeer("peer1"), "Unexpected bad peer status")
			},
		},
		{
			name: "peer with requests below threshold",
			update: func(scorer *scorers.DataColumnRPCRequestScorer) {
				scorer.RecordRequest("peer1", 10)
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Expected score: -10 * DefaultDataColumnRPCRequestPenaltyFactor
				expectedScore := -10.0 * scorers.DefaultDataColumnRPCRequestPenaltyFactor
				assert.Equal(t, expectedScore, scorer.Score("peer1"), "Unexpected score")
				assert.NoError(t, scorer.IsBadPeer("peer1"), "Unexpected bad peer status")
			},
		},
		{
			name: "peer at threshold",
			update: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Set requests to exactly the threshold
				for i := 0; i < int(scorers.DefaultDataColumnRPCRequestThreshold/10); i++ {
					scorer.RecordRequest("peer1", 10)
				}
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Expected score: -threshold * DefaultDataColumnRPCRequestPenaltyFactor
				expectedScore := -float64(scorers.DefaultDataColumnRPCRequestThreshold) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
				assert.Equal(t, expectedScore, scorer.Score("peer1"), "Unexpected score")
				assert.NotNil(t, scorer.IsBadPeer("peer1"), "Expected peer to be marked as bad")
			},
		},
		{
			name: "peer above threshold",
			update: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Set requests above the threshold
				for i := 0; i < int(scorers.DefaultDataColumnRPCRequestThreshold/10)+1; i++ {
					scorer.RecordRequest("peer1", 10)
				}
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Expected score: -(threshold+10) * DefaultDataColumnRPCRequestPenaltyFactor
				expectedScore := -float64(scorers.DefaultDataColumnRPCRequestThreshold+10) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
				assert.Equal(t, expectedScore, scorer.Score("peer1"), "Unexpected score")
				assert.NotNil(t, scorer.IsBadPeer("peer1"), "Expected peer to be marked as bad")
			},
		},
		{
			name: "peer with decay",
			update: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Set initial requests
				scorer.RecordRequest("peer1", 50)
				// Trigger decay
				scorer.Decay()
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// After decay, count should be (50 - DefaultDataColumnRPCRequestDecay)
				expectedCount := uint64(50 - scorers.DefaultDataColumnRPCRequestDecay)
				expectedScore := -float64(expectedCount) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
				assert.Equal(t, expectedScore, scorer.Score("peer1"), "Unexpected score after decay")
				assert.NoError(t, scorer.IsBadPeer("peer1"), "Unexpected bad peer status after decay")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
				ScorerParams: &scorers.Config{},
			})
			scorer := peerStatuses.Scorers().DataColumnRPCRequestScorer()
			if tt.update != nil {
				tt.update(scorer)
			}
			tt.check(scorer)
		})
	}
}

func TestScorers_DataColumnRPCRequest_BadPeers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
		ScorerParams: &scorers.Config{},
	})
	scorer := peerStatuses.Scorers().DataColumnRPCRequestScorer()

	// Add three peers with different request counts
	pid1 := peer.ID("peer1")
	pid2 := peer.ID("peer2")
	pid3 := peer.ID("peer3")

	// Peer1: Below threshold
	scorer.RecordRequest(pid1, 10)
	// Peer2: At threshold
	for i := 0; i < int(scorers.DefaultDataColumnRPCRequestThreshold/10); i++ {
		scorer.RecordRequest(pid2, 10)
	}
	// Peer3: Above threshold
	for i := 0; i < int(scorers.DefaultDataColumnRPCRequestThreshold/10)+1; i++ {
		scorer.RecordRequest(pid3, 10)
	}

	// Check bad peers list
	badPeers := scorer.BadPeers()
	assert.Equal(t, 2, len(badPeers), "Unexpected number of bad peers")

	// Verify specific peers
	assert.NoError(t, scorer.IsBadPeer(pid1), "Peer1 should not be bad")
	assert.NotNil(t, scorer.IsBadPeer(pid2), "Peer2 should be bad")
	assert.NotNil(t, scorer.IsBadPeer(pid3), "Peer3 should be bad")
}

func TestScorers_DataColumnRPCRequest_Params(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test with default config (nil config)
	t.Run("default config", func(t *testing.T) {
		peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
			ScorerParams: &scorers.Config{},
		})
		scorer := peerStatuses.Scorers().DataColumnRPCRequestScorer()
		params := scorer.Params()

		assert.Equal(t, scorers.DefaultDataColumnRPCRequestDecayInterval, params.DecayInterval, "Wrong default decay interval")
		assert.Equal(t, scorers.DefaultDataColumnRPCRequestDecay, params.Decay, "Wrong default decay value")
		assert.Equal(t, scorers.DefaultDataColumnRPCRequestThreshold, params.Threshold, "Wrong default threshold")
		assert.Equal(t, scorers.DefaultDataColumnRPCRequestPenaltyFactor, params.PenaltyFactor, "Wrong default penalty factor")
	})

	// Test with custom config
	t.Run("custom config", func(t *testing.T) {
		customConfig := &scorers.DataColumnRPCRequestScorerConfig{
			DecayInterval: time.Minute,
			Decay:         20,
			Threshold:     200,
			PenaltyFactor: 0.05,
		}
		peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
			ScorerParams: &scorers.Config{
				DataColumnRPCRequestScorerConfig: customConfig,
			},
		})
		scorer := peerStatuses.Scorers().DataColumnRPCRequestScorer()
		params := scorer.Params()

		assert.Equal(t, customConfig.DecayInterval, params.DecayInterval, "Wrong custom decay interval")
		assert.Equal(t, customConfig.Decay, params.Decay, "Wrong custom decay value")
		assert.Equal(t, customConfig.Threshold, params.Threshold, "Wrong custom threshold")
		assert.Equal(t, customConfig.PenaltyFactor, params.PenaltyFactor, "Wrong custom penalty factor")

		// Verify the config affects scoring
		scorer.RecordRequest("peer1", 150)
		expectedScore := -150.0 * customConfig.PenaltyFactor
		assert.Equal(t, expectedScore, scorer.Score("peer1"), "Wrong score with custom penalty factor")
		assert.NoError(t, scorer.IsBadPeer("peer1"), "Peer should not be bad yet")

		// Push peer over custom threshold
		scorer.RecordRequest("peer1", 51)
		assert.NotNil(t, scorer.IsBadPeer("peer1"), "Peer should be bad after exceeding custom threshold")
	})

	// Test partial config (some values specified, others default)
	t.Run("partial config", func(t *testing.T) {
		partialConfig := &scorers.DataColumnRPCRequestScorerConfig{
			DecayInterval: time.Minute,
			Threshold:     200,
		}
		peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
			ScorerParams: &scorers.Config{
				DataColumnRPCRequestScorerConfig: partialConfig,
			},
		})
		scorer := peerStatuses.Scorers().DataColumnRPCRequestScorer()
		params := scorer.Params()

		assert.Equal(t, partialConfig.DecayInterval, params.DecayInterval, "Wrong decay interval")
		assert.Equal(t, partialConfig.Threshold, params.Threshold, "Wrong threshold")
		// Unspecified values should use defaults
		assert.Equal(t, scorers.DefaultDataColumnRPCRequestDecay, params.Decay, "Should use default decay")
		assert.Equal(t, scorers.DefaultDataColumnRPCRequestPenaltyFactor, params.PenaltyFactor, "Should use default penalty factor")
	})

	// Test config immutability
	t.Run("config immutability", func(t *testing.T) {
		customConfig := &scorers.DataColumnRPCRequestScorerConfig{
			DecayInterval: time.Minute,
			Decay:         20,
			Threshold:     200,
			PenaltyFactor: 0.05,
		}
		peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
			ScorerParams: &scorers.Config{
				DataColumnRPCRequestScorerConfig: customConfig,
			},
		})
		scorer := peerStatuses.Scorers().DataColumnRPCRequestScorer()

		// Modify original config
		customConfig.DecayInterval = time.Hour
		customConfig.Decay = 50
		customConfig.Threshold = 500
		customConfig.PenaltyFactor = 0.1

		// Verify scorer's config is unchanged
		params := scorer.Params()
		assert.Equal(t, time.Minute, params.DecayInterval, "Config should be immutable")
		assert.Equal(t, uint64(20), params.Decay, "Config should be immutable")
		assert.Equal(t, uint64(200), params.Threshold, "Config should be immutable")
		assert.Equal(t, 0.05, params.PenaltyFactor, "Config should be immutable")
	})
}

func TestScorers_DataColumnRPCRequest_Count(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
		ScorerParams: &scorers.Config{},
	})
	scorer := peerStatuses.Scorers().DataColumnRPCRequestScorer()

	// Test unknown peer
	t.Run("unknown peer", func(t *testing.T) {
		pid := peer.ID("peer1")
		// Verify peer is unknown initially
		_, err := peerStatuses.ConnectionState(pid)
		assert.ErrorContains(t, "peer unknown", err, "Peer should not exist")

		// Record request for unknown peer
		scorer.RecordRequest(pid, 5)

		// Verify peer data after request
		state, err := peerStatuses.ConnectionState(pid)
		require.NoError(t, err, "Peer should exist")
		assert.Equal(t, peers.Disconnected, state, "Wrong connection state")
		assert.Equal(t, float64(-0.1), scorer.Score(pid), "Wrong score for request count of 5")
	})

	// Test multiple requests accumulation
	t.Run("request accumulation", func(t *testing.T) {
		pid := peer.ID("peer2")

		// Record series of requests
		scorer.RecordRequest(pid, 10)
		assert.Equal(t, float64(-0.2), scorer.Score(pid), "Wrong score after first request")

		scorer.RecordRequest(pid, 15)
		assert.Equal(t, float64(-0.5), scorer.Score(pid), "Wrong score after second request")

		scorer.RecordRequest(pid, 20)
		assert.Equal(t, float64(-0.9), scorer.Score(pid), "Wrong score after third request")
	})

	// Test invalid requests
	t.Run("invalid requests", func(t *testing.T) {
		pid := peer.ID("peer3")

		// Record initial valid request
		scorer.RecordRequest(pid, 10)
		initialScore := scorer.Score(pid)

		// Try invalid requests
		scorer.RecordRequest(pid, 0)  // Zero columns
		scorer.RecordRequest(pid, -1) // Negative columns
		scorer.RecordRequest("", 5)   // Empty peer ID

		// Verify score unchanged
		assert.Equal(t, initialScore, scorer.Score(pid), "Score should not change for invalid requests")
	})

	// Test request timing
	t.Run("request timing", func(t *testing.T) {
		pid := peer.ID("peer4")

		// Record first request
		scorer.RecordRequest(pid, 5)
		time.Sleep(time.Millisecond)
		firstScore := scorer.Score(pid)

		// Record second request
		scorer.RecordRequest(pid, 5)
		secondScore := scorer.Score(pid)

		// Verify scores reflect accumulation
		assert.Equal(t, 2*firstScore, secondScore, "Second score should be double the first")
	})

	// Test concurrent requests
	t.Run("concurrent requests", func(t *testing.T) {
		pid := peer.ID("peer5")
		const numRequests = 100
		const columnsPerRequest = 5

		// Launch multiple goroutines to record requests concurrently
		var wg sync.WaitGroup
		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				scorer.RecordRequest(pid, columnsPerRequest)
			}()
		}
		wg.Wait()

		// Verify final score
		expectedScore := -float64(numRequests*columnsPerRequest) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assert.Equal(t, expectedScore, scorer.Score(pid), "Wrong score after concurrent requests")
	})

	// Test ByRange requests with count multiplier
	t.Run("byrange requests", func(t *testing.T) {
		pid := peer.ID("peer6")

		// Record a ByRange request with count=3 and 2 columns
		scorer.RecordRequest(pid, 6) // 3 * 2 columns
		expectedScore := -float64(6) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assert.Equal(t, expectedScore, scorer.Score(pid), "Wrong score for ByRange request")

		// Record another ByRange request with count=2 and 3 columns
		scorer.RecordRequest(pid, 6) // 2 * 3 columns
		expectedScore = -float64(12) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assert.Equal(t, expectedScore, scorer.Score(pid), "Wrong score after multiple ByRange requests")
	})
}
