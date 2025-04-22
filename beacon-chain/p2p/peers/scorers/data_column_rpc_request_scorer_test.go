package scorers_test

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/scorers"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
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
