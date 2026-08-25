package peerscoring

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBadResponsesScorer(t *testing.T) {
	scorer := badResponsesScorer{}
	tests := []struct {
		name            string
		strikes         int
		decays          int
		wantScore       float64
		wantGreylisted  bool
		wantWhitelistIn time.Duration
	}{
		{"no strikes", 0, 0, 0, false, 0},
		{"below threshold", 2, 0, -2.5, false, 0},            // -(2/4*10) * 0.5
		{"decays offset strikes", 5, 2, -3.75, false, 0},     // effective 3
		{"at threshold", 4, 0, -5, true, time.Hour},          // score stays proportional, one decay to whitelist
		{"past threshold", 7, 0, -8.75, true, 4 * time.Hour}, // 7-4+1 decays to whitelist
		{"decayed back under threshold", 4, 1, -3.75, false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			si := testInfo(&PeerScoringInfo{badResponses: strikes(tc.strikes), nDecays: tc.decays}, 0, 0)
			require.Equal(t, tc.wantScore, scorer.Score(testPid, si))
			require.Equal(t, tc.wantGreylisted, scorer.IsPeerGreylisted(testPid, si))
			require.Equal(t, tc.wantWhitelistIn, scorer.TimeToWhitelisting(testPid, si))
		})
	}
}
