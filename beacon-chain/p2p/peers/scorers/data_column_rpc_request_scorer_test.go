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

// assertFloatEqual is a helper function to compare floating point values with a small epsilon
func assertFloatEqual(t *testing.T, expected, actual float64, msg string) {
	t.Helper()
	epsilon := 1e-10
	diff := expected - actual
	if diff < -epsilon || diff > epsilon {
		t.Errorf("%s, want: %f, got: %f", msg, expected, actual)
	}
}

func TestScorers_DataColumnRPCRequest_Decay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
		ScorerParams: &scorers.Config{},
	})
	scorer := peerStatuses.Scorers().DataColumnRPCRequestScorer()

	// Test basic decay
	t.Run("basic decay", func(t *testing.T) {
		pid := peer.ID("peer1")
		scorer.RecordRequest(pid, 50)

		// Trigger decay
		scorer.Decay()

		// After decay, count should be (50 - DefaultDataColumnRPCRequestDecay)
		expectedCount := uint64(50 - scorers.DefaultDataColumnRPCRequestDecay)
		expectedScore := -float64(expectedCount) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assertFloatEqual(t, expectedScore, scorer.Score(pid), "Wrong score after decay")
	})

	// Test multiple decay cycles
	t.Run("multiple decay cycles", func(t *testing.T) {
		pid := peer.ID("peer2")
		scorer.RecordRequest(pid, 100)

		// Apply decay multiple times
		for i := 0; i < 3; i++ {
			scorer.Decay()
		}

		// After 3 decays, count should be (100 - 3*DefaultDataColumnRPCRequestDecay)
		expectedCount := uint64(100 - 3*scorers.DefaultDataColumnRPCRequestDecay)
		expectedScore := -float64(expectedCount) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assertFloatEqual(t, expectedScore, scorer.Score(pid), "Wrong score after multiple decays")
	})

	// Test decay to zero
	t.Run("decay to zero", func(t *testing.T) {
		pid := peer.ID("peer3")
		// Use a small value that will decay to zero
		scorer.RecordRequest(pid, 10)

		// Single decay should bring count to 0
		scorer.Decay()
		assertFloatEqual(t, float64(0), scorer.Score(pid), "Score should be zero after decay")

		// Additional decay should not make score positive
		scorer.Decay()
		assertFloatEqual(t, float64(0), scorer.Score(pid), "Score should remain zero after additional decay")
	})

	// Test decay with multiple peers
	t.Run("multiple peers decay", func(t *testing.T) {
		pid1 := peer.ID("peer4")
		pid2 := peer.ID("peer5")
		pid3 := peer.ID("peer6")

		// Set different initial counts
		scorer.RecordRequest(pid1, 30)  // Will decay but remain > 0
		scorer.RecordRequest(pid2, 10)  // Will decay to 0
		scorer.RecordRequest(pid3, 100) // Will decay but remain well above 0

		// Record initial scores
		scores := make(map[peer.ID]float64)
		scores[pid1] = scorer.Score(pid1)
		scores[pid2] = scorer.Score(pid2)
		scores[pid3] = scorer.Score(pid3)

		// Apply decay
		scorer.Decay()

		// Verify each peer's decay
		for _, pid := range []peer.ID{pid1, pid2, pid3} {
			initialScore := scores[pid]
			newScore := scorer.Score(pid)

			// New score should be less negative (closer to 0) than initial score
			assert.Equal(t, true, newScore > initialScore || newScore == 0,
				"Score should either increase towards 0 or remain at 0 after decay")
			// Score should never become positive
			assert.Equal(t, true, newScore <= 0,
				"Score should never become positive after decay")
		}

		// Specific checks for each peer
		decayedScore1 := -float64(30-scorers.DefaultDataColumnRPCRequestDecay) *
			scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assertFloatEqual(t, decayedScore1, scorer.Score(pid1), "Wrong score for peer1 after decay")
		assertFloatEqual(t, float64(0), scorer.Score(pid2), "Score for peer2 should be 0 after decay")
		decayedScore3 := -float64(100-scorers.DefaultDataColumnRPCRequestDecay) *
			scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assertFloatEqual(t, decayedScore3, scorer.Score(pid3), "Wrong score for peer3 after decay")
	})

	// Test decay with custom decay value
	t.Run("custom decay value", func(t *testing.T) {
		customConfig := &scorers.DataColumnRPCRequestScorerConfig{
			Decay: 30,
		}
		peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
			ScorerParams: &scorers.Config{
				DataColumnRPCRequestScorerConfig: customConfig,
			},
		})
		customScorer := peerStatuses.Scorers().DataColumnRPCRequestScorer()

		pid := peer.ID("peer7")
		customScorer.RecordRequest(pid, 100)

		// Apply decay with custom value
		customScorer.Decay()

		expectedCount := uint64(100 - customConfig.Decay)
		expectedScore := -float64(expectedCount) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assertFloatEqual(t, expectedScore, customScorer.Score(pid),
			"Wrong score after decay with custom decay value")
	})
}

func TestScorers_DataColumnRPCRequest_BadPeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
		ScorerParams: &scorers.Config{},
	})
	scorer := peerStatuses.Scorers().DataColumnRPCRequestScorer()

	// Test transition from good to bad
	t.Run("good to bad transition", func(t *testing.T) {
		pid := peer.ID("peer1")

		// Start with requests below threshold
		requestsBelow := int(scorers.DefaultDataColumnRPCRequestThreshold - 10)
		scorer.RecordRequest(pid, requestsBelow)
		assert.NoError(t, scorer.IsBadPeer(pid), "Peer should not be bad when below threshold")
		assert.Equal(t, 0, len(scorer.BadPeers()), "Should have no bad peers")

		// Add more requests to exceed threshold
		scorer.RecordRequest(pid, 15)
		assert.NotNil(t, scorer.IsBadPeer(pid), "Peer should be bad after exceeding threshold")
		assert.Equal(t, 1, len(scorer.BadPeers()), "Should have one bad peer")
		assert.Equal(t, pid, scorer.BadPeers()[0], "Bad peer should match test peer")
	})

	// Test peer remaining bad after decay
	t.Run("remain bad after decay", func(t *testing.T) {
		pid := peer.ID("peer2")

		// Push well over threshold
		scorer.RecordRequest(pid, int(scorers.DefaultDataColumnRPCRequestThreshold*2))
		assert.NotNil(t, scorer.IsBadPeer(pid), "Peer should be bad when significantly over threshold")

		// Apply decay
		scorer.Decay()
		assert.NotNil(t, scorer.IsBadPeer(pid),
			"Peer should remain bad after decay if still above threshold")
	})

	// Test peer recovery through decay
	t.Run("recovery through decay", func(t *testing.T) {
		pid := peer.ID("peer3")

		// Set just over threshold
		scorer.RecordRequest(pid, int(scorers.DefaultDataColumnRPCRequestThreshold+1))
		assert.NotNil(t, scorer.IsBadPeer(pid), "Peer should be bad when just over threshold")

		// Apply multiple decays until peer recovers
		decaysNeeded := (scorers.DefaultDataColumnRPCRequestThreshold+1)/
			scorers.DefaultDataColumnRPCRequestDecay + 1
		for i := uint64(0); i < decaysNeeded; i++ {
			scorer.Decay()
		}
		assert.NoError(t, scorer.IsBadPeer(pid),
			"Peer should recover after sufficient decay cycles")
	})

	// Test multiple peers with different statuses
	t.Run("multiple peer statuses", func(t *testing.T) {
		// Create peers with different request counts
		goodPeer1 := peer.ID("good1")
		goodPeer2 := peer.ID("good2")
		badPeer1 := peer.ID("bad1")
		badPeer2 := peer.ID("bad2")

		// Set request counts
		scorer.RecordRequest(goodPeer1, 1)                                                   // Well below threshold
		scorer.RecordRequest(goodPeer2, int(scorers.DefaultDataColumnRPCRequestThreshold-1)) // Just below
		scorer.RecordRequest(badPeer1, int(scorers.DefaultDataColumnRPCRequestThreshold+1))  // Just above
		scorer.RecordRequest(badPeer2, int(scorers.DefaultDataColumnRPCRequestThreshold*2))  // Well above

		// Verify individual statuses
		assert.NoError(t, scorer.IsBadPeer(goodPeer1), "goodPeer1 should not be bad")
		assert.NoError(t, scorer.IsBadPeer(goodPeer2), "goodPeer2 should not be bad")
		assert.NotNil(t, scorer.IsBadPeer(badPeer1), "badPeer1 should be bad")
		assert.NotNil(t, scorer.IsBadPeer(badPeer2), "badPeer2 should be bad")

		// Verify bad peers list
		badPeers := scorer.BadPeers()
		assert.Equal(t, 2, len(badPeers), "Should have exactly two bad peers")
		assert.Equal(t, true, containsPeer(badPeers, badPeer1), "badPeer1 should be in bad peers list")
		assert.Equal(t, true, containsPeer(badPeers, badPeer2), "badPeer2 should be in bad peers list")
	})

	// Test with custom threshold
	t.Run("custom threshold", func(t *testing.T) {
		customConfig := &scorers.DataColumnRPCRequestScorerConfig{
			Threshold: 50, // Lower threshold
		}
		peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
			ScorerParams: &scorers.Config{
				DataColumnRPCRequestScorerConfig: customConfig,
			},
		})
		customScorer := peerStatuses.Scorers().DataColumnRPCRequestScorer()

		pid := peer.ID("peer4")

		// Test with custom threshold
		customScorer.RecordRequest(pid, 40)
		assert.NoError(t, customScorer.IsBadPeer(pid),
			"Peer should not be bad when below custom threshold")

		customScorer.RecordRequest(pid, 11)
		assert.NotNil(t, customScorer.IsBadPeer(pid),
			"Peer should be bad when above custom threshold")
	})
}

// containsPeer is a helper function to check if a peer ID is in a list
func containsPeer(peers []peer.ID, pid peer.ID) bool {
	for _, p := range peers {
		if p == pid {
			return true
		}
	}
	return false
}
