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

// Use a fixed current slot for deterministic testing.
const currentSlot uint64 = 1000

// Define MaxGossipAgeSlots based on default or ensure it's accessible if customized.
const maxGossipAgeSlots = scorers.DefaultDataColumnMaxGossipAgeSlots

// Helper to create a new scorer for isolated sub-tests
func newDataColumnScorer(ctx context.Context, cfg *scorers.DataColumnRPCRequestScorerConfig) *scorers.DataColumnRPCRequestScorer {
	peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
		ScorerParams: &scorers.Config{
			DataColumnRPCRequestScorerConfig: cfg,
		},
	})
	return peerStatuses.Scorers().DataColumnRPCRequestScorer()
}

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
			name: "peer with request exactly MaxGossipAgeSlots old",
			update: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Request a slot that is exactly MaxGossipAgeSlots old
				// Based on current logic `columnSlot+MaxGossipAgeSlots < currentSlot` (false),
				// this request *is* penalized.
				scorer.RecordRequest(peer.ID("peer1"), currentSlot, currentSlot-maxGossipAgeSlots)
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Expected count = 1, score = -1 * PenaltyFactor
				expectedScore := -1.0 * scorers.DefaultDataColumnRPCRequestPenaltyFactor
				assertFloatEqual(t, expectedScore, scorer.Score("peer1"), "Unexpected score for request exactly MaxGossipAgeSlots old")
				assert.NoError(t, scorer.IsBadPeer("peer1"), "Unexpected bad peer status for request exactly MaxGossipAgeSlots old")
			},
		},
		{
			name: "peer with no penalized requests",
			update: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Request a slot that is older than MaxGossipAgeSlots (currentSlot - columnSlot > maxGossipAgeSlots)
				// This should not be penalized.
				scorer.RecordRequest(peer.ID("peer1"), currentSlot, currentSlot-maxGossipAgeSlots-1)
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				assert.Equal(t, 0.0, scorer.Score("peer1"), "Unexpected score for old request")
				assert.NoError(t, scorer.IsBadPeer("peer1"), "Unexpected bad peer status for old request")
			},
		},
		{
			name: "peer with penalized requests below threshold",
			update: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Make 10 requests for recent slots (currentSlot - columnSlot < maxGossipAgeSlots)
				for i := range 10 {
					scorer.RecordRequest(peer.ID("peer1"), currentSlot, currentSlot-uint64(i%int(maxGossipAgeSlots))) // Vary recent slots
				}
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Expected count = 10
				expectedScore := -10.0 * scorers.DefaultDataColumnRPCRequestPenaltyFactor
				assertFloatEqual(t, expectedScore, scorer.Score("peer1"), "Unexpected score")
				assert.NoError(t, scorer.IsBadPeer("peer1"), "Unexpected bad peer status")
			},
		},
		{
			name: "peer at threshold",
			update: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Make requests equal to the threshold
				requests := int(scorers.DefaultDataColumnRPCRequestThreshold)
				for range requests {
					scorer.RecordRequest(peer.ID("peer1"), currentSlot, currentSlot-1) // Request recent slot
				}
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Expected count = threshold
				expectedScore := -float64(scorers.DefaultDataColumnRPCRequestThreshold) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
				assertFloatEqual(t, expectedScore, scorer.Score("peer1"), "Unexpected score")
				assert.NotNil(t, scorer.IsBadPeer("peer1"), "Expected peer to be marked as bad")
			},
		},
		{
			name: "peer above threshold",
			update: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Make requests above the threshold
				requests := int(scorers.DefaultDataColumnRPCRequestThreshold + 10)
				for range requests {
					scorer.RecordRequest(peer.ID("peer1"), currentSlot, currentSlot-1) // Request recent slot
				}
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Expected count = threshold + 10
				expectedScore := -float64(scorers.DefaultDataColumnRPCRequestThreshold+10) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
				assertFloatEqual(t, expectedScore, scorer.Score("peer1"), "Unexpected score")
				assert.NotNil(t, scorer.IsBadPeer("peer1"), "Expected peer to be marked as bad")
			},
		},
		{
			name: "peer with decay",
			update: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// Set initial request count to 50 by making 50 valid requests
				for range 50 {
					scorer.RecordRequest(peer.ID("peer1"), currentSlot, currentSlot-1)
				}
				// Trigger decay
				scorer.Decay()
			},
			check: func(scorer *scorers.DataColumnRPCRequestScorer) {
				// After decay, count should be (50 - DefaultDataColumnRPCRequestDecay)
				expectedCount := uint64(50 - scorers.DefaultDataColumnRPCRequestDecay)
				expectedScore := -float64(expectedCount) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
				assertFloatEqual(t, expectedScore, scorer.Score("peer1"), "Unexpected score after decay")
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

	// Peer1: Below threshold (make 10 valid requests)
	for range 10 {
		scorer.RecordRequest(pid1, currentSlot, currentSlot-1)
	}
	// Peer2: At threshold (make DefaultDataColumnRPCRequestThreshold valid requests)
	requestsAtThreshold := int(scorers.DefaultDataColumnRPCRequestThreshold)
	for range requestsAtThreshold {
		scorer.RecordRequest(pid2, currentSlot, currentSlot-1)
	}
	// Peer3: Above threshold (make DefaultDataColumnRPCRequestThreshold + 1 valid requests)
	requestsAboveThreshold := int(scorers.DefaultDataColumnRPCRequestThreshold + 1)
	for range requestsAboveThreshold {
		scorer.RecordRequest(pid3, currentSlot, currentSlot-1)
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
		// Make 150 valid requests
		for range 150 {
			scorer.RecordRequest(peer.ID("peer1"), currentSlot, currentSlot-1)
		}
		expectedScore := -150.0 * customConfig.PenaltyFactor
		assertFloatEqual(t, expectedScore, scorer.Score("peer1"), "Wrong score with custom penalty factor")
		assert.NoError(t, scorer.IsBadPeer("peer1"), "Peer should not be bad yet (150 < threshold 200)")

		// Push peer over custom threshold (make 51 more valid requests, total 201)
		for range 51 {
			scorer.RecordRequest(peer.ID("peer1"), currentSlot, currentSlot-1)
		}
		assert.NotNil(t, scorer.IsBadPeer("peer1"), "Peer should be bad after exceeding custom threshold (201 > 200)")
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

		// Record 5 valid requests for unknown peer
		for range 5 {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}

		// Verify peer data after request
		state, err := peerStatuses.ConnectionState(pid)
		require.NoError(t, err, "Peer should exist")
		assert.Equal(t, peers.Disconnected, state, "Wrong connection state")
		assert.Equal(t, float64(-0.1), scorer.Score(pid), "Wrong score for request count of 5")
	})

	// Test multiple requests accumulation
	t.Run("request accumulation", func(t *testing.T) {
		pid := peer.ID("peer2")

		// Record series of requests, check cumulative count
		// 10 requests
		for range 10 {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}
		assertFloatEqual(t, -10.0*scorers.DefaultDataColumnRPCRequestPenaltyFactor, scorer.Score(pid), "Wrong score after 10 requests")

		// +15 requests (total 25)
		for range 15 {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}
		assertFloatEqual(t, -25.0*scorers.DefaultDataColumnRPCRequestPenaltyFactor, scorer.Score(pid), "Wrong score after 25 requests")

		// +20 requests (total 45)
		for range 20 {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}
		assertFloatEqual(t, -45.0*scorers.DefaultDataColumnRPCRequestPenaltyFactor, scorer.Score(pid), "Wrong score after 45 requests")
	})

	// Test invalid requests (only invalid peer ID should be ignored now)
	// Requesting old slots is also ignored but tested separately.
	t.Run("invalid requests", func(t *testing.T) {
		pid := peer.ID("peer3")

		// Record initial valid request (count = 1)
		scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		initialScore := scorer.Score(pid)
		require.Equal(t, -1.0*scorers.DefaultDataColumnRPCRequestPenaltyFactor, initialScore, "Check initial score setup")

		// Try invalid requests
		scorer.RecordRequest("", currentSlot, currentSlot-1) // Empty peer ID

		// Verify score unchanged for the valid peer
		assert.Equal(t, initialScore, scorer.Score(pid), "Score should not change for invalid peer ID requests")
	})

	// Test request timing (score accumulation is based on count, not timing between calls)
	t.Run("request timing", func(t *testing.T) {
		pid := peer.ID("peer4")

		// Record first request (count = 1)
		scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		time.Sleep(time.Millisecond)

		// Record second request (count = 2)
		scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		secondScore := scorer.Score(pid)

		// Verify scores reflect accumulation based on count
		expectedSecondScore := -2.0 * scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assertFloatEqual(t, expectedSecondScore, secondScore, "Second score should reflect count=2")
	})

	// Test concurrent requests
	t.Run("concurrent requests", func(t *testing.T) {
		pid := peer.ID("peer5")
		const numRequests = 100

		// Launch multiple goroutines to record requests concurrently
		var wg sync.WaitGroup
		for range numRequests {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Record a valid request
				scorer.RecordRequest(pid, currentSlot, currentSlot-1)
			}()
		}
		wg.Wait()

		// Verify final score (count should be numRequests)
		expectedScore := -float64(numRequests) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assertFloatEqual(t, expectedScore, scorer.Score(pid), "Wrong score after concurrent requests")
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
		// Set initial count to 50
		for range 50 {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}

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
		// Set initial count to 100
		for range 100 {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}

		// Apply decay multiple times
		for range 3 {
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
		// Use a small initial count (e.g., 10)
		for range 10 {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}

		// Single decay might bring count to 0 if Decay >= 10
		scorer.Decay()
		// Calculate expected count after decay
		expectedCount := uint64(0)
		if 10 > scorers.DefaultDataColumnRPCRequestDecay {
			expectedCount = 10 - scorers.DefaultDataColumnRPCRequestDecay
		}
		expectedScore := -float64(expectedCount) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assertFloatEqual(t, expectedScore, scorer.Score(pid), "Score should be correct after first decay")

		// Additional decay should not make score positive (should keep it at 0 if it reached 0)
		scorer.Decay()
		assertFloatEqual(t, 0.0, scorer.Score(pid), "Score should remain zero or decay further towards zero")
	})

	// Test decay with multiple peers
	t.Run("multiple peers decay", func(t *testing.T) {
		pid1 := peer.ID("peer4")
		pid2 := peer.ID("peer5")
		pid3 := peer.ID("peer6")

		// Set different initial counts via valid requests
		counts := map[peer.ID]int{pid1: 30, pid2: 10, pid3: 100}
		for pid, count := range counts {
			for range count {
				scorer.RecordRequest(pid, currentSlot, currentSlot-1)
			}
		}

		// Record initial scores
		initialScores := make(map[peer.ID]float64)
		for pid, count := range counts {
			initialScores[pid] = -float64(count) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
		}

		// Apply decay
		scorer.Decay()

		// Verify each peer's decay
		for pid, initialScore := range initialScores {
			newScore := scorer.Score(pid)
			assert.Equal(t, true, newScore > initialScore || newScore == 0,
				"Score should either increase towards 0 or remain at 0 after decay")
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
		// Set initial count to 100
		for range 100 {
			customScorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}

		// Apply decay with custom value
		customScorer.Decay()

		// Calculate expected count after custom decay
		expectedCount := uint64(0)
		if 100 > customConfig.Decay {
			expectedCount = 100 - customConfig.Decay
		}
		// Use the default penalty factor since it wasn't overridden in this partial config
		expectedScore := -float64(expectedCount) * scorers.DefaultDataColumnRPCRequestPenaltyFactor
		assertFloatEqual(t, expectedScore, customScorer.Score(pid),
			"Wrong score after decay with custom decay value")
	})
}

func TestScorers_DataColumnRPCRequest_BadPeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Note: Each sub-test now creates its own scorer for isolation.

	// Test transition from good to bad
	t.Run("good to bad transition", func(t *testing.T) {
		scorer := newDataColumnScorer(ctx, nil) // Use default config
		pid := peer.ID("peer1")

		// Start with requests count below threshold
		requestsBelow := int(scorers.DefaultDataColumnRPCRequestThreshold - 10)
		for range requestsBelow {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}
		assert.NoError(t, scorer.IsBadPeer(pid), "Peer should not be bad when below threshold")
		assert.Equal(t, 0, len(scorer.BadPeers()), "Should have no bad peers")

		// Add more requests to exceed threshold (15 more, total = threshold + 5)
		for range 15 {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}
		assert.NotNil(t, scorer.IsBadPeer(pid), "Peer should be bad after exceeding threshold")
		assert.Equal(t, 1, len(scorer.BadPeers()), "Should have one bad peer")
		assert.Equal(t, pid, scorer.BadPeers()[0], "Bad peer should match test peer")
	})

	// Test peer remaining bad after decay
	t.Run("remain bad after decay", func(t *testing.T) {
		scorer := newDataColumnScorer(ctx, nil) // Use default config
		pid := peer.ID("peer2")

		// Push well over threshold (e.g., threshold * 2). Using defaults (100), this is 200.
		requestsOver := int(scorers.DefaultDataColumnRPCRequestThreshold * 2)
		for range requestsOver {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}
		assert.NotNil(t, scorer.IsBadPeer(pid), "Peer should be bad when significantly over threshold")

		// Apply decay once. With defaults (decay=10), count becomes 190, still >= 100.
		scorer.Decay()

		// Assert peer is still bad after one decay cycle.
		assert.NotNil(t, scorer.IsBadPeer(pid),
			"Peer should remain bad after decay as count (190) is still >= threshold (100)")
	})

	// Test peer recovery through decay
	t.Run("recovery through decay", func(t *testing.T) {
		scorer := newDataColumnScorer(ctx, nil) // Use default config
		pid := peer.ID("peer3")

		// Set just over threshold (threshold + 1). Using defaults, this is 101.
		requestsJustOver := int(scorers.DefaultDataColumnRPCRequestThreshold + 1)
		for range requestsJustOver {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}
		assert.NotNil(t, scorer.IsBadPeer(pid), "Peer should be bad when just over threshold")

		// Apply decay repeatedly until the scorer no longer considers the peer bad.
		const maxDecays = 1000 // Safety break
		decaysApplied := 0
		for scorer.IsBadPeer(pid) != nil {
			if decaysApplied >= maxDecays {
				t.Fatalf("Peer did not recover after %d decay cycles", maxDecays)
			}
			scorer.Decay()
			decaysApplied++
		}

		// Assert the peer is no longer bad.
		assert.NoError(t, scorer.IsBadPeer(pid),
			"Peer should recover after %d decay cycles", decaysApplied)
	})

	// Test multiple peers with different statuses
	t.Run("multiple peer statuses", func(t *testing.T) {
		scorer := newDataColumnScorer(ctx, nil) // Use default config
		// Create peers
		goodPeer1 := peer.ID("good1")
		goodPeer2 := peer.ID("good2")
		badPeer1 := peer.ID("bad1")
		badPeer2 := peer.ID("bad2")

		// Set request counts via valid requests
		// goodPeer1: 1 request
		scorer.RecordRequest(goodPeer1, currentSlot, currentSlot-1)
		// goodPeer2: threshold - 1 requests
		for range int(scorers.DefaultDataColumnRPCRequestThreshold - 1) {
			scorer.RecordRequest(goodPeer2, currentSlot, currentSlot-1)
		}
		// badPeer1: threshold + 1 requests
		for range int(scorers.DefaultDataColumnRPCRequestThreshold + 1) {
			scorer.RecordRequest(badPeer1, currentSlot, currentSlot-1)
		}
		// badPeer2: threshold * 2 requests
		for range int(scorers.DefaultDataColumnRPCRequestThreshold * 2) {
			scorer.RecordRequest(badPeer2, currentSlot, currentSlot-1)
		}

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
		// Create scorer with custom config
		scorer := newDataColumnScorer(ctx, customConfig)

		pid := peer.ID("peer4")

		// Test with custom threshold
		// Make 40 valid requests (below custom threshold 50)
		for range 40 {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}
		assert.NoError(t, scorer.IsBadPeer(pid),
			"Peer should not be bad when below custom threshold")

		// Make 11 more valid requests (total 51, above custom threshold 50)
		for range 11 {
			scorer.RecordRequest(pid, currentSlot, currentSlot-1)
		}
		assert.NotNil(t, scorer.IsBadPeer(pid),
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
