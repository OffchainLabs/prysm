package peerscoring

import (
	"testing"
	"time"

	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
)

func TestRpcStatusScorer(t *testing.T) {
	scorer := rpcStatusScorer{}
	fresh := time.Now()
	expired := time.Now().Add(-2 * time.Hour) // beyond the 1h test TTL
	tests := []struct {
		name           string
		status         *RpcStatus
		wantGreyListed bool
	}{
		{"no status", nil, false},
		{"no chain state", &RpcStatus{}, false},
		{
			"non-terminal validation error does not greylist",
			&RpcStatus{chainState: &pb.StatusV2{HeadSlot: 50}, validationError: errors.New("temporary"), lastUpdated: fresh},
			false,
		},
		// Terminal validation errors greylist the peer until the verdict expires.
		{"wrong fork digest greylists", &RpcStatus{validationError: p2ptypes.ErrWrongForkDigestVersion, lastUpdated: fresh}, true},
		{"invalid finalized root greylists", &RpcStatus{validationError: p2ptypes.ErrInvalidFinalizedRoot, lastUpdated: fresh}, true},
		{"invalid request greylists", &RpcStatus{validationError: p2ptypes.ErrInvalidRequest, lastUpdated: fresh}, true},
		{
			"wrapped terminal error greylists",
			&RpcStatus{validationError: errors.Wrap(p2ptypes.ErrWrongForkDigestVersion, "status"), lastUpdated: fresh},
			true,
		},
		{
			"terminal error with chain state still greylists",
			&RpcStatus{chainState: &pb.StatusV2{HeadSlot: 50}, validationError: p2ptypes.ErrInvalidRequest, lastUpdated: fresh},
			true,
		},
		{
			"terminal verdict older than the TTL no longer greylists",
			&RpcStatus{validationError: p2ptypes.ErrWrongForkDigestVersion, lastUpdated: expired},
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			si := testInfo(&PeerScoringInfo{rpcStatus: tc.status})
			err := scorer.IsPeerGreyListed(testPid, si)
			ttw := scorer.TimeToWhiteListing(testPid, si)
			if tc.wantGreyListed {
				require.ErrorIs(t, err, ErrPeerGreyListed)
				// The verdict expires with the TTL, so the remaining time is bounded by it.
				require.Equal(t, true, ttw > 0 && ttw <= si.params.statusGreyListTTL)
			} else {
				require.NoError(t, err)
				require.Equal(t, time.Duration(0), ttw)
			}
		})
	}
}
