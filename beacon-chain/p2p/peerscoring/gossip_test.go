package peerscoring

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestGossipScorer(t *testing.T) {
	scorer := gossipScorer{}
	tests := []struct {
		name           string
		gossipScore    float64
		wantScore      float64
		wantGreylisted bool
	}{
		{"no gossip data", 0, 0, false},
		{"positive score", 8, 2, false}, // 8 * 0.25
		{"negative above threshold", -100, -25, false},
		{"at threshold", -16000, -4000, false}, // greylisting is strictly below
		{"below threshold", -16000.5, -4000.125, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			si := testInfo(&PeerScoringInfo{gossipScore: tc.gossipScore}, 0, 0)
			require.Equal(t, tc.wantScore, scorer.Score(testPid, si))
			require.Equal(t, tc.wantGreylisted, scorer.IsPeerGreylisted(testPid, si))
			require.Equal(t, time.Duration(0), scorer.TimeToWhitelisting(testPid, si)) // recovery is libp2p-driven
		})
	}
}
