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
		wantGreyListed  bool
		wantWhiteListIn time.Duration
	}{
		{"no strikes", 0, 0, false, 0},
		{"below threshold", 2, 0, false, 0},
		{"decays offset strikes", 5, 2, false, 0},     // effective 3
		{"at threshold", 4, 0, true, time.Hour},       // one decay to whitelist
		{"past threshold", 7, 0, true, 4 * time.Hour}, // 7-4+1 decays to whitelist
		{"decayed back under threshold", 4, 1, false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			si := testInfo(&PeerScoringInfo{badResponseCount: tc.strikes - tc.decays, badResponses: strikes(tc.strikes)})
			err := scorer.IsPeerGreyListed(testPid, si)
			if tc.wantGreyListed {
				require.ErrorIs(t, err, ErrPeerGreyListed)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantWhiteListIn, scorer.TimeToWhiteListing(testPid, si))
		})
	}
}
