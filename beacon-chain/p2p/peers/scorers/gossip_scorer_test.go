package scorers_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/scorers"
	pbrpc "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
)

func TestScorers_Gossip_Score(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name   string
		update func(scorer *scorers.GossipScorer)
		check  func(scorer *scorers.GossipScorer)
	}{
		{
			name: "nonexistent peer",
			update: func(*scorers.GossipScorer) {
			},
			check: func(scorer *scorers.GossipScorer) {
				assert.Equal(t, 0.0, scorer.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "existent bad peer",
			update: func(scorer *scorers.GossipScorer) {
				scorer.SetGossipData("peer1", -101.0, 1, nil)
			},
			check: func(scorer *scorers.GossipScorer) {
				assert.Equal(t, -101.0, scorer.Score("peer1"), "Unexpected score")
				assert.NotNil(t, scorer.IsBadPeer("peer1"), "Unexpected good peer")
			},
		},
		{
			name: "good peer",
			update: func(scorer *scorers.GossipScorer) {
				scorer.SetGossipData("peer1", 10.0, 0, map[string]*pbrpc.TopicScoreSnapshot{"a": {TimeInMesh: 100}})
			},
			check: func(scorer *scorers.GossipScorer) {
				assert.Equal(t, 10.0, scorer.Score("peer1"), "Unexpected score")
				assert.NoError(t, scorer.IsBadPeer("peer1"), "Unexpected bad peer")
				_, _, topicMap, err := scorer.GossipData("peer1")
				assert.NoError(t, err)
				assert.Equal(t, uint64(100), topicMap["a"].TimeInMesh, "incorrect time in mesh")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
				ScorerParams: &scorers.Config{},
			})
			scorer := peerStatuses.Scorers().GossipScorer()
			if tt.update != nil {
				tt.update(scorer)
			}
			tt.check(scorer)
		})
	}
}
